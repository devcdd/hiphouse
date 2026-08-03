package main

// 공개 프로필 — 남이 내 활동을 볼 수 있는 유일한 경로.
//
// 공개 범위는 이미 공개인 것에 맞춘다: listComments가 user_id와 nickname을 그대로
// 내보내므로 그 둘은 새로 노출되는 정보가 아니다. 여기서 새로 공개되는 건 "누가 어떤
// 앨범에 몇 점을 줬는지"뿐이고, 즐겨찾기·팔로우 목록은 애초에 공개하지 않는다
// (GET /follows, /favorites는 전부 requireAuth라 caller 본인 것만 돌려준다).
//
// users.profile_public이 이 페이지 전체의 스위치다. 끄면 남는 건 닉네임과 가입일뿐 —
// 평가 목록도, 평가·댓글 집계도 나가지 않는다. 개별 항목마다 토글을 두지 않은 이유는
// 숨길 수 있는 표면이 여기 하나이기 때문이고, 나중에 프로필에 뭔가를 더 붙이더라도
// 이 플래그 하나만 보면 되도록 이름을 ratings_ 가 아니라 profile_ 로 잡았다.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// PublicUser is the profile header: who they are plus how much they've rated.
// RatingAvg is in stars (0..5), null when they haven't rated anything.
//
// ProfilePublic false means the owner turned 프로필 공개 off: every aggregate here
// is blanked and listPublicRatedAlbums returns nothing. The flag itself stays
// public so the page can say "비공개" instead of "활동이 없음" — the two look the
// same otherwise, and only one of them is true.
type PublicUser struct {
	ID            string   `json:"id" db:"id"`
	Nickname      string   `json:"nickname" db:"nickname"`
	CreatedAt     string   `json:"created_at" db:"created_at"`
	ProfilePublic bool     `json:"profile_public" db:"profile_public"`
	RatingCount   int      `json:"rating_count" db:"rating_count"`
	RatingAvg     *float64 `json:"rating_avg" db:"rating_avg"`
	CommentCount  int      `json:"comment_count" db:"comment_count"`
}

// RatedAlbum is an album plus the score this profile's owner gave it.
// The embedded Album keeps the card shape identical to every other album list.
type RatedAlbum struct {
	Album
	Score int `json:"score" db:"score"`
}

// AdminUser is one row of the 관리자 회원 목록: identity plus how much they've
// done. Unlike PublicUser this carries the role and the private-ish activity
// counts (즐겨찾기·팔로우), which is why it is admin-only.
type AdminUser struct {
	ID            string `json:"id" db:"id"`
	Nickname      string `json:"nickname" db:"nickname"`
	Role          string `json:"role" db:"role"`
	CreatedAt     string `json:"created_at" db:"created_at"`
	RatingCount   int    `json:"rating_count" db:"rating_count"`
	CommentCount  int    `json:"comment_count" db:"comment_count"`
	FavoriteCount int    `json:"favorite_count" db:"favorite_count"`
	FollowCount   int    `json:"follow_count" db:"follow_count"`
}

// GET /admin/users?q=&limit=&offset= — 가입 최신순 회원 목록. q는 닉네임/ID 부분 일치.
func (s *server) adminListUsers(w http.ResponseWriter, r *http.Request) {
	limit, offset := clampPage(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, err := s.db.Query(r.Context(), `
		SELECT u.id, u.nickname, u.role,
		       to_char(u.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
		       (SELECT COUNT(*)::int FROM ratings rt WHERE rt.user_id = u.id) AS rating_count,
		       (SELECT COUNT(*)::int FROM comments c WHERE c.user_id = u.id AND c.deleted_at IS NULL) AS comment_count,
		       (SELECT COUNT(*)::int FROM favorites f WHERE f.user_id = u.id) AS favorite_count,
		       (SELECT COUNT(*)::int FROM follows fl WHERE fl.user_id = u.id) AS follow_count
		FROM users u
		WHERE $1 = '' OR u.nickname ILIKE '%' || $1 || '%' OR u.id ILIKE '%' || $1 || '%'
		ORDER BY u.created_at DESC, u.id LIMIT $2 OFFSET $3`, q, limit, offset)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	users, err := pgx.CollectRows(rows, pgx.RowToStructByName[AdminUser])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if users == nil {
		users = []AdminUser{}
	}
	writeJSON(w, 200, users)
}

// publicUserSQL — 프로필 헤더. 닉네임과 가입일만 무조건 나가고, 활동 집계 셋은
// 전부 profile_public 뒤에 있다. 비공개면 count는 0, avg는 NULL.
//
// 댓글 자체는 앨범 페이지에 닉네임과 함께 계속 붙어 있다 (숨기면 남의 답글이 상대
// 없이 남는다). 다만 "이 사람이 몇 개 썼는지"는 프로필 통계라 프로필을 닫으면 같이 닫는다.
const publicUserSQL = `
	SELECT u.id, u.nickname, u.profile_public,
	       to_char(u.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
	       CASE WHEN u.profile_public
	            THEN (SELECT COUNT(*)::int FROM ratings rt WHERE rt.user_id = u.id)
	            ELSE 0 END AS rating_count,
	       CASE WHEN u.profile_public
	            THEN (SELECT AVG(rt.score)::float / 2 FROM ratings rt WHERE rt.user_id = u.id)
	            END AS rating_avg,
	       CASE WHEN u.profile_public
	            THEN (SELECT COUNT(*)::int FROM comments c WHERE c.user_id = u.id AND c.deleted_at IS NULL)
	            ELSE 0 END AS comment_count
	FROM users u WHERE u.id = $1`

// GET /users/{id} — public profile header.
func (s *server) getPublicUser(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), publicUserSQL, r.PathValue("id"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	user, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[PublicUser])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, 404, "user not found")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, user)
}

// profilePrivacyGuard is the enforcement point for 프로필 비공개. It rides along in
// the same query rather than a separate lookup, so there is no window where the
// check passes and the rows are read under a since-flipped flag. Any future
// public-profile list must carry this same guard.
const profilePrivacyGuard = "EXISTS (SELECT 1 FROM users pu WHERE pu.id = $1 AND pu.profile_public)"

// publicRatedAlbumsSQL — albums this user rated, most-recently rated first.
// Soft-deleted albums are hidden: a profile is a public page, not an admin view.
// A private profile yields zero rows; the header's profile_public is what tells
// the page to say 비공개 rather than 평가 없음.
//
// $1 is the profile owner, NOT the caller: this is a public read with no session,
// so the guard binds the owner to their own flag.
const publicRatedAlbumsSQL = "SELECT " + albumSelectCols + ", rt.score " +
	"FROM albums JOIN ratings rt ON rt.album_id = albums.id " +
	"WHERE rt.user_id = $1 AND albums.deleted_at IS NULL AND " + profilePrivacyGuard + " " +
	"ORDER BY rt.updated_at DESC LIMIT $2 OFFSET $3"

// GET /users/{id}/ratings
func (s *server) listPublicRatedAlbums(w http.ResponseWriter, r *http.Request) {
	limit, offset := clampPage(r)
	rows, err := s.db.Query(r.Context(), publicRatedAlbumsSQL, r.PathValue("id"), limit, offset)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	albums, err := pgx.CollectRows(rows, pgx.RowToStructByName[RatedAlbum])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if albums == nil {
		albums = []RatedAlbum{}
	}
	writeJSON(w, 200, albums)
}

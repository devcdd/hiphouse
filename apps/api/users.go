package main

// 공개 프로필 — 남이 내 활동을 볼 수 있는 유일한 경로.
//
// 공개 범위는 이미 공개인 것에 맞춘다: listComments가 user_id와 nickname을 그대로
// 내보내므로 그 둘은 새로 노출되는 정보가 아니다. 여기서 새로 공개되는 건 "누가 어떤
// 앨범에 몇 점을 줬는지"뿐이고, 즐겨찾기·팔로우 목록은 애초에 공개하지 않는다
// (GET /follows, /favorites는 전부 requireAuth라 caller 본인 것만 돌려준다).
//
// users.profile_public이 이 페이지 전체의 스위치다. 끄면 남에게 남는 건 닉네임과
// 가입일뿐 — 평가 목록도, 평가·댓글 집계도 나가지 않는다. 주인 본인은 예외로 전부
// 그대로 본다(visibleToViewer). 개별 항목마다 토글을 두지 않은 이유는
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
// ProfilePublic이 false면 주인이 프로필 공개를 끈 것이다. 주인 본인을 뺀 모두에게
// 여기 집계는 전부 비고 listPublicRatedAlbums도 아무것도 돌려주지 않는다. 플래그
// 자체는 계속 공개다 — 그래야 화면이 "활동이 없음"이 아니라 "비공개"라고 말할 수 있다.
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

// visibleToViewer — 비공개 프로필이라도 주인 본인에게는 열어 준다. 공개 설정 토글이
// 이 페이지 위에 있으니 내용이 통째로 비면 뭘 껐는지 확인할 방법이 없다. $2는 caller의
// id(비로그인이면 "")라 남에게는 절대 걸리지 않는다.
const visibleToViewer = "(u.profile_public OR u.id = $2)"

// publicUserSQL — 프로필 헤더. 닉네임과 가입일만 무조건 나가고, 활동 집계 셋은
// 전부 이 게이트 뒤에 있다. 남이 보는 비공개 프로필이면 count는 0, avg는 NULL.
//
// 댓글 자체는 앨범 페이지에 닉네임과 함께 계속 붙어 있다 (숨기면 남의 답글이 상대
// 없이 남는다). 다만 "이 사람이 몇 개 썼는지"는 프로필 통계라 프로필을 닫으면 같이 닫는다.
const publicUserSQL = `
	SELECT u.id, u.nickname, u.profile_public,
	       to_char(u.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
	       CASE WHEN ` + visibleToViewer + `
	            THEN (SELECT COUNT(*)::int FROM ratings rt WHERE rt.user_id = u.id)
	            ELSE 0 END AS rating_count,
	       CASE WHEN ` + visibleToViewer + `
	            THEN (SELECT AVG(rt.score)::float / 2 FROM ratings rt WHERE rt.user_id = u.id)
	            END AS rating_avg,
	       CASE WHEN ` + visibleToViewer + `
	            THEN (SELECT COUNT(*)::int FROM comments c WHERE c.user_id = u.id AND c.deleted_at IS NULL)
	            ELSE 0 END AS comment_count
	FROM users u WHERE u.id = $1`

// GET /users/{id} — public profile header.
func (s *server) getPublicUser(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), publicUserSQL, r.PathValue("id"), s.callerID(r))
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
const profilePrivacyGuard = "EXISTS (SELECT 1 FROM users pu WHERE pu.id = $1 AND (pu.profile_public OR pu.id = $4))"

// publicRatedAlbumsSQL — 이 사용자가 평가한 앨범, 최근 평가순. 소프트 삭제된 앨범은
// 빠진다 (프로필은 공개 페이지지 관리자 화면이 아니다). 비공개 프로필은 주인 말고는
// 0행이고, 화면이 "평가 없음" 대신 "비공개"라고 말하게 하는 건 헤더의 profile_public이다.
//
// $1은 프로필 주인, $4는 caller(비로그인이면 ""). 가드는 주인을 자기 플래그에 묶고,
// 그 뒤를 읽을 수 있는 건 주인뿐이다.
const publicRatedAlbumsSQL = "SELECT " + albumSelectCols + ", rt.score " +
	"FROM albums JOIN ratings rt ON rt.album_id = albums.id " +
	"WHERE rt.user_id = $1 AND albums.deleted_at IS NULL AND " + profilePrivacyGuard + " " +
	"ORDER BY rt.updated_at DESC LIMIT $2 OFFSET $3"

// GET /users/{id}/ratings
func (s *server) listPublicRatedAlbums(w http.ResponseWriter, r *http.Request) {
	limit, offset := clampPage(r)
	rows, err := s.db.Query(r.Context(), publicRatedAlbumsSQL, r.PathValue("id"), limit, offset, s.callerID(r))
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

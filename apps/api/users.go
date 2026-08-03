package main

// 공개 프로필 — 다른 사용자의 평가를 볼 수 있는 유일한 경로.
//
// 공개 범위는 이미 공개인 것에 맞춘다: listComments가 user_id와 nickname을 그대로
// 내보내므로 그 둘은 새로 노출되는 정보가 아니다. 여기서 새로 공개되는 건 "누가 어떤
// 앨범에 몇 점을 줬는지"뿐이고, 즐겨찾기·팔로우 목록은 공개하지 않는다.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// PublicUser is the profile header: who they are plus how much they've rated.
// RatingAvg is in stars (0..5), null when they haven't rated anything.
type PublicUser struct {
	ID           string   `json:"id" db:"id"`
	Nickname     string   `json:"nickname" db:"nickname"`
	CreatedAt    string   `json:"created_at" db:"created_at"`
	RatingCount  int      `json:"rating_count" db:"rating_count"`
	RatingAvg    *float64 `json:"rating_avg" db:"rating_avg"`
	CommentCount int      `json:"comment_count" db:"comment_count"`
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

// GET /users/{id} — public profile header.
func (s *server) getPublicUser(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT u.id, u.nickname,
		       to_char(u.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
		       (SELECT COUNT(*)::int FROM ratings rt WHERE rt.user_id = u.id) AS rating_count,
		       (SELECT AVG(rt.score)::float / 2 FROM ratings rt WHERE rt.user_id = u.id) AS rating_avg,
		       (SELECT COUNT(*)::int FROM comments c WHERE c.user_id = u.id AND c.deleted_at IS NULL) AS comment_count
		FROM users u WHERE u.id = $1`, r.PathValue("id"))
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

// GET /users/{id}/ratings — albums this user rated, most-recently rated first.
// Soft-deleted albums are hidden: a profile is a public page, not an admin view.
func (s *server) listPublicRatedAlbums(w http.ResponseWriter, r *http.Request) {
	limit, offset := clampPage(r)
	rows, err := s.db.Query(r.Context(),
		"SELECT "+albumSelectCols+", rt.score FROM albums JOIN ratings rt ON rt.album_id = albums.id "+
			"WHERE rt.user_id = $1 AND albums.deleted_at IS NULL "+
			"ORDER BY rt.updated_at DESC LIMIT $2 OFFSET $3", r.PathValue("id"), limit, offset)
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

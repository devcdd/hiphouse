package main

// 공개 프로필 — 다른 사용자의 평가를 볼 수 있는 유일한 경로.
//
// 공개 범위는 이미 공개인 것에 맞춘다: listComments가 user_id와 nickname을 그대로
// 내보내므로 그 둘은 새로 노출되는 정보가 아니다. 여기서 새로 공개되는 건 "누가 어떤
// 앨범에 몇 점을 줬는지"뿐이고, 즐겨찾기·팔로우 목록은 공개하지 않는다.

import (
	"errors"
	"net/http"

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

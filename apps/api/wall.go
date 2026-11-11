package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	maxWallPostLen  = 50
	wallPageSize    = 50
	wallCooldownSec = 3 // 같은 사용자가 같은 담벼락에 연달아 쓰는 도배만 막는 최소 간격
)

// WallPost is one artist 담벼락 message.
type WallPost struct {
	ID        int64  `json:"id" db:"id"`
	UserID    string `json:"user_id" db:"user_id"`
	Nickname  string `json:"nickname" db:"nickname"`
	Body      string `json:"body" db:"body"`
	CreatedAt string `json:"created_at" db:"created_at"`
}

const wallPostCols = `p.id, p.user_id, u.nickname, p.body,
	to_char(p.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at`

// validWallBody trims the message and reports why it can't be posted ("" = ok).
func validWallBody(raw string) (string, string) {
	text := strings.TrimSpace(raw)
	switch {
	case text == "":
		return "", "body is required"
	case len([]rune(text)) > maxWallPostLen:
		return "", "body is too long (max " + strconv.Itoa(maxWallPostLen) + " characters)"
	}
	return text, ""
}

// listWallPosts는 공개 읽기다. 최신 wallPageSize개를 오래된 순으로 돌려주고,
// ?before=<id>를 주면 그 id보다 오래된 페이지를 준다 (채팅창 위로 더 불러오기).
func (s *server) listWallPosts(w http.ResponseWriter, r *http.Request) {
	before := queryIntPtr(r, "before")
	sql := "SELECT * FROM (SELECT " + wallPostCols + " FROM artist_wall_posts p JOIN users u ON u.id = p.user_id WHERE p.artist_id = $1"
	args := []any{r.PathValue("id")}
	if before != nil {
		args = append(args, *before)
		sql += " AND p.id < $2"
	}
	sql += " ORDER BY p.id DESC LIMIT " + strconv.Itoa(wallPageSize) + ") page ORDER BY id"
	rows, err := s.db.Query(r.Context(), sql, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	posts, err := pgx.CollectRows(rows, pgx.RowToStructByName[WallPost])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if posts == nil {
		posts = []WallPost{}
	}
	writeJSON(w, 200, posts)
}

func (s *server) addWallPost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &body) {
		return
	}
	text, msg := validWallBody(body.Body)
	if msg != "" {
		writeErr(w, 400, msg)
		return
	}
	artistID, userID := r.PathValue("id"), currentUser(r).ID

	var exists, tooSoon bool
	err := s.db.QueryRow(r.Context(), `
		SELECT EXISTS (SELECT 1 FROM artists WHERE id=$1),
		       EXISTS (SELECT 1 FROM artist_wall_posts WHERE artist_id=$1 AND user_id=$2
		               AND created_at > now() - make_interval(secs => $3))`,
		artistID, userID, wallCooldownSec).Scan(&exists, &tooSoon)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !exists {
		writeErr(w, 404, "artist not found")
		return
	}
	if tooSoon {
		writeErr(w, 429, "posting too fast")
		return
	}

	var p WallPost
	err = s.db.QueryRow(r.Context(), `
		WITH p AS (INSERT INTO artist_wall_posts(artist_id, user_id, body) VALUES($1,$2,$3) RETURNING *)
		SELECT `+wallPostCols+` FROM p JOIN users u ON u.id = p.user_id`,
		artistID, userID, text).Scan(&p.ID, &p.UserID, &p.Nickname, &p.Body, &p.CreatedAt)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, p)
}

// deleteWallPost는 작성자 본인 또는 관리자만. 스레드가 없어서 소프트 삭제 없이 바로 지운다.
func (s *server) deleteWallPost(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid post id")
		return
	}
	u := currentUser(r)
	sql := "DELETE FROM artist_wall_posts WHERE id=$1"
	args := []any{id}
	if u.Role != "admin" {
		sql += " AND user_id=$2"
		args = append(args, u.ID)
	}
	tag, err := s.db.Exec(r.Context(), sql, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, 404, "post not found")
		return
	}
	w.WriteHeader(204)
}

package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	maxWallPostLen = 50
	wallPageSize   = 50
	wallDailyLimit = 3 // 사용자 한 명이 담벼락 하나에 하루(KST) 쓸 수 있는 글 수. 지운 글도 센다.
)

// WallPost is one artist 담벼락 message.
type WallPost struct {
	ID        int64  `json:"id" db:"id"`
	UserID    string `json:"user_id" db:"user_id"`
	Nickname  string `json:"nickname" db:"nickname"`
	Body      string `json:"body" db:"body"`
	CreatedAt string `json:"created_at" db:"created_at"`
	// 작성자가 관리자 검수를 통과한 아티스트 본인이면 그 아티스트. 아니면 null.
	ArtistID   *string `json:"artist_id" db:"artist_id"`
	ArtistName *string `json:"artist_name" db:"artist_name"`
}

const wallPostCols = `p.id, p.user_id, u.nickname, p.body,
	to_char(p.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
	va.id AS artist_id, COALESCE(va.display_name, va.name) AS artist_name`

const wallPostJoins = ` JOIN users u ON u.id = p.user_id
	LEFT JOIN artist_claims cl ON cl.user_id = p.user_id AND cl.status = 'approved'
	LEFT JOIN artists va ON va.id = cl.artist_id`

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
	sql := "SELECT * FROM (SELECT " + wallPostCols + " FROM artist_wall_posts p" + wallPostJoins + " WHERE p.artist_id = $1 AND p.deleted_at IS NULL"
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

// WallQuota는 오늘 이 담벼락에 더 쓸 수 있는 횟수다. 담벼락 주인 아티스트로 인증된
// 본인은 팬과 소통하는 자리라서 제한하지 않는다 (Unlimited).
type WallQuota struct {
	Limit     int  `json:"limit"`
	Used      int  `json:"used"`
	Unlimited bool `json:"unlimited"`
}

// wallQuota는 tx 안에서 부른다 — addWallPost가 같은 tx에서 advisory lock을 잡고 세야
// 동시에 들어온 요청 두 개가 둘 다 3번째로 통과하지 않는다.
func wallQuota(ctx context.Context, q pgx.Tx, artistID, userID string) (WallQuota, error) {
	quota := WallQuota{Limit: wallDailyLimit}
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM artist_claims WHERE user_id=$2 AND artist_id=$1 AND status='approved'),
		       (SELECT COUNT(*)::int FROM artist_wall_posts WHERE artist_id=$1 AND user_id=$2
		          AND created_at >= date_trunc('day', now() AT TIME ZONE 'Asia/Seoul') AT TIME ZONE 'Asia/Seoul')`,
		artistID, userID).Scan(&quota.Unlimited, &quota.Used)
	return quota, err
}

func (s *server) getWallQuota(w http.ResponseWriter, r *http.Request) {
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback(r.Context())
	quota, err := wallQuota(r.Context(), tx, r.PathValue("id"), currentUser(r).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, quota)
}

// countWallPosts는 접힌 담벼락 헤더용 공개 읽기다. 지운 글은 세지 않는다.
func (s *server) countWallPosts(w http.ResponseWriter, r *http.Request) {
	var n int
	if err := s.db.QueryRow(r.Context(),
		"SELECT COUNT(*)::int FROM artist_wall_posts WHERE artist_id=$1 AND deleted_at IS NULL", r.PathValue("id")).Scan(&n); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]int{"count": n})
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
	ctx := r.Context()
	artistID, userID := r.PathValue("id"), currentUser(r).ID

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM artists WHERE id=$1)", artistID).Scan(&exists); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !exists {
		writeErr(w, 404, "artist not found")
		return
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('wall:' || $1 || ':' || $2))", artistID, userID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	quota, err := wallQuota(ctx, tx, artistID, userID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !quota.Unlimited && quota.Used >= quota.Limit {
		writeErr(w, 429, "daily wall post limit reached")
		return
	}

	var p WallPost
	err = tx.QueryRow(ctx, `
		WITH p AS (INSERT INTO artist_wall_posts(artist_id, user_id, body) VALUES($1,$2,$3) RETURNING *)
		SELECT `+wallPostCols+` FROM p`+wallPostJoins,
		artistID, userID, text).Scan(&p.ID, &p.UserID, &p.Nickname, &p.Body, &p.CreatedAt, &p.ArtistID, &p.ArtistName)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, p)
}

// deleteWallPost는 작성자 본인 또는 관리자만. 소프트 삭제라서 지운 글도 하루 횟수에 남는다
// (지우고 다시 쓰는 식으로 제한을 우회하지 못하게).
func (s *server) deleteWallPost(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid post id")
		return
	}
	u := currentUser(r)
	sql := "UPDATE artist_wall_posts SET deleted_at=now() WHERE id=$1 AND deleted_at IS NULL"
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

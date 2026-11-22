package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const maxCommentLen = 1000

// Comment is one album comment. ParentID nil = top-level; replies are one level
// deep only, so a reply never becomes a parent itself.
type Comment struct {
	ID       int64  `json:"id" db:"id"`
	ParentID *int64 `json:"parent_id" db:"parent_id"`
	UserID   string `json:"user_id" db:"user_id"`
	Nickname string `json:"nickname" db:"nickname"`
	// AvatarURL은 API 기준 프로필 이미지 경로다. 사진이 없거나 탈퇴한 작성자면 null.
	AvatarURL *string `json:"avatar_url" db:"avatar_url"`
	Body      string  `json:"body" db:"body"`
	CreatedAt string  `json:"created_at" db:"created_at"`
	Deleted   bool    `json:"deleted" db:"deleted"`
	Edited    bool    `json:"edited" db:"edited"`
}

// listComments is public. Deleted comments survive only as tombstones for their
// still-live replies; a deleted comment with nothing under it disappears.
// Ordering groups each thread by its root (ids are monotonic, so this is
// oldest-first both between threads and inside one).
func (s *server) listComments(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT c.id, c.parent_id, COALESCE(c.user_id, '') AS user_id, COALESCE(u.nickname, '') AS nickname,
		       (SELECT `+avatarURLExpr+` FROM users au WHERE au.id = c.user_id) AS avatar_url,
		       CASE WHEN c.deleted_at IS NULL THEN c.body ELSE '' END AS body,
		       to_char(c.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
		       (c.deleted_at IS NOT NULL) AS deleted,
		       (c.edited_at IS NOT NULL) AS edited
		FROM comments c LEFT JOIN users u ON u.id = c.user_id
		WHERE c.album_id = $1
		  AND (c.deleted_at IS NULL
		       OR EXISTS (SELECT 1 FROM comments rp WHERE rp.parent_id = c.id AND rp.deleted_at IS NULL))
		ORDER BY COALESCE(c.parent_id, c.id), c.id`, r.PathValue("id"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	comments, err := pgx.CollectRows(rows, pgx.RowToStructByName[Comment])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if comments == nil {
		comments = []Comment{}
	}
	writeJSON(w, 200, comments)
}

func (s *server) addComment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Body     string `json:"body"`
		ParentID *int64 `json:"parent_id"`
	}
	if !decode(w, r, &body) {
		return
	}
	text := strings.TrimSpace(body.Body)
	if text == "" {
		writeErr(w, 400, "body is required")
		return
	}
	if len([]rune(text)) > maxCommentLen {
		writeErr(w, 400, "body is too long (max "+strconv.Itoa(maxCommentLen)+" characters)")
		return
	}
	albumID := r.PathValue("id")

	// A reply must hang off a live top-level comment on this same album —
	// otherwise threads could nest without limit or cross albums.
	if body.ParentID != nil {
		var parentOf *int64
		var parentAlbum string
		err := s.db.QueryRow(r.Context(),
			"SELECT parent_id, album_id FROM comments WHERE id=$1 AND deleted_at IS NULL",
			*body.ParentID).Scan(&parentOf, &parentAlbum)
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, 404, "parent comment not found")
			return
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if parentAlbum != albumID {
			writeErr(w, 400, "parent comment belongs to another album")
			return
		}
		if parentOf != nil {
			writeErr(w, 400, "replies are only one level deep")
			return
		}
	}

	var c Comment
	err := s.db.QueryRow(r.Context(), `
		INSERT INTO comments(album_id, user_id, parent_id, body) VALUES($1,$2,$3,$4)
		RETURNING id, parent_id, user_id,
		          (SELECT nickname FROM users WHERE id=$2),
		          (SELECT `+avatarURLExpr+` FROM users WHERE id=$2), body,
		          to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), false, false`,
		albumID, currentUser(r).ID, body.ParentID, text).
		Scan(&c.ID, &c.ParentID, &c.UserID, &c.Nickname, &c.AvatarURL, &c.Body, &c.CreatedAt, &c.Deleted, &c.Edited)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, c)
}

// updateComment rewrites a comment's body. Author only — an admin can remove a
// comment but must not be able to put words in someone's mouth. edited_at marks
// it so readers can tell the text changed after the fact.
func (s *server) updateComment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid comment id")
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &body) {
		return
	}
	text := strings.TrimSpace(body.Body)
	if text == "" {
		writeErr(w, 400, "body is required")
		return
	}
	if len([]rune(text)) > maxCommentLen {
		writeErr(w, 400, "body is too long (max "+strconv.Itoa(maxCommentLen)+" characters)")
		return
	}

	var c Comment
	err = s.db.QueryRow(r.Context(), `
		UPDATE comments SET body=$1, edited_at=now()
		WHERE id=$2 AND user_id=$3 AND deleted_at IS NULL
		RETURNING id, parent_id, user_id,
		          (SELECT nickname FROM users WHERE id=$3),
		          (SELECT `+avatarURLExpr+` FROM users WHERE id=$3), body,
		          to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), false, true`,
		text, id, currentUser(r).ID).
		Scan(&c.ID, &c.ParentID, &c.UserID, &c.Nickname, &c.AvatarURL, &c.Body, &c.CreatedAt, &c.Deleted, &c.Edited)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 404, "comment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, c)
}

// deleteComment soft-deletes so replies written by other users survive; the row
// is only hidden once nothing live hangs off it (see listComments).
func (s *server) deleteComment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid comment id")
		return
	}
	u := currentUser(r)
	sql := "UPDATE comments SET deleted_at=now() WHERE id=$1 AND deleted_at IS NULL"
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
		writeErr(w, 404, "comment not found")
		return
	}
	w.WriteHeader(204)
}

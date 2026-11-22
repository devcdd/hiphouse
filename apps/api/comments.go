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

const (
	commentPageSize   = 10
	commentPageMax    = 50
	replyPreviewCount = 3 // 스레드마다 첫 페이지에 같이 싣는 답글 수. 나머지는 listReplies로 더 불러온다.
)

const commentCols = `c.id, c.parent_id, COALESCE(c.user_id, '') AS user_id, COALESCE(u.nickname, '') AS nickname,
	(SELECT ` + avatarURLExpr + ` FROM users au WHERE au.id = c.user_id) AS avatar_url,
	CASE WHEN c.deleted_at IS NULL THEN c.body ELSE '' END AS body,
	to_char(c.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
	(c.deleted_at IS NOT NULL) AS deleted,
	(c.edited_at IS NOT NULL) AS edited`

// commentThread는 최상위 댓글 하나와 앞쪽 답글 미리보기다. ReplyCount는 살아 있는 답글 전체 수라서
// Replies보다 크면 화면에 답글 더 보기 버튼이 붙는다.
type commentThread struct {
	Comment
	ReplyCount int       `json:"reply_count" db:"reply_count"`
	Replies    []Comment `json:"replies" db:"-"`
}

type commentPage struct {
	Threads    []commentThread `json:"threads"`
	NextCursor *int64          `json:"next_cursor"`
	Total      int             `json:"total"` // 이 앨범의 살아 있는 댓글 수(답글 포함)
}

type replyPage struct {
	Replies    []Comment `json:"replies"`
	NextCursor *int64    `json:"next_cursor"`
}

func commentLimit(r *http.Request) int {
	n := queryInt(r, "limit", commentPageSize)
	if n < 1 || n > commentPageMax {
		return commentPageSize
	}
	return n
}

// listComments는 공개 API다. 최상위 댓글을 최신순으로 ?before=<id> 커서마다 limit개씩 주고,
// 각 스레드에 오래된 순 답글 replyPreviewCount개를 붙인다. 삭제된 댓글은 살아 있는 답글이
// 있을 때만 자리 표시로 남는다. id는 단조 증가라 id 순서가 작성 순서다.
func (s *server) listComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	albumID := r.PathValue("id")
	limit := commentLimit(r)
	rows, err := s.db.Query(ctx, `
		SELECT `+commentCols+`,
		       (SELECT count(*) FROM comments rp WHERE rp.parent_id = c.id AND rp.deleted_at IS NULL)::int AS reply_count
		FROM comments c LEFT JOIN users u ON u.id = c.user_id
		WHERE c.album_id = $1 AND c.parent_id IS NULL
		  AND ($2::bigint IS NULL OR c.id < $2)
		  AND (c.deleted_at IS NULL
		       OR EXISTS (SELECT 1 FROM comments rp WHERE rp.parent_id = c.id AND rp.deleted_at IS NULL))
		ORDER BY c.id DESC
		LIMIT $3`, albumID, queryInt64Ptr(r, "before"), limit+1)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	threads, err := pgx.CollectRows(rows, pgx.RowToStructByName[commentThread])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	page := commentPage{Threads: []commentThread{}}
	if len(threads) > limit {
		threads = threads[:limit]
		page.NextCursor = &threads[limit-1].ID
	}

	ids := make([]int64, len(threads))
	byID := make(map[int64]*commentThread, len(threads))
	for i := range threads {
		threads[i].Replies = []Comment{}
		ids[i] = threads[i].ID
		byID[threads[i].ID] = &threads[i]
	}
	if len(ids) > 0 {
		rows, err = s.db.Query(ctx, `
			SELECT `+commentCols+`
			FROM (SELECT *, row_number() OVER (PARTITION BY parent_id ORDER BY id) AS rn
			      FROM comments WHERE parent_id = ANY($1) AND deleted_at IS NULL) c
			LEFT JOIN users u ON u.id = c.user_id
			WHERE c.rn <= $2
			ORDER BY c.parent_id, c.id`, ids, replyPreviewCount)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		replies, err := pgx.CollectRows(rows, pgx.RowToStructByName[Comment])
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		for _, rp := range replies {
			if t := byID[*rp.ParentID]; t != nil {
				t.Replies = append(t.Replies, rp)
			}
		}
	}
	page.Threads = append(page.Threads, threads...)

	if err := s.db.QueryRow(ctx,
		"SELECT count(*) FROM comments WHERE album_id = $1 AND deleted_at IS NULL", albumID).Scan(&page.Total); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, page)
}

// listReplies는 공개 API다. 한 스레드의 살아 있는 답글을 오래된 순으로 ?after=<id> 뒤부터 limit개씩 준다.
// 첫 요청의 after는 listComments가 준 미리보기의 마지막 답글 id다.
func (s *server) listReplies(w http.ResponseWriter, r *http.Request) {
	parentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid comment id")
		return
	}
	limit := commentLimit(r)
	rows, err := s.db.Query(r.Context(), `
		SELECT `+commentCols+`
		FROM comments c LEFT JOIN users u ON u.id = c.user_id
		WHERE c.parent_id = $1 AND c.deleted_at IS NULL
		  AND ($2::bigint IS NULL OR c.id > $2)
		ORDER BY c.id
		LIMIT $3`, parentID, queryInt64Ptr(r, "after"), limit+1)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	replies, err := pgx.CollectRows(rows, pgx.RowToStructByName[Comment])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	page := replyPage{Replies: []Comment{}}
	if len(replies) > limit {
		replies = replies[:limit]
		page.NextCursor = &replies[limit-1].ID
	}
	page.Replies = append(page.Replies, replies...)
	writeJSON(w, 200, page)
}

func queryInt64Ptr(r *http.Request, key string) *int64 {
	if n, err := strconv.ParseInt(r.URL.Query().Get(key), 10, 64); err == nil {
		return &n
	}
	return nil
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

package main

// 프로필 이미지. 리사이즈·WebP 변환은 브라우저가 하고 서버는 검사만 한다.
// 서버 메모리가 작아 큰 원본을 디코딩하면 같은 머신의 다른 서비스까지 흔들리고,
// Go 표준 라이브러리에는 WebP 인코더가 없다.
//
// 바이트는 users가 아니라 user_avatars에 둔다. userCols를 읽는 모든 조회에 이미지가
// 딸려 오지 않게 하려는 것이다. 클라이언트는 avatar_url만 알면 되므로, 나중에 S3로
// 옮겨도 getAvatar를 리다이렉트로 바꾸면 끝난다.

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	_ "golang.org/x/image/webp"
)

const (
	// 브라우저가 256×256 WebP로 줄이면 20~30KB라 넉넉하게 잡은 값이다.
	avatarMaxBytes = 256 << 10
	avatarMaxSide  = 1024
)

// avatarURLExpr는 이미지가 있을 때만 경로를 만든다. v에 밀리초를 넣어 이미지를 바꿀 때마다
// URL이 달라지므로 getAvatar가 immutable 캐시를 걸 수 있다.
const avatarURLExpr = `CASE WHEN avatar_updated_at IS NULL THEN NULL
	ELSE '/users/' || id || '/avatar?v=' || (extract(epoch FROM avatar_updated_at) * 1000)::bigint END AS avatar_url`

// validateAvatar는 실제 이미지인지와 형식·크기만 본다. 픽셀 전체를 디코딩하지 않는다.
// 반환값은 저장할 Content-Type이다.
func validateAvatar(data []byte) (string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", errors.New("이미지 파일이 아닙니다")
	}
	switch format {
	case "webp", "jpeg", "png":
	default:
		return "", errors.New("WebP, JPEG, PNG 이미지만 올릴 수 있습니다")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > avatarMaxSide || cfg.Height > avatarMaxSide {
		return "", fmt.Errorf("이미지는 가로·세로 %dpx 이하여야 합니다", avatarMaxSide)
	}
	return "image/" + format, nil
}

// PUT /me/avatar — 본문은 이미지 바이트 그대로. 성공하면 갱신된 User를 돌려준다.
func (s *server) putAvatar(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, avatarMaxBytes))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeErr(w, 413, fmt.Sprintf("이미지는 %dKB 이하여야 합니다", avatarMaxBytes>>10))
		return
	}
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	contentType, err := validateAvatar(data)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}

	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback(ctx)
	id := currentUser(r).ID
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_avatars(user_id, data, content_type) VALUES($1,$2,$3)
		ON CONFLICT(user_id) DO UPDATE SET data=EXCLUDED.data, content_type=EXCLUDED.content_type`,
		id, data, contentType); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var u User
	if err := tx.QueryRow(ctx, "UPDATE users SET avatar_updated_at=now() WHERE id=$1 RETURNING "+userCols, id).
		Scan(u.scanDest()...); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, u)
}

// DELETE /me/avatar — 이미지를 지우고 갱신된 User를 돌려준다. 이미지가 없어도 200이다.
func (s *server) deleteAvatar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback(ctx)
	id := currentUser(r).ID
	if _, err := tx.Exec(ctx, "DELETE FROM user_avatars WHERE user_id=$1", id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var u User
	if err := tx.QueryRow(ctx, "UPDATE users SET avatar_updated_at=NULL WHERE id=$1 RETURNING "+userCols, id).
		Scan(u.scanDest()...); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, u)
}

// GET /users/{id}/avatar — 닉네임처럼 프로필 비공개와 상관없이 공개한다.
// 댓글 옆에 붙는 얼굴이라 프로필을 닫았다고 숨기면 남의 답글 상대가 비어 보인다.
func (s *server) getAvatar(w http.ResponseWriter, r *http.Request) {
	var (
		data        []byte
		contentType string
	)
	err := s.db.QueryRow(r.Context(), "SELECT data, content_type FROM user_avatars WHERE user_id=$1", r.PathValue("id")).
		Scan(&data, &contentType)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 404, "avatar not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Content-Length", strconv.Itoa(len(data)))
	h.Set("X-Content-Type-Options", "nosniff")
	// v가 없는 URL은 이미지가 바뀌어도 주소가 같으니 오래 캐시하면 옛 사진이 남는다.
	if r.URL.Query().Has("v") {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "public, max-age=300")
	}
	w.Write(data)
}

package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// OWASP 권고(2023) PBKDF2-HMAC-SHA256 반복 횟수. 바꿔도 저장 문자열에 횟수가 남아 기존 해시는 그대로 검증된다.
const pbkdf2Iter = 600_000

const (
	passwordMinLen = 8
	// 해시 비용이 길이에 비례하므로 상한을 둬 긴 입력으로 CPU를 묶는 요청을 막는다.
	passwordMaxLen = 128
)

// hashPassword는 "pbkdf2-sha256$<반복>$<salt>$<hash>" 형식(base64 raw)으로 돌려준다.
func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iter, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iter, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

func checkPassword(stored, pw string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

// normalizeEmail은 공백을 빼고 소문자로 맞춘다. 표시 이름이 붙은 "이름 <a@b.c>" 형식은 받지 않는다.
func normalizeEmail(raw string) (string, string) {
	e := strings.ToLower(strings.TrimSpace(raw))
	if e == "" {
		return "", "이메일을 입력해 주세요"
	}
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e || len(e) > 254 || !strings.Contains(e[strings.LastIndex(e, "@"):], ".") {
		return "", "이메일 형식이 올바르지 않습니다"
	}
	return e, ""
}

func validatePassword(pw string) string {
	switch n := utf8.RuneCountInString(pw); {
	case n < passwordMinLen:
		return fmt.Sprintf("비밀번호는 %d자 이상이어야 합니다", passwordMinLen)
	case n > passwordMaxLen:
		return fmt.Sprintf("비밀번호는 %d자 이하여야 합니다", passwordMaxLen)
	}
	return ""
}

// 카카오 회원번호(숫자)와 겹치지 않게 접두사를 붙인다.
func newEmailUserID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "e" + hex.EncodeToString(b), nil
}

// signupEmail: POST /auth/email/signup {email, password, code} -> 201 {token, refresh_token, user}.
// code는 POST /auth/email/code로 받은 인증 코드다. 닉네임은 받지 않는다.
// nickname_set=false라 카카오 첫 로그인처럼 /welcome에서 정한다.
func (s *server) signupEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !decode(w, r, &body) {
		return
	}
	email, msg := normalizeEmail(body.Email)
	if msg == "" {
		msg = validatePassword(body.Password)
	}
	code, ok := normalizeCode(body.Code)
	if msg == "" && !ok {
		msg = "인증 코드 6자리를 입력해 주세요"
	}
	if msg != "" {
		writeErr(w, 400, msg)
		return
	}

	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback(ctx)

	// FOR UPDATE로 같은 코드에 동시에 들어온 시도를 줄 세워 시도 횟수가 새지 않게 한다.
	var codeHash string
	var attempts int
	var expired bool
	err = tx.QueryRow(ctx,
		"SELECT code_hash, attempts, expires_at < now() FROM email_codes WHERE email=$1 FOR UPDATE", email).
		Scan(&codeHash, &attempts, &expired)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeErr(w, 400, "인증 코드를 먼저 받아 주세요")
		return
	case err != nil:
		writeErr(w, 500, err.Error())
		return
	case expired:
		writeErr(w, 400, "인증 코드가 만료되었습니다. 코드를 다시 받아 주세요")
		return
	case attempts >= emailCodeMaxAttempts:
		writeErr(w, 429, "인증 코드를 너무 많이 틀렸습니다. 코드를 다시 받아 주세요")
		return
	}
	if !emailCodeMatches(codeHash, email, code) {
		// 틀린 시도는 실패 응답과 별개로 남아야 하므로 바로 커밋한다.
		if _, err := tx.Exec(ctx, "UPDATE email_codes SET attempts=attempts+1 WHERE email=$1", email); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if err := tx.Commit(ctx); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeErr(w, 400, "인증 코드가 맞지 않습니다")
		return
	}

	// 해시 연산은 비싸서 코드가 맞은 뒤에만 한다.
	hash, err := hashPassword(body.Password)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	id, err := newEmailUserID()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var u User
	err = tx.QueryRow(ctx,
		"INSERT INTO users(id,email,password_hash) VALUES($1,$2,$3) RETURNING "+userCols,
		id, email, hash).
		Scan(&u.ID, &u.Nickname, &u.Role, &u.ProfilePublic, &u.NicknameSet)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		writeErr(w, 409, "이미 가입된 이메일입니다")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if _, err := tx.Exec(ctx, "DELETE FROM email_codes WHERE email=$1", email); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	token, refresh, err := s.issueSession(ctx, u)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"token": token, "refresh_token": refresh, "user": u})
}

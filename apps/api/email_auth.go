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
	"sync"
	"time"
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

// 로그인 실패 제한. 실패한 시도만 센다. 이메일 기준 제한은 남이 일부러 틀려서
// 주인을 잠글 수 있어 짧게 두고, IP 기준은 넓게 둬서 여러 계정 대입을 막는다.
const (
	loginFailEmailLimit = 10
	loginFailIPLimit    = 30
	loginFailWindow     = 15 * time.Minute
)

// 없는 이메일로 로그인해도 해시 계산을 한 번 돌려, 응답 시간으로 가입 여부가 드러나지 않게 한다.
var dummyPasswordHash = sync.OnceValue(func() string {
	h, _ := hashPassword("hiphouse-dummy-password")
	return h
})

const loginFailedMsg = "이메일 또는 비밀번호가 맞지 않습니다"

// signupEmail: POST /auth/email/signup {email, password, code} -> 201 {token, refresh_token, user}.
// code는 POST /auth/email/code(purpose=signup)로 받은 인증 코드다. 닉네임은 받지 않는다.
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
	email, code, msg := validateCodeForm(body.Email, body.Password, body.Code)
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
	if err := consumeEmailCode(ctx, tx, purposeSignup, email, code); err != nil {
		writeCodeErr(w, err)
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
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.writeSession(w, r, 201, u)
}

// loginEmail: POST /auth/email/login {email, password} -> 200 {token, refresh_token, user}.
// 없는 이메일과 틀린 비밀번호를 같은 401 문장으로 답해 가입 여부를 알려 주지 않는다.
func (s *server) loginEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	email, msg := normalizeEmail(body.Email)
	if msg != "" {
		writeErr(w, 400, msg)
		return
	}
	now := time.Now()
	ip := clientIP(r)
	if s.loginEmailFails.blocked(email, now) || s.loginIPFails.blocked(ip, now) {
		writeErr(w, 429, "로그인 시도가 너무 많습니다. 잠시 후 다시 시도해 주세요")
		return
	}
	fail := func() {
		s.loginEmailFails.record(email, now)
		s.loginIPFails.record(ip, now)
		writeErr(w, 401, loginFailedMsg)
	}
	// 가입 때 받을 수 없는 길이는 해시를 돌리지 않고 거절해, 긴 입력으로 CPU를 묶지 못하게 한다.
	if utf8.RuneCountInString(body.Password) > passwordMaxLen {
		fail()
		return
	}

	var u User
	var hash *string
	err := s.db.QueryRow(r.Context(),
		"SELECT "+userCols+", password_hash FROM users WHERE email=$1", email).
		Scan(&u.ID, &u.Nickname, &u.Role, &u.ProfilePublic, &u.NicknameSet, &hash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 500, err.Error())
		return
	}
	if err != nil || hash == nil {
		checkPassword(dummyPasswordHash(), body.Password)
		fail()
		return
	}
	if !checkPassword(*hash, body.Password) {
		fail()
		return
	}
	s.writeSession(w, r, 200, u)
}

// resetPassword: POST /auth/email/reset {email, code, password} -> 200 {token, refresh_token, user}.
// code는 POST /auth/email/code(purpose=reset)로 받은 코드다. 바꾸면 기존 refresh token을 모두 지우고
// 새 세션을 준다. 비밀번호가 새어 재설정한 경우, 다른 기기에 남은 로그인도 같이 끊어야 하기 때문이다.
func (s *server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !decode(w, r, &body) {
		return
	}
	email, code, msg := validateCodeForm(body.Email, body.Password, body.Code)
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
	if err := consumeEmailCode(ctx, tx, purposeReset, email, code); err != nil {
		writeCodeErr(w, err)
		return
	}
	hash, err := hashPassword(body.Password)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var u User
	err = tx.QueryRow(ctx,
		"UPDATE users SET password_hash=$2 WHERE email=$1 AND password_hash IS NOT NULL RETURNING "+userCols,
		email, hash).
		Scan(&u.ID, &u.Nickname, &u.Role, &u.ProfilePublic, &u.NicknameSet)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 404, "이메일로 가입된 계정이 없습니다")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 폐기 표시(revoked_at)가 아니라 지운다. 표시만 하면 다른 기기가 그 토큰을 쓰는 순간 재사용 감지가
	// 모든 세션을 끊어, 방금 재설정한 기기의 새 세션까지 날아간다.
	if _, err := tx.Exec(ctx, "DELETE FROM refresh_tokens WHERE user_id=$1", u.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 남은 로그인 실패 기록 때문에 새 비밀번호로도 바로 못 들어가는 일이 없게 지운다.
	s.loginEmailFails.reset(email)
	s.writeSession(w, r, 200, u)
}

// validateCodeForm은 가입과 재설정이 같이 받는 {email, password, code}를 검사한다.
func validateCodeForm(rawEmail, password, rawCode string) (email, code, msg string) {
	email, msg = normalizeEmail(rawEmail)
	if msg == "" {
		msg = validatePassword(password)
	}
	code, ok := normalizeCode(rawCode)
	if msg == "" && !ok {
		msg = "인증 코드 6자리를 입력해 주세요"
	}
	return email, code, msg
}

func (s *server) writeSession(w http.ResponseWriter, r *http.Request, status int, u User) {
	token, refresh, err := s.issueSession(r.Context(), u)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, status, map[string]any{"token": token, "refresh_token": refresh, "user": u})
}

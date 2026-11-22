package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"log"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	texttemplate "text/template"
	"time"

	"github.com/jackc/pgx/v5"
)

// 이메일 인증 코드: POST /auth/email/code로 6자리 코드를 메일로 보내고, 가입(signup)과
// 비밀번호 재설정(reset)이 그 코드를 확인한 뒤에만 계정을 만들거나 비밀번호를 바꾼다.

const (
	emailCodeTTL      = 10 * time.Minute
	emailCodeCooldown = 60 * time.Second
	// 6자리는 경우의 수가 100만이라 코드 하나당 시도 횟수를 묶어야 추측이 막힌다.
	emailCodeMaxAttempts = 5
	// 메일 폭탄 방지: IP 하나가 시간당 받을 수 있는 코드 수.
	emailCodeIPLimit  = 10
	emailCodeIPWindow = time.Hour
)

type codePurpose string

const (
	purposeSignup codePurpose = "signup"
	purposeReset  codePurpose = "reset"
)

// emailCodeCopy는 용도마다 다른 메일 문구다.
var emailCodeCopy = map[codePurpose]struct{ Subject, Title, Lead, Footer string }{
	purposeSignup: {
		Subject: "[HIPHOUSE] 이메일 인증 코드 ",
		Title:   "이메일 인증 코드",
		Lead:    "가입 화면에 이 코드를 입력해 주세요.",
		Footer:  "가입을 요청한 적이 없다면 이 메일은 무시하셔도 됩니다. 누군가 이 주소를 잘못 입력했을 수 있어요.",
	},
	purposeReset: {
		Subject: "[HIPHOUSE] 비밀번호 재설정 코드 ",
		Title:   "비밀번호 재설정 코드",
		Lead:    "비밀번호 재설정 화면에 이 코드를 입력해 주세요.",
		Footer:  "비밀번호 재설정을 요청한 적이 없다면 이 메일은 무시하셔도 됩니다. 비밀번호는 바뀌지 않아요.",
	},
}

func newEmailCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// 용도와 이메일을 섞어, 다른 주소나 다른 용도의 코드 해시와 맞바꿔 쓸 수 없게 한다.
func hashEmailCode(purpose codePurpose, email, code string) string {
	sum := sha256.Sum256([]byte(string(purpose) + ":" + email + ":" + code))
	return hex.EncodeToString(sum[:])
}

func emailCodeMatches(storedHash string, purpose codePurpose, email, code string) bool {
	return subtle.ConstantTimeCompare([]byte(storedHash), []byte(hashEmailCode(purpose, email, code))) == 1
}

// normalizeCode는 사용자가 붙여 넣은 "123 456" 같은 공백을 빼고 6자리 숫자인지 본다.
func normalizeCode(raw string) (string, bool) {
	c := strings.Join(strings.Fields(raw), "")
	if len(c) != 6 {
		return "", false
	}
	for _, r := range c {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return c, true
}

// windowLimiter는 키마다 window 안의 기록 시각을 들고 있다가 limit에 닿으면 막는다.
// 프로세스 메모리라 재시작하면 초기화된다. API가 한 대라 이걸로 충분하다.
type windowLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string][]time.Time
}

func newWindowLimiter(limit int, window time.Duration) *windowLimiter {
	return &windowLimiter{limit: limit, window: window, hits: map[string][]time.Time{}}
}

// allow는 막혀 있지 않으면 이번 요청을 기록하고 true를 돌려준다.
func (l *windowLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fullLocked(key, now) {
		return false
	}
	l.addLocked(key, now)
	return true
}

// blocked와 record는 실패한 시도만 세고 싶을 때(로그인) 나눠 쓴다.
func (l *windowLimiter) blocked(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.fullLocked(key, now)
}

func (l *windowLimiter) record(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fullLocked(key, now)
	l.addLocked(key, now)
}

// reset은 키의 기록을 지운다. 비밀번호를 재설정한 계정의 로그인 실패 기록을 비울 때 쓴다.
func (l *windowLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}

func (l *windowLimiter) fullLocked(key string, now time.Time) bool {
	cutoff := now.Add(-l.window)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.hits, key)
	} else {
		l.hits[key] = kept
	}
	return len(kept) >= l.limit
}

func (l *windowLimiter) addLocked(key string, now time.Time) {
	l.hits[key] = append(l.hits[key], now)
	// 한 번 쓰고 다시 오지 않는 키가 맵에 쌓이지 않게, 맵이 커지면 지난 키를 정리한다.
	if len(l.hits) > 10_000 {
		cutoff := now.Add(-l.window)
		for k, ts := range l.hits {
			if !ts[len(ts)-1].After(cutoff) {
				delete(l.hits, k)
			}
		}
	}
}

// clientIP는 nginx가 넣는 X-Real-IP를 우선한다. 로컬 개발처럼 프록시가 없으면 접속 주소를 쓴다.
func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// requestEmailCode: POST /auth/email/code {email, purpose} -> 200 {expires_in, resend_after} (초 단위).
// purpose는 "signup"(기본값) 또는 "reset". 가입은 이미 가입된 이메일이면 409, 재설정은 이메일 계정이 없으면 404.
func (s *server) requestEmailCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email   string `json:"email"`
		Purpose string `json:"purpose"`
	}
	if !decode(w, r, &body) {
		return
	}
	purpose := codePurpose(body.Purpose)
	if purpose == "" {
		purpose = purposeSignup
	}
	if _, ok := emailCodeCopy[purpose]; !ok {
		writeErr(w, 400, "purpose는 signup 또는 reset이어야 합니다")
		return
	}
	email, msg := normalizeEmail(body.Email)
	if msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if !s.codeLimiter.allow(clientIP(r), time.Now()) {
		writeErr(w, 429, "요청이 너무 많습니다. 잠시 후 다시 시도해 주세요")
		return
	}
	ctx := r.Context()
	var registered bool
	if err := s.db.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM users WHERE email=$1 AND password_hash IS NOT NULL)", email).Scan(&registered); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 가입 코드 요청이 이미 가입 여부를 알려 주므로, 재설정에서만 숨겨 봐야 얻는 게 없어 그대로 알린다.
	switch {
	case purpose == purposeSignup && registered:
		writeErr(w, 409, "이미 가입된 이메일입니다")
		return
	case purpose == purposeReset && !registered:
		writeErr(w, 404, "이메일로 가입된 계정이 없습니다")
		return
	}

	code, err := newEmailCode()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 재발송 대기 시간 확인과 저장을 한 문장으로 해서, 동시에 들어온 두 요청이 둘 다 메일을 보내지 못하게 한다.
	err = s.db.QueryRow(ctx, `
		INSERT INTO email_codes(email, purpose, code_hash, expires_at)
		VALUES($1, $2, $3, now() + $4 * interval '1 second')
		ON CONFLICT(email, purpose) DO UPDATE SET
		  code_hash=EXCLUDED.code_hash, expires_at=EXCLUDED.expires_at, attempts=0, sent_at=now()
		WHERE email_codes.sent_at < now() - $5 * interval '1 second'
		RETURNING email`,
		email, purpose, hashEmailCode(purpose, email, code),
		int(emailCodeTTL.Seconds()), int(emailCodeCooldown.Seconds())).Scan(new(string))
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 429, "인증 코드는 1분에 한 번 받을 수 있습니다")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	if err := s.sendEmailCode(ctx, purpose, email, code); err != nil {
		log.Printf("email code: %s(%s) 발송 실패: %v", email, purpose, err)
		// 보내지 못한 코드가 재발송 대기만 걸고 남지 않게 지운다.
		s.db.Exec(ctx, "DELETE FROM email_codes WHERE email=$1 AND purpose=$2", email, purpose)
		writeErr(w, 502, "인증 메일을 보내지 못했습니다. 잠시 후 다시 시도해 주세요")
		return
	}
	writeJSON(w, 200, map[string]int{
		"expires_in":   int(emailCodeTTL.Seconds()),
		"resend_after": int(emailCodeCooldown.Seconds()),
	})
}

// errCodeRejected는 코드가 틀렸거나 쓸 수 없을 때 돌려준다. status와 문장을 그대로 응답에 쓴다.
type errCodeRejected struct {
	status int
	msg    string
}

func (e errCodeRejected) Error() string { return e.msg }

// consumeEmailCode는 tx 안에서 코드를 확인하고, 맞으면 지운다. 호출한 쪽이 tx를 커밋해야 소비가 확정된다.
// 틀린 코드는 시도 횟수를 올리고 tx를 바로 커밋한다. 실패 응답과 상관없이 횟수가 남아야 추측이 막힌다.
func consumeEmailCode(ctx context.Context, tx pgx.Tx, purpose codePurpose, email, code string) error {
	var codeHash string
	var attempts int
	var expired bool
	// FOR UPDATE로 같은 코드에 동시에 들어온 시도를 줄 세워 시도 횟수가 새지 않게 한다.
	err := tx.QueryRow(ctx,
		"SELECT code_hash, attempts, expires_at < now() FROM email_codes WHERE email=$1 AND purpose=$2 FOR UPDATE",
		email, purpose).Scan(&codeHash, &attempts, &expired)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return errCodeRejected{400, "인증 코드를 먼저 받아 주세요"}
	case err != nil:
		return err
	case expired:
		return errCodeRejected{400, "인증 코드가 만료되었습니다. 코드를 다시 받아 주세요"}
	case attempts >= emailCodeMaxAttempts:
		return errCodeRejected{429, "인증 코드를 너무 많이 틀렸습니다. 코드를 다시 받아 주세요"}
	}
	if !emailCodeMatches(codeHash, purpose, email, code) {
		if _, err := tx.Exec(ctx,
			"UPDATE email_codes SET attempts=attempts+1 WHERE email=$1 AND purpose=$2", email, purpose); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return errCodeRejected{400, "인증 코드가 맞지 않습니다"}
	}
	_, err = tx.Exec(ctx, "DELETE FROM email_codes WHERE email=$1 AND purpose=$2", email, purpose)
	return err
}

// writeCodeErr는 consumeEmailCode의 오류를 응답으로 바꾼다.
func writeCodeErr(w http.ResponseWriter, err error) {
	var rej errCodeRejected
	if errors.As(err, &rej) {
		writeErr(w, rej.status, rej.msg)
		return
	}
	writeErr(w, 500, err.Error())
}

func (s *server) sendEmailCode(ctx context.Context, purpose codePurpose, email, code string) error {
	cfg, ok := loadMailConfig()
	if !ok {
		// SMTP 없이 로컬에서 흐름을 돌려 볼 수 있게 코드를 서버 로그에만 남긴다.
		log.Printf("email code: SMTP 미설정이라 메일 대신 로그로 남김 %s(%s) → %s", email, purpose, code)
		return nil
	}
	cfg.To = []string{email}
	msg, err := composeEmailCodeMail(cfg, purpose, code, time.Now())
	if err != nil {
		return err
	}
	return smtpSend(ctx, cfg, msg)
}

type emailCodeMail struct {
	Code    string
	Minutes int
	SiteURL string
	Title   string
	Lead    string
	Footer  string
}

func composeEmailCodeMail(cfg mailConfig, purpose codePurpose, code string, now time.Time) ([]byte, error) {
	c := emailCodeCopy[purpose]
	m := emailCodeMail{
		Code: code, Minutes: int(emailCodeTTL.Minutes()), SiteURL: mailOrigin(),
		Title: c.Title, Lead: c.Lead, Footer: c.Footer,
	}
	var htmlBody, textBody bytes.Buffer
	if err := emailCodeHTML.Execute(&htmlBody, m); err != nil {
		return nil, err
	}
	if err := emailCodeText.Execute(&textBody, m); err != nil {
		return nil, err
	}
	return composeMail(cfg, c.Subject+code, textBody.Bytes(), htmlBody.Bytes(), now)
}

var emailCodeText = texttemplate.Must(texttemplate.New("text").Parse(
	`HIPHOUSE {{.Title}}

{{.Code}}

{{.Lead}} 코드는 {{.Minutes}}분 동안 쓸 수 있습니다.
{{.Footer}}

{{.SiteURL}}
`))

// 알림 메일과 같은 이유로 표 레이아웃과 인라인 스타일만 쓴다.
// 메일 클라이언트는 스크립트를 지워 복사 버튼을 만들 수 없어서, 코드를 한 번에 선택되게만 한다(user-select:all).
var emailCodeHTML = template.Must(template.New("html").Parse(`<!doctype html>
<html lang="ko">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light">
<meta name="supported-color-schemes" content="light">
<title>HIPHOUSE {{.Title}}</title>
</head>
<body style="margin:0;padding:0;background:#ededed;-webkit-text-size-adjust:100%;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:#ededed;">{{.Title}} {{.Code}}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#ededed;">
<tr><td align="center" style="padding:32px 12px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:480px;background:#ffffff;border-radius:14px;overflow:hidden;font-family:-apple-system,BlinkMacSystemFont,'Apple SD Gothic Neo','Pretendard','Malgun Gothic',sans-serif;color:#111111;">

<tr><td style="background:#111111;padding:22px 28px;font-size:20px;font-weight:900;letter-spacing:0.18em;color:#ffffff;">HIPHOUSE</td></tr>

<tr><td style="padding:34px 28px 8px;">
<div style="font-size:24px;line-height:1.3;font-weight:900;letter-spacing:-0.02em;">{{.Title}}</div>
<div style="margin-top:8px;font-size:15px;line-height:1.5;color:#555555;">{{.Lead}}</div>
</td></tr>

<tr><td align="center" style="padding:20px 28px;">
<div style="display:inline-block;padding:18px 26px;border-radius:12px;background:#f2f2f2;font-family:'SF Mono',Menlo,Consolas,monospace;font-size:34px;font-weight:800;letter-spacing:0.32em;-webkit-user-select:all;user-select:all;">{{.Code}}</div>
</td></tr>

<tr><td style="padding:4px 28px 32px;font-size:14px;line-height:1.6;color:#555555;">
코드는 {{.Minutes}}분 동안 쓸 수 있습니다.
</td></tr>

<tr><td style="background:#f7f7f7;padding:20px 28px;font-size:12px;line-height:1.6;color:#8a8a8a;">
{{.Footer}}
</td></tr>

</table>
</td></tr>
</table>
</body>
</html>
`))

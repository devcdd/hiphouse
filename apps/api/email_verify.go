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

// 가입 전 이메일 인증: POST /auth/email/code로 6자리 코드를 메일로 보내고,
// POST /auth/email/signup이 그 코드를 확인한 뒤에만 계정을 만든다. 인증 안 된 계정은 생기지 않는다.

const (
	emailCodeTTL      = 10 * time.Minute
	emailCodeCooldown = 60 * time.Second
	// 6자리는 경우의 수가 100만이라 코드 하나당 시도 횟수를 묶어야 추측이 막힌다.
	emailCodeMaxAttempts = 5
	// 메일 폭탄 방지: IP 하나가 시간당 받을 수 있는 코드 수.
	emailCodeIPLimit  = 10
	emailCodeIPWindow = time.Hour
)

func newEmailCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// 이메일을 섞어 다른 주소의 코드 해시와 맞바꿔 쓸 수 없게 한다.
func hashEmailCode(email, code string) string {
	sum := sha256.Sum256([]byte(email + ":" + code))
	return hex.EncodeToString(sum[:])
}

func emailCodeMatches(storedHash, email, code string) bool {
	return subtle.ConstantTimeCompare([]byte(storedHash), []byte(hashEmailCode(email, code))) == 1
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

// windowLimiter는 키마다 window 안의 요청 시각을 들고 있다가 limit을 넘으면 거절한다.
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

func (l *windowLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.limit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	// 한 번 쓰고 다시 오지 않는 IP가 맵에 쌓이지 않게, 맵이 커지면 지난 키를 정리한다.
	if len(l.hits) > 10_000 {
		for k, ts := range l.hits {
			if len(ts) == 0 || !ts[len(ts)-1].After(cutoff) {
				delete(l.hits, k)
			}
		}
	}
	return true
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

// requestEmailCode: POST /auth/email/code {email} -> 200 {expires_in, resend_after} (초 단위).
func (s *server) requestEmailCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
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
	var taken bool
	if err := s.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)", email).Scan(&taken); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if taken {
		writeErr(w, 409, "이미 가입된 이메일입니다")
		return
	}

	code, err := newEmailCode()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 재발송 대기 시간 확인과 저장을 한 문장으로 해서, 동시에 들어온 두 요청이 둘 다 메일을 보내지 못하게 한다.
	err = s.db.QueryRow(ctx, `
		INSERT INTO email_codes(email, code_hash, expires_at) VALUES($1, $2, now() + $3 * interval '1 second')
		ON CONFLICT(email) DO UPDATE SET
		  code_hash=EXCLUDED.code_hash, expires_at=EXCLUDED.expires_at, attempts=0, sent_at=now()
		WHERE email_codes.sent_at < now() - $4 * interval '1 second'
		RETURNING email`,
		email, hashEmailCode(email, code), int(emailCodeTTL.Seconds()), int(emailCodeCooldown.Seconds())).Scan(new(string))
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 429, "인증 코드는 1분에 한 번 받을 수 있습니다")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	if err := s.sendEmailCode(ctx, email, code); err != nil {
		log.Printf("email code: %s 발송 실패: %v", email, err)
		// 보내지 못한 코드가 재발송 대기만 걸고 남지 않게 지운다.
		s.db.Exec(ctx, "DELETE FROM email_codes WHERE email=$1", email)
		writeErr(w, 502, "인증 메일을 보내지 못했습니다. 잠시 후 다시 시도해 주세요")
		return
	}
	writeJSON(w, 200, map[string]int{
		"expires_in":   int(emailCodeTTL.Seconds()),
		"resend_after": int(emailCodeCooldown.Seconds()),
	})
}

func (s *server) sendEmailCode(ctx context.Context, email, code string) error {
	cfg, ok := loadMailConfig()
	if !ok {
		// SMTP 없이 로컬에서 가입 흐름을 돌려 볼 수 있게 코드를 서버 로그에만 남긴다.
		log.Printf("email code: SMTP 미설정이라 메일 대신 로그로 남김 %s → %s", email, code)
		return nil
	}
	cfg.To = []string{email}
	msg, err := composeEmailCodeMail(cfg, code, time.Now())
	if err != nil {
		return err
	}
	return smtpSend(ctx, cfg, msg)
}

type emailCodeMail struct {
	Code    string
	Minutes int
	SiteURL string
}

func composeEmailCodeMail(cfg mailConfig, code string, now time.Time) ([]byte, error) {
	m := emailCodeMail{Code: code, Minutes: int(emailCodeTTL.Minutes()), SiteURL: mailOrigin()}
	var htmlBody, textBody bytes.Buffer
	if err := emailCodeHTML.Execute(&htmlBody, m); err != nil {
		return nil, err
	}
	if err := emailCodeText.Execute(&textBody, m); err != nil {
		return nil, err
	}
	return composeMail(cfg, "[HIPHOUSE] 이메일 인증 코드 "+code, textBody.Bytes(), htmlBody.Bytes(), now)
}

var emailCodeText = texttemplate.Must(texttemplate.New("text").Parse(
	`HIPHOUSE 이메일 인증 코드

{{.Code}}

가입 화면에 위 코드를 입력해 주세요. 코드는 {{.Minutes}}분 동안 쓸 수 있습니다.
가입을 요청한 적이 없다면 이 메일은 무시하셔도 됩니다.

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
<title>HIPHOUSE 이메일 인증 코드</title>
</head>
<body style="margin:0;padding:0;background:#ededed;-webkit-text-size-adjust:100%;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:#ededed;">인증 코드 {{.Code}}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#ededed;">
<tr><td align="center" style="padding:32px 12px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:480px;background:#ffffff;border-radius:14px;overflow:hidden;font-family:-apple-system,BlinkMacSystemFont,'Apple SD Gothic Neo','Pretendard','Malgun Gothic',sans-serif;color:#111111;">

<tr><td style="background:#111111;padding:22px 28px;font-size:20px;font-weight:900;letter-spacing:0.18em;color:#ffffff;">HIPHOUSE</td></tr>

<tr><td style="padding:34px 28px 8px;">
<div style="font-size:24px;line-height:1.3;font-weight:900;letter-spacing:-0.02em;">이메일 인증 코드</div>
<div style="margin-top:8px;font-size:15px;line-height:1.5;color:#555555;">가입 화면에 아래 코드를 입력해 주세요.</div>
</td></tr>

<tr><td align="center" style="padding:20px 28px;">
<div style="display:inline-block;padding:18px 26px;border-radius:12px;background:#f2f2f2;font-family:'SF Mono',Menlo,Consolas,monospace;font-size:34px;font-weight:800;letter-spacing:0.32em;-webkit-user-select:all;user-select:all;">{{.Code}}</div>
</td></tr>

<tr><td style="padding:4px 28px 32px;font-size:14px;line-height:1.6;color:#555555;">
코드는 {{.Minutes}}분 동안 쓸 수 있습니다.
</td></tr>

<tr><td style="background:#f7f7f7;padding:20px 28px;font-size:12px;line-height:1.6;color:#8a8a8a;">
가입을 요청한 적이 없다면 이 메일은 무시하셔도 됩니다. 누군가 이 주소를 잘못 입력했을 수 있어요.
</td></tr>

</table>
</td></tr>
</table>
</body>
</html>
`))

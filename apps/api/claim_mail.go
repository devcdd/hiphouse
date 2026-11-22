package main

// 아티스트 본인 인증 신청이 들어오면 NOTIFY_EMAIL_TO에 알린다. 신보 알림 메일과 같은 SMTP 설정을 쓰고,
// 설정이 없으면 아무것도 보내지 않는다.

import (
	"bytes"
	"context"
	"html/template"
	"log"
	texttemplate "text/template"
	"time"
)

type claimMail struct {
	Nickname   string
	UserID     string
	ArtistName string
	ArtistID   string
	ArtistURL  string
	Note       string
	AdminURL   string
	SentAt     string
	Subject    string
}

func buildClaimMail(c ArtistClaim, origin string, now time.Time) claimMail {
	name := c.ArtistID
	if c.ArtistDisplayName != nil && *c.ArtistDisplayName != "" {
		name = *c.ArtistDisplayName
	} else if c.ArtistName != nil && *c.ArtistName != "" {
		name = *c.ArtistName
	}
	nick := c.Nickname
	if nick == "" {
		nick = c.UserID
	}
	return claimMail{
		Nickname:   nick,
		UserID:     c.UserID,
		ArtistName: name,
		ArtistID:   c.ArtistID,
		ArtistURL:  origin + "/artists/" + c.ArtistID,
		Note:       c.Note,
		AdminURL:   origin + "/admin?tab=claims",
		SentAt:     now.Format("2006-01-02 15:04"),
		Subject:    "[힙집] 아티스트 인증 요청: " + nick + " → " + name,
	}
}

func composeClaimMail(cfg mailConfig, m claimMail, now time.Time) ([]byte, error) {
	var htmlBody, textBody bytes.Buffer
	if err := claimHTML.Execute(&htmlBody, m); err != nil {
		return nil, err
	}
	if err := claimText.Execute(&textBody, m); err != nil {
		return nil, err
	}
	return composeMail(cfg, m.Subject, textBody.Bytes(), htmlBody.Bytes(), now)
}

// notifyClaim은 응답을 보낸 뒤 고루틴에서 돈다. 요청 context는 응답과 함께 끝나므로 새 context를 쓰고,
// 실패해도 신청 자체에는 영향을 주지 않고 로그만 남긴다.
func (s *server) notifyClaim(userID string) {
	cfg, ok := loadMailConfig()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c, err := s.loadClaim(ctx, userID)
	if err != nil || c == nil {
		log.Printf("claim mail: 신청 조회 실패(%s): %v", userID, err)
		return
	}
	msg, err := composeClaimMail(cfg, buildClaimMail(*c, mailOrigin(), seoulNow()), time.Now())
	if err != nil {
		log.Printf("claim mail: 메일 작성 실패: %v", err)
		return
	}
	if err := smtpSend(ctx, cfg, msg); err != nil {
		log.Printf("claim mail: 발송 실패: %v", err)
		return
	}
	log.Printf("claim mail: %s 신청 알림을 %v에 보냄", userID, cfg.To)
}

var claimText = texttemplate.Must(texttemplate.New("claim").Parse(`아티스트 본인 인증 요청이 들어왔습니다.

신청자: {{.Nickname}} ({{.UserID}})
아티스트: {{.ArtistName}} {{.ArtistURL}}
메모: {{if .Note}}{{.Note}}{{else}}(없음){{end}}
신청 시각: {{.SentAt}}

검수하기: {{.AdminURL}}
`))

var claimHTML = template.Must(template.New("claim").Parse(`<!doctype html>
<html lang="ko"><body style="margin:0;padding:24px;background:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,'Apple SD Gothic Neo',sans-serif;color:#111">
<div style="max-width:520px;margin:0 auto;background:#fff;border-radius:14px;padding:24px">
<h1 style="margin:0 0 16px;font-size:18px">아티스트 본인 인증 요청</h1>
<table style="border-collapse:collapse;font-size:14px;line-height:1.6">
<tr><td style="color:#71717a;padding-right:16px;vertical-align:top">신청자</td><td>{{.Nickname}} <span style="color:#a1a1aa">({{.UserID}})</span></td></tr>
<tr><td style="color:#71717a;padding-right:16px;vertical-align:top">아티스트</td><td><a href="{{.ArtistURL}}" style="color:#111">{{.ArtistName}}</a></td></tr>
<tr><td style="color:#71717a;padding-right:16px;vertical-align:top">메모</td><td style="white-space:pre-wrap">{{if .Note}}{{.Note}}{{else}}<span style="color:#a1a1aa">없음</span>{{end}}</td></tr>
<tr><td style="color:#71717a;padding-right:16px;vertical-align:top">신청 시각</td><td>{{.SentAt}}</td></tr>
</table>
<a href="{{.AdminURL}}" style="display:inline-block;margin-top:20px;padding:10px 18px;border-radius:8px;background:#111;color:#fff;text-decoration:none;font-weight:700;font-size:14px">검수하러 가기</a>
</div>
</body></html>
`))

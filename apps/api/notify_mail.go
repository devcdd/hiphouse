package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/jackc/pgx/v5"
)

// 신보 스윕이 새로 담은 앨범을 스윕 한 번에 메일 한 통으로 알린다.
// SMTP_USER와 SMTP_PASSWORD가 없으면 아무것도 보내지 않는다.

type mailConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	To       []string
}

func loadMailConfig() (mailConfig, bool) {
	c := mailConfig{
		Host: env("SMTP_HOST", "smtp.gmail.com"),
		Port: env("SMTP_PORT", "587"),
		User: env("SMTP_USER", ""),
		// Gmail 앱 비밀번호는 4자씩 띄어 보여 주는데, 그대로 붙여 넣어도 되게 공백을 뺀다.
		Password: strings.ReplaceAll(env("SMTP_PASSWORD", ""), " ", ""),
	}
	for _, t := range strings.Split(env("NOTIFY_EMAIL_TO", c.User), ",") {
		if t = strings.TrimSpace(t); t != "" {
			c.To = append(c.To, t)
		}
	}
	return c, c.User != "" && c.Password != "" && len(c.To) > 0
}

type mailAlbum struct {
	ID          string
	Name        string
	Artists     string
	ImageURL    string
	ReleaseDate string
	TypeLabel   string
	Label       string
	URL         string
	Badge       string
}

type newAlbumsMail struct {
	Albums    []mailAlbum
	Artists   int
	SentAt    string
	SiteURL   string
	Subject   string
	Preheader string
}

func (s *server) loadMailAlbums(ctx context.Context, ids []string) ([]mailAlbum, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, COALESCE(display_name, name), COALESCE(image_url, ''), COALESCE(release_date, ''),
		       `+typeLabelExpr+`, COALESCE(label, ''),
		       COALESCE((SELECT string_agg(COALESCE(ar.display_name, ar.name), ', ' ORDER BY aa.position)
		                 FROM album_artists aa JOIN artists ar ON ar.id = aa.artist_id
		                 WHERE aa.album_id = albums.id), '')
		FROM albums
		WHERE id = ANY($1) AND deleted_at IS NULL
		ORDER BY release_date DESC NULLS LAST, name`, ids)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (mailAlbum, error) {
		var a mailAlbum
		err := r.Scan(&a.ID, &a.Name, &a.ImageURL, &a.ReleaseDate, &a.TypeLabel, &a.Label, &a.Artists)
		return a, err
	})
}

func buildNewAlbumsMail(albums []mailAlbum, origin string, now time.Time) newAlbumsMail {
	today := now.Format("2006-01-02")
	artists := map[string]bool{}
	for i := range albums {
		a := &albums[i]
		a.URL = origin + "/albums/" + a.ID
		artists[a.Artists] = true
		switch {
		case a.ReleaseDate == today:
			a.Badge = "오늘 발매"
		case len(a.ReleaseDate) == len(today) && a.ReleaseDate > today:
			a.Badge = "발매 예정"
		}
		a.ReleaseDate = strings.ReplaceAll(a.ReleaseDate, "-", ".")
	}
	subject := fmt.Sprintf("[HIPHOUSE] 새 앨범 %d장", len(albums))
	pre := ""
	if len(albums) > 0 {
		first := albums[0].Name
		if albums[0].Artists != "" {
			first = albums[0].Artists + " - " + first
		}
		pre = first
		if len(albums) > 1 {
			pre += fmt.Sprintf(" 외 %d장", len(albums)-1)
		}
		subject += ": " + pre
	}
	return newAlbumsMail{
		Albums:    albums,
		Artists:   len(artists),
		SentAt:    now.Format("2006년 1월 2일") + " (" + koWeekday[now.Weekday()] + ") " + now.Format("15:04"),
		SiteURL:   origin,
		Subject:   subject,
		Preheader: pre,
	}
}

var koWeekday = [...]string{"일", "월", "화", "수", "목", "금", "토"}

func mailOrigin() string {
	return strings.TrimRight(env("PUBLIC_ORIGIN", "https://hiphouse.kr"), "/")
}

func seoulNow() time.Time {
	if loc, err := time.LoadLocation("Asia/Seoul"); err == nil {
		return time.Now().In(loc)
	}
	return time.Now()
}

// notifyNewAlbums는 스윕에서 호출되므로 실패해도 로그만 남기고 스윕 결과에는 영향을 주지 않는다.
func (s *server) notifyNewAlbums(ctx context.Context, ids []string) {
	if len(ids) == 0 {
		return
	}
	cfg, ok := loadMailConfig()
	if !ok {
		return
	}
	albums, err := s.loadMailAlbums(ctx, ids)
	if err != nil {
		log.Printf("new album mail: 앨범 조회 실패: %v", err)
		return
	}
	if len(albums) == 0 {
		return
	}
	m := buildNewAlbumsMail(albums, mailOrigin(), seoulNow())
	if err := sendNewAlbumsMail(ctx, cfg, m); err != nil {
		log.Printf("new album mail: 발송 실패: %v", err)
		return
	}
	log.Printf("new album mail: %d장 알림을 %v에 보냄", len(albums), cfg.To)
}

// POST /admin/notify/test {limit} — 최근 발매 앨범으로 알림 메일을 미리 보낸다.
func (s *server) adminNotifyTest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Limit int `json:"limit"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	if body.Limit < 1 || body.Limit > 30 {
		body.Limit = 5
	}
	cfg, ok := loadMailConfig()
	if !ok {
		writeErr(w, 400, "SMTP_USER, SMTP_PASSWORD가 설정되지 않았습니다")
		return
	}
	rows, err := s.db.Query(r.Context(),
		"SELECT id FROM albums WHERE deleted_at IS NULL ORDER BY release_date DESC NULLS LAST, id LIMIT $1", body.Limit)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	albums, err := s.loadMailAlbums(r.Context(), ids)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	m := buildNewAlbumsMail(albums, mailOrigin(), seoulNow())
	m.Subject = "[테스트] " + m.Subject
	if err := sendNewAlbumsMail(r.Context(), cfg, m); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"sent": len(albums), "to": cfg.To})
}

func sendNewAlbumsMail(ctx context.Context, cfg mailConfig, m newAlbumsMail) error {
	msg, err := composeNewAlbumsMail(cfg, m, time.Now())
	if err != nil {
		return err
	}
	return smtpSend(ctx, cfg, msg)
}

func composeNewAlbumsMail(cfg mailConfig, m newAlbumsMail, now time.Time) ([]byte, error) {
	var htmlBody, textBody bytes.Buffer
	if err := newAlbumsHTML.Execute(&htmlBody, m); err != nil {
		return nil, err
	}
	if err := newAlbumsText.Execute(&textBody, m); err != nil {
		return nil, err
	}
	return composeMail(cfg, m.Subject, textBody.Bytes(), htmlBody.Bytes(), now)
}

// composeMail은 text/plain과 text/html을 함께 담은 multipart/alternative 메시지를 만든다.
func composeMail(cfg mailConfig, subject string, text, html []byte, now time.Time) ([]byte, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := func(k, v string) { fmt.Fprintf(&buf, "%s: %s\r\n", k, v) }
	h("From", mime.BEncoding.Encode("UTF-8", "HIPHOUSE")+" <"+cfg.User+">")
	h("To", strings.Join(cfg.To, ", "))
	h("Subject", mime.BEncoding.Encode("UTF-8", subject))
	h("Date", now.Format(time.RFC1123Z))
	h("Message-ID", "<"+randomID()+"@hiphouse.kr>")
	h("MIME-Version", "1.0")
	h("Content-Type", "multipart/alternative; boundary="+mw.Boundary())
	buf.WriteString("\r\n")

	for _, part := range []struct {
		typ  string
		body []byte
	}{{"text/plain", text}, {"text/html", html}} {
		pw, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.typ + "; charset=UTF-8"},
			"Content-Transfer-Encoding": {"base64"},
		})
		if err != nil {
			return nil, err
		}
		writeBase64Lines(pw, part.body)
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RFC 2045는 base64 본문 한 줄을 76자로 제한한다.
func writeBase64Lines(w interface{ Write([]byte) (int, error) }, b []byte) {
	enc := base64.StdEncoding.EncodeToString(b)
	for len(enc) > 76 {
		w.Write([]byte(enc[:76] + "\r\n"))
		enc = enc[76:]
	}
	w.Write([]byte(enc + "\r\n"))
}

func randomID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return strconv.FormatInt(time.Now().Unix(), 36) + "." + hex.EncodeToString(b)
}

// smtp.SendMail에는 타임아웃이 없어서, SMTP 서버가 응답을 멈추면 호출한 고루틴이 영영 묶인다.
func smtpSend(ctx context.Context, cfg mailConfig, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(cfg.Host, cfg.Port))
	if err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	conn.SetDeadline(deadline)
	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if err := c.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}
	if err := c.Auth(smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if err := c.Mail(cfg.User); err != nil {
		return err
	}
	for _, to := range cfg.To {
		if err := c.Rcpt(to); err != nil {
			return fmt.Errorf("rcpt %s: %w", to, err)
		}
	}
	wc, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return c.Quit()
}

var newAlbumsText = texttemplate.Must(texttemplate.New("text").Parse(
	`HIPHOUSE 새 앨범 {{len .Albums}}장 ({{.SentAt}})
{{range .Albums}}
- {{if .Artists}}{{.Artists}} - {{end}}{{.Name}}{{if .TypeLabel}} [{{.TypeLabel}}]{{end}}{{if .ReleaseDate}} {{.ReleaseDate}}{{end}}
  {{.URL}}
{{end}}
{{.SiteURL}}
`))

// 메일 클라이언트는 <style>과 외부 CSS를 지우거나 무시하는 곳이 많아서 표 레이아웃에 인라인 스타일만 쓴다.
var newAlbumsHTML = template.Must(template.New("html").Parse(`<!doctype html>
<html lang="ko">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light">
<meta name="supported-color-schemes" content="light">
<title>{{.Subject}}</title>
</head>
<body style="margin:0;padding:0;background:#ededed;-webkit-text-size-adjust:100%;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:#ededed;">{{.Preheader}}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#ededed;">
<tr><td align="center" style="padding:32px 12px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:600px;background:#ffffff;border-radius:14px;overflow:hidden;font-family:-apple-system,BlinkMacSystemFont,'Apple SD Gothic Neo','Pretendard','Malgun Gothic',sans-serif;color:#111111;">

<tr><td style="background:#111111;padding:22px 28px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0"><tr>
<td style="font-size:20px;font-weight:900;letter-spacing:0.18em;color:#ffffff;">HIPHOUSE</td>
<td align="right" style="font-family:'SF Mono',Menlo,Consolas,monospace;font-size:11px;letter-spacing:0.16em;color:#8a8a8a;">NEW RELEASES</td>
</tr></table>
</td></tr>

<tr><td style="padding:34px 28px 10px;">
<div style="font-family:'SF Mono',Menlo,Consolas,monospace;font-size:12px;letter-spacing:0.08em;color:#8a8a8a;">{{.SentAt}} 스윕</div>
<div style="margin-top:10px;font-size:34px;line-height:1.15;font-weight:900;letter-spacing:-0.02em;">새 앨범 {{len .Albums}}장</div>
<div style="margin-top:8px;font-size:15px;color:#555555;">아티스트 {{.Artists}}명의 새 앨범이 hiphouse에 들어왔어요.</div>
</td></tr>

<tr><td style="padding:14px 16px 6px;">
{{range .Albums}}
<a href="{{.URL}}" style="display:block;text-decoration:none;color:#111111;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="border-top:1px solid #eeeeee;">
<tr>
<td width="96" valign="top" style="padding:16px 12px;">
{{if .ImageURL}}<img src="{{.ImageURL}}" width="96" height="96" alt="" style="display:block;width:96px;height:96px;border-radius:8px;object-fit:cover;background:#f2f2f2;">{{else}}<div style="width:96px;height:96px;border-radius:8px;background:#f2f2f2;"></div>{{end}}
</td>
<td valign="middle" style="padding:16px 12px 16px 2px;">
<div style="font-size:0;line-height:0;">
{{if .TypeLabel}}<span style="display:inline-block;font-size:11px;line-height:1;font-weight:700;padding:5px 8px;border:1.5px solid #111111;border-radius:999px;margin-right:6px;">{{.TypeLabel}}</span>{{end}}
{{if .Badge}}<span style="display:inline-block;font-size:11px;line-height:1;font-weight:700;padding:6.5px 9px;background:#111111;color:#ffffff;border-radius:999px;">{{.Badge}}</span>{{end}}
</div>
<div style="margin-top:9px;font-size:18px;line-height:1.3;font-weight:800;letter-spacing:-0.01em;">{{.Name}}</div>
{{if .Artists}}<div style="margin-top:3px;font-size:14px;line-height:1.4;color:#555555;">{{.Artists}}</div>{{end}}
<div style="margin-top:8px;font-family:'SF Mono',Menlo,Consolas,monospace;font-size:12px;color:#8a8a8a;">{{.ReleaseDate}}{{if .Label}}&nbsp;&nbsp;{{.Label}}{{end}}</div>
</td>
<td width="20" valign="middle" style="padding-right:12px;font-size:20px;color:#c4c4c4;">&rsaquo;</td>
</tr>
</table>
</a>
{{end}}
</td></tr>

<tr><td align="center" style="padding:18px 28px 36px;">
<a href="{{.SiteURL}}" style="display:inline-block;background:#111111;color:#ffffff;text-decoration:none;font-size:15px;font-weight:700;padding:14px 30px;border-radius:999px;">hiphouse에서 평가하기</a>
</td></tr>

<tr><td style="background:#f7f7f7;padding:20px 28px;font-size:12px;line-height:1.6;color:#8a8a8a;">
서버 신보 스윕이 새로 담은 앨범을 모아 보냅니다. 서버 .env에서 SMTP_PASSWORD를 지우면 메일이 멈춰요.
</td></tr>

</table>
</td></tr>
</table>
</body>
</html>
`))

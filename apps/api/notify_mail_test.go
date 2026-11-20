package main

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"strings"
	"testing"
	"time"
)

func sampleMailAlbums() []mailAlbum {
	return []mailAlbum{
		{ID: "am1", Name: "오늘 나온 앨범", Artists: "빈지노", ImageURL: "https://example.com/a.jpg", ReleaseDate: "2026-11-20", TypeLabel: "정규", Label: "Hi & Lo"},
		{ID: "am2", Name: "Coming <Soon>", Artists: "pH-1, 김하온", ReleaseDate: "2026-11-27", TypeLabel: "EP"},
		{ID: "am3", Name: "옛날 싱글", ReleaseDate: "2026"},
	}
}

func TestBuildNewAlbumsMail(t *testing.T) {
	now := time.Date(2026, 11, 20, 18, 10, 0, 0, time.FixedZone("KST", 9*3600))
	m := buildNewAlbumsMail(sampleMailAlbums(), "https://hiphouse.kr", now)

	if m.Subject != "[HIPHOUSE] 새 앨범 3장: 빈지노 - 오늘 나온 앨범 외 2장" {
		t.Errorf("subject: %q", m.Subject)
	}
	if m.SentAt != "2026년 11월 20일 (금) 18:10" {
		t.Errorf("sentAt: %q", m.SentAt)
	}
	if m.Artists != 3 {
		t.Errorf("artists: %d", m.Artists)
	}
	badges := []string{m.Albums[0].Badge, m.Albums[1].Badge, m.Albums[2].Badge}
	if badges[0] != "오늘 발매" || badges[1] != "발매 예정" || badges[2] != "" {
		t.Errorf("badges: %q", badges)
	}
	if m.Albums[0].URL != "https://hiphouse.kr/albums/am1" || m.Albums[0].ReleaseDate != "2026.11.20" {
		t.Errorf("album0: %+v", m.Albums[0])
	}
}

func TestComposeNewAlbumsMail(t *testing.T) {
	now := time.Date(2026, 11, 20, 18, 10, 0, 0, time.UTC)
	cfg := mailConfig{User: "me@gmail.com", To: []string{"me@gmail.com"}}
	m := buildNewAlbumsMail(sampleMailAlbums(), "https://hiphouse.kr", now)
	raw, err := composeNewAlbumsMail(cfg, m, now)
	if err != nil {
		t.Fatal(err)
	}

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	subj, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || subj != m.Subject {
		t.Errorf("subject round-trip: %q %v", subj, err)
	}
	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		// multipart.Reader는 quoted-printable만 풀어 주므로 base64는 직접 푼다.
		b, _ := io.ReadAll(p)
		dec, err := decodeBase64Lines(b)
		if err != nil {
			t.Fatal(err)
		}
		parts[strings.Split(p.Header.Get("Content-Type"), ";")[0]] = dec
	}

	html, text := parts["text/html"], parts["text/plain"]
	if !strings.Contains(html, "Coming &lt;Soon&gt;") || !strings.Contains(html, "https://hiphouse.kr/albums/am2") {
		t.Error("html body missing escaped album name or link")
	}
	if !strings.Contains(text, "Coming <Soon>") || strings.Contains(text, "&amp;") {
		t.Errorf("text body should not be HTML-escaped:\n%s", text)
	}

	if p := os.Getenv("MAIL_PREVIEW"); p != "" {
		os.WriteFile(p, []byte(html), 0o644)
	}
}

func decodeBase64Lines(b []byte) (string, error) {
	out, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, strings.NewReader(strings.ReplaceAll(string(b), "\r\n", ""))))
	return string(out), err
}

func TestLoadMailConfig(t *testing.T) {
	t.Setenv("SMTP_USER", "me@gmail.com")
	t.Setenv("SMTP_PASSWORD", "abcd efgh ijkl mnop")
	t.Setenv("NOTIFY_EMAIL_TO", "")
	c, ok := loadMailConfig()
	if !ok || c.Password != "abcdefghijklmnop" || len(c.To) != 1 || c.To[0] != "me@gmail.com" {
		t.Errorf("config: %+v ok=%v", c, ok)
	}
	t.Setenv("SMTP_PASSWORD", "")
	if _, ok := loadMailConfig(); ok {
		t.Error("empty password should disable mail")
	}
}

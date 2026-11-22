package main

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestBuildClaimMail(t *testing.T) {
	now := time.Date(2026, 11, 22, 15, 4, 0, 0, time.UTC)
	name, display := "Beenzino", "빈지노"
	c := ArtistClaim{UserID: "5005649185", Nickname: "디디", ArtistID: "ar1", ArtistName: &name, ArtistDisplayName: &display}
	m := buildClaimMail(c, "https://hiphouse.kr", now)
	if m.ArtistName != "빈지노" || m.AdminURL != "https://hiphouse.kr/admin?tab=claims" || m.ArtistURL != "https://hiphouse.kr/artists/ar1" {
		t.Errorf("display name or links: %+v", m)
	}
	if m.Subject != "[힙집] 아티스트 인증 요청: 디디 → 빈지노" {
		t.Errorf("subject: %q", m.Subject)
	}

	c.ArtistDisplayName, c.Nickname = nil, ""
	if m := buildClaimMail(c, "https://hiphouse.kr", now); m.ArtistName != "Beenzino" || m.Nickname != "5005649185" {
		t.Errorf("fallbacks: %+v", m)
	}
}

func TestComposeClaimMail(t *testing.T) {
	now := time.Date(2026, 11, 22, 15, 4, 0, 0, time.UTC)
	cfg := mailConfig{User: "me@gmail.com", To: []string{"me@gmail.com"}}
	name := "Beenzino"
	c := ArtistClaim{UserID: "u1", Nickname: "디디", ArtistID: "ar1", ArtistName: &name, Note: "<b>본인</b> 인스타 @realbeenzino"}
	m := buildClaimMail(c, "https://hiphouse.kr", now)
	raw, err := composeClaimMail(cfg, m, now)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if subj, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject")); err != nil || subj != m.Subject {
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
		b, _ := io.ReadAll(p)
		dec, err := decodeBase64Lines(b)
		if err != nil {
			t.Fatal(err)
		}
		parts[strings.Split(p.Header.Get("Content-Type"), ";")[0]] = dec
	}
	html, text := parts["text/html"], parts["text/plain"]
	// 메모는 신청자가 쓴 글이라 HTML 본문에서는 반드시 이스케이프돼야 한다.
	if strings.Contains(html, "<b>본인</b>") || !strings.Contains(html, "&lt;b&gt;본인&lt;/b&gt;") {
		t.Errorf("note must be escaped in html:\n%s", html)
	}
	if !strings.Contains(html, "https://hiphouse.kr/admin?tab=claims") {
		t.Error("html body missing admin link")
	}
	if !strings.Contains(text, "<b>본인</b> 인스타 @realbeenzino") || !strings.Contains(text, "검수하기: https://hiphouse.kr/admin?tab=claims") {
		t.Errorf("text body:\n%s", text)
	}
}

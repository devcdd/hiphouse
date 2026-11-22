package main

import (
	"bytes"
	"io"
	"net/http/httptest"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestNewEmailCode(t *testing.T) {
	for range 200 {
		c, err := newEmailCode()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := normalizeCode(c); !ok {
			t.Fatalf("generated code %q is not 6 digits", c)
		}
	}
}

func TestNormalizeCode(t *testing.T) {
	ok := map[string]string{"123456": "123456", " 123 456 ": "123456", "000001": "000001"}
	for in, want := range ok {
		if got, valid := normalizeCode(in); !valid || got != want {
			t.Errorf("normalizeCode(%q) = (%q,%v), want %q", in, got, valid, want)
		}
	}
	for _, in := range []string{"", "12345", "1234567", "12a456", "１２３４５６"} {
		if _, valid := normalizeCode(in); valid {
			t.Errorf("normalizeCode(%q): expected invalid", in)
		}
	}
}

func TestEmailCodeMatches(t *testing.T) {
	h := hashEmailCode(purposeSignup, "a@b.com", "123456")
	if !emailCodeMatches(h, purposeSignup, "a@b.com", "123456") {
		t.Fatal("right code rejected")
	}
	if emailCodeMatches(h, purposeSignup, "a@b.com", "123457") || emailCodeMatches(h, purposeSignup, "c@d.com", "123456") {
		t.Fatal("code accepted for wrong code or other email")
	}
	if emailCodeMatches(h, purposeReset, "a@b.com", "123456") {
		t.Fatal("signup code must not work as a reset code")
	}
}

func TestWindowLimiter(t *testing.T) {
	l := newWindowLimiter(2, time.Minute)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !l.allow("ip", t0) || !l.allow("ip", t0.Add(time.Second)) {
		t.Fatal("first two requests should pass")
	}
	if l.allow("ip", t0.Add(2*time.Second)) {
		t.Fatal("third request inside window should be blocked")
	}
	if !l.allow("other", t0.Add(2*time.Second)) {
		t.Fatal("keys must be counted separately")
	}
	if !l.allow("ip", t0.Add(61*time.Second)) {
		t.Fatal("oldest hit should have left the window")
	}
}

func TestWindowLimiterFailuresOnly(t *testing.T) {
	l := newWindowLimiter(2, time.Minute)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if l.blocked("a@b.com", t0) {
		t.Fatal("blocked before any failure")
	}
	l.record("a@b.com", t0)
	if l.blocked("a@b.com", t0) {
		t.Fatal("blocked after one failure with limit 2")
	}
	l.record("a@b.com", t0)
	if !l.blocked("a@b.com", t0) {
		t.Fatal("should be blocked after reaching the limit")
	}
	l.reset("a@b.com")
	if l.blocked("a@b.com", t0) {
		t.Fatal("reset should clear failures")
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("POST", "/auth/email/code", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	if got := clientIP(r); got != "10.0.0.1" {
		t.Errorf("without proxy header = %q", got)
	}
	r.Header.Set("X-Real-IP", "203.0.113.9")
	if got := clientIP(r); got != "203.0.113.9" {
		t.Errorf("with X-Real-IP = %q", got)
	}
}

func TestComposeEmailCodeMail(t *testing.T) {
	cfg := mailConfig{User: "me@gmail.com", To: []string{"new@user.kr"}}
	raw, err := composeEmailCodeMail(cfg, purposeReset, "042917", time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if to := msg.Header.Get("To"); to != "new@user.kr" {
		t.Errorf("To = %q, want only the signing-up address", to)
	}
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		t.Fatal(err)
	}
	// 첫 파트(text/plain)의 헤더 다음 본문만 떼어 base64를 푼다.
	dec, err := decodeBase64Lines([]byte(strings.Split(strings.Split(string(body), "\r\n\r\n")[1], "\r\n--")[0]))
	if err != nil || !strings.Contains(dec, "042917") || !strings.Contains(dec, "비밀번호 재설정") {
		t.Errorf("text part should contain the code and reset wording: %q %v", dec, err)
	}
}

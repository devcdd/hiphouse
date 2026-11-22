package main

import (
	"strings"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	h, err := hashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "pbkdf2-sha256$600000$") {
		t.Fatalf("unexpected format: %s", h)
	}
	if !checkPassword(h, "correct horse") {
		t.Fatal("right password rejected")
	}
	if checkPassword(h, "correct horsE") {
		t.Fatal("wrong password accepted")
	}
	h2, _ := hashPassword("correct horse")
	if h == h2 {
		t.Fatal("salt not random: same hash twice")
	}
	for _, bad := range []string{"", "plain", "md5$1$a$b", "pbkdf2-sha256$x$a$b", "pbkdf2-sha256$0$a$b"} {
		if checkPassword(bad, "correct horse") {
			t.Fatalf("malformed hash %q accepted", bad)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	ok := map[string]string{
		"  User@Example.COM ":   "user@example.com",
		"a.b+tag@sub.domain.kr": "a.b+tag@sub.domain.kr",
	}
	for in, want := range ok {
		if got, msg := normalizeEmail(in); msg != "" || got != want {
			t.Errorf("normalizeEmail(%q) = (%q,%q), want %q", in, got, msg, want)
		}
	}
	for _, in := range []string{"", "   ", "no-at", "a@localhost", "이름 <a@b.com>", "a@b.com, c@d.com", strings.Repeat("a", 250) + "@b.com"} {
		if _, msg := normalizeEmail(in); msg == "" {
			t.Errorf("normalizeEmail(%q): expected error", in)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	cases := map[string]bool{
		"1234567":                false,
		"12345678":               true,
		"한글비밀번호여덟":               true, // 바이트가 아니라 글자 수로 센다
		strings.Repeat("a", 128): true,
		strings.Repeat("a", 129): false,
	}
	for pw, want := range cases {
		if got := validatePassword(pw) == ""; got != want {
			t.Errorf("validatePassword(len %d) ok=%v, want %v", len(pw), got, want)
		}
	}
}

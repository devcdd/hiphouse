package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOGTitle(t *testing.T) {
	ko := "언더그라운드 락스타"
	empty := ""
	cases := []struct {
		name        string
		albumName   string
		displayName *string
		artists     []string
		want        string
	}{
		{"원본명만", "6SEOUL", nil, nil, "6SEOUL | 힙집"},
		{"한글 표시 이름 우선", "UNDERGROUND ROCKSTAR", &ko, nil, "언더그라운드 락스타 | 힙집"},
		{"빈 표시 이름은 원본명", "6SEOUL", &empty, nil, "6SEOUL | 힙집"},
		{"크레딧 아티스트 나열", "6SEOUL", nil, []string{"Sik-K", "펀치넬로"}, "6SEOUL — Sik-K, 펀치넬로 | 힙집"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ogTitle(c.albumName, c.displayName, c.artists); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestOGDescription(t *testing.T) {
	y := 2024
	full := "정규"
	avg := 4.25
	cases := []struct {
		name  string
		year  *int
		label *string
		avg   *float64
		count int
		want  string
	}{
		// 4.25는 정확히 표현되는 이진수라 진짜 동점이고, Go는 짝수로 내림해 4.2가 된다
		// (웹의 toFixed는 4.3). 평균이 딱 .x5로 떨어질 때만 생기는 차이라 그대로 둔다.
		{"전부 있음", &y, &full, &avg, 17, "2024 · 정규 · ★4.2 (17명)"},
		{"평가 없음", &y, &full, nil, 0, "2024 · 정규 · 평가 없음"},
		{"연도 없음", nil, &full, nil, 0, "정규 · 평가 없음"},
		{"연도·유형 모두 없음", nil, nil, nil, 0, "평가 없음"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ogDescription(c.year, c.label, c.avg, c.count); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The canonical URL must point at the SPA the human will land on, not at the
// API host nginx proxied to — so the forwarded headers win over r.Host.
func TestSiteOrigin(t *testing.T) {
	t.Setenv("PUBLIC_ORIGIN", "")
	r := httptest.NewRequest(http.MethodGet, "http://api:8080/og/albums/x", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "hiphouse.kr")
	if got := siteOrigin(r); got != "https://hiphouse.kr" {
		t.Fatalf("forwarded headers ignored: %q", got)
	}

	t.Setenv("PUBLIC_ORIGIN", "https://forced.example/")
	if got := siteOrigin(r); got != "https://forced.example" {
		t.Fatalf("PUBLIC_ORIGIN should win and lose its trailing slash: %q", got)
	}
}

// A quote in an album name must not be able to break out of the content="…"
// attribute — html/template escapes per context, and this locks that in.
func TestOGTemplateEscapesAttributes(t *testing.T) {
	var b strings.Builder
	err := ogTemplate.Execute(&b, ogPage{
		Title:        `"><script>alert(1)</script>`,
		Description:  "설명",
		CanonicalURL: "https://hiphouse.kr/albums/x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "<script>") {
		t.Fatalf("unescaped markup leaked into the page:\n%s", b.String())
	}
}

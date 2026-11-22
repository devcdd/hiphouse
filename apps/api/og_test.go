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

func TestAlbumOGPage(t *testing.T) {
	y, typ, date, label := 2024, "정규", "2024-04-11", "AOMG"
	avg := 4.25
	page := buildAlbumOG("https://hiphouse.kr", "a1", "마니", "MARNI",
		[]ogArtist{{ID: "ar1", Name: "Loopy"}}, &y, &typ, &date, &label, &avg, 3, []string{"Intro", "MARNI"})
	var b strings.Builder
	if err := ogTemplate.Execute(&b, page); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	for _, want := range []string{
		`<title>마니 — Loopy | 힙집</title>`,
		`<link rel="canonical" href="https://hiphouse.kr/albums/a1">`,
		`<a href="https://hiphouse.kr/artists/ar1">Loopy</a>`,
		`<li>MARNI</li>`,
		`원제: MARNI`,
		`<script type="application/ld+json">{`,
		`"@type":"MusicAlbum"`,
		`"datePublished":"2024-04-11"`,
		`"ratingCount":3`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
}

// JSON-LD 안의 앨범 이름에 </script>가 들어가도 스크립트 블록을 끝내지 못해야 한다.
func TestOGJSONLDEscapesScriptClose(t *testing.T) {
	page := buildAlbumOG("https://hiphouse.kr", "a1", "</script><b>x", "</script><b>x", nil, nil, nil, nil, nil, nil, 0, nil)
	var b strings.Builder
	if err := ogTemplate.Execute(&b, page); err != nil {
		t.Fatal(err)
	}
	if strings.Count(b.String(), "</script>") != 1 {
		t.Fatalf("script block broken:\n%s", b.String())
	}
}

func TestArtistOGPage(t *testing.T) {
	y := 2024
	page := buildArtistOG("https://hiphouse.kr", "ar1", "팔로알토", "Paloalto", []string{"korean hip hop"},
		[]ogAlbumRef{{ID: "a1", Name: "Diaspora", Year: &y}})
	var b strings.Builder
	if err := ogTemplate.Execute(&b, page); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	for _, want := range []string{
		`<title>팔로알토 | 힙집</title>`,
		`팔로알토의 앨범 1장.`,
		`<a href="https://hiphouse.kr/albums/a1">Diaspora (2024)</a>`,
		`"@type":"MusicGroup"`,
		`"alternateName":"Paloalto"`,
		`content="profile"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
}

func TestBuildSitemap(t *testing.T) {
	got := string(buildSitemap("https://hiphouse.kr", []string{"/", "/albums/a&b"}))
	if !strings.HasPrefix(got, `<?xml version="1.0" encoding="UTF-8"?>`) ||
		!strings.Contains(got, "<loc>https://hiphouse.kr/</loc>") ||
		!strings.Contains(got, "<loc>https://hiphouse.kr/albums/a&amp;b</loc>") {
		t.Fatalf("sitemap:\n%s", got)
	}
}

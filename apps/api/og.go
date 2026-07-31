package main

// 링크 공유용 메타 태그 프리렌더.
//
// 웹은 nginx가 서빙하는 SPA라 index.html 하나만 돌아간다 — 카카오톡·트위터·디스코드
// 스크래퍼는 JS를 실행하지 않으므로 앨범 링크를 붙여도 커버도 제목도 안 뜬다.
// nginx가 스크래퍼 User-Agent만 여기로 넘기고(apps/web/nginx.conf), 사람은 평소대로
// SPA를 받는다. 사람이 이 URL에 직접 닿는 경우(스크래퍼 UA 흉내, 링크 검사기)를 위해
// 본문에 실제 링크를 남겨 둔다.

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"
)

// ogPage is everything the template needs, already resolved to display strings.
type ogPage struct {
	Title        string
	Description  string
	ImageURL     string
	CanonicalURL string
}

var ogTemplate = template.Must(template.New("og").Parse(`<!doctype html>
<html lang="ko">
<head>
<meta charset="utf-8">
<title>{{.Title}}</title>
<link rel="canonical" href="{{.CanonicalURL}}">
<meta name="description" content="{{.Description}}">
<meta property="og:type" content="music.album">
<meta property="og:site_name" content="힙집">
<meta property="og:title" content="{{.Title}}">
<meta property="og:description" content="{{.Description}}">
<meta property="og:url" content="{{.CanonicalURL}}">
{{if .ImageURL}}<meta property="og:image" content="{{.ImageURL}}">{{end}}
<meta name="twitter:card" content="{{if .ImageURL}}summary_large_image{{else}}summary{{end}}">
<meta name="twitter:title" content="{{.Title}}">
<meta name="twitter:description" content="{{.Description}}">
{{if .ImageURL}}<meta name="twitter:image" content="{{.ImageURL}}">{{end}}
</head>
<body>
<h1>{{.Title}}</h1>
<p>{{.Description}}</p>
<p><a href="{{.CanonicalURL}}">힙집에서 보기</a></p>
</body>
</html>
`))

// siteOrigin is the public URL the SPA is served from — what the scraper should
// store and what a human clicking the shared card should land on. PUBLIC_ORIGIN
// wins; otherwise it's rebuilt from the proxy headers nginx already sets.
func siteOrigin(r *http.Request) string {
	if o := env("PUBLIC_ORIGIN", ""); o != "" {
		return strings.TrimRight(o, "/")
	}
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

// ogTitle prefers the 한글 표시 이름 and appends the credited artists, matching
// what the SPA shows on the album page.
func ogTitle(name string, displayName *string, artists []string) string {
	title := name
	if displayName != nil && *displayName != "" {
		title = *displayName
	}
	if len(artists) > 0 {
		title += " — " + strings.Join(artists, ", ")
	}
	return title + " | 힙집"
}

// ogDescription builds "2024 · 정규 · ★4.2 (17명)". Missing pieces just drop out;
// an album nobody rated says so rather than showing a fake zero.
func ogDescription(year *int, typeLabel *string, ratingAvg *float64, ratingCount int) string {
	var facts []string
	if year != nil {
		facts = append(facts, strconv.Itoa(*year))
	}
	if typeLabel != nil && *typeLabel != "" {
		facts = append(facts, *typeLabel)
	}
	if ratingAvg != nil {
		facts = append(facts, "★"+strconv.FormatFloat(*ratingAvg, 'f', 1, 64)+" ("+strconv.Itoa(ratingCount)+"명)")
	} else {
		facts = append(facts, "평가 없음")
	}
	return strings.Join(facts, " · ")
}

// GET /og/albums/{id} — meta-tag-only HTML for link scrapers.
func (s *server) ogAlbum(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var (
		name        string
		displayName *string
		year        *int
		typeLabel   *string
		imageURL    *string
		ratingAvg   *float64
		ratingCount int
		artists     []string
	)
	err := s.db.QueryRow(r.Context(), `
		SELECT albums.name, albums.display_name, albums.year,
		       CASE WHEN album_type='album' THEN '정규'
		            WHEN album_type='single' AND total_tracks >= 3 THEN 'EP'
		            WHEN album_type='single' THEN '싱글'
		            ELSE album_type END,
		       albums.image_url, `+ratingAvgExpr+`, albums.rating_count,
		       COALESCE((SELECT array_agg(COALESCE(ar.display_name, ar.name) ORDER BY aa.position)
		                 FROM album_artists aa JOIN artists ar ON ar.id = aa.artist_id
		                 WHERE aa.album_id = albums.id), '{}')
		FROM albums WHERE albums.id = $1 AND albums.deleted_at IS NULL`, id).
		Scan(&name, &displayName, &year, &typeLabel, &imageURL, &ratingAvg, &ratingCount, &artists)
	if err != nil {
		// 없는/삭제된 앨범은 스크래퍼에게도 404 — 빈 카드를 만들어 두면 공유된 링크가
		// 계속 살아 있는 것처럼 보인다.
		http.NotFound(w, r)
		return
	}

	page := ogPage{
		Title:        ogTitle(name, displayName, artists),
		Description:  ogDescription(year, typeLabel, ratingAvg, ratingCount),
		CanonicalURL: siteOrigin(r) + "/albums/" + id,
	}
	if imageURL != nil {
		page.ImageURL = *imageURL
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// 스크래퍼는 같은 링크를 여러 번 긁는다. 짧게 캐시해 크롤 폭주를 막되, 평점이
	// 오래 굳지 않을 정도로만.
	w.Header().Set("Cache-Control", "public, max-age=300")
	_ = ogTemplate.Execute(w, page)
}

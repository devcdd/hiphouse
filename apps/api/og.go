package main

// 링크 공유와 검색봇용 프리렌더.
//
// 웹은 nginx가 서빙하는 SPA라 index.html 하나만 돌아간다. 카카오톡·트위터·디스코드
// 스크래퍼와 JS 렌더링이 약한 검색봇(네이버 Yeti 등)은 앨범 링크를 붙여도 커버도 제목도
// 못 읽는다. nginx가 이 User-Agent들만 여기로 넘기고(apps/web/nginx.conf), 사람은 평소대로
// SPA를 받는다. 검색봇은 이 본문만 색인하므로 화면에 보이는 핵심 정보(아티스트, 발매 정보,
// 수록곡, 평점)를 본문과 JSON-LD에 함께 담는다. 화면과 다른 내용을 주면 클로킹으로 보일 수
// 있으니, 여기에는 SPA에도 실제로 보이는 정보만 넣는다.

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"
)

type ogLink struct {
	Text string
	URL  string
}

// ogPage is everything the template needs, already resolved to display strings.
type ogPage struct {
	Title        string
	Description  string
	ImageURL     string
	CanonicalURL string
	OGType       string // music.album | profile
	Heading      string
	Facts        []string
	Links        []ogLink // 아티스트 링크(앨범) 또는 앨범 링크(아티스트)
	LinksTitle   string
	Tracks       []string
	// JSONLD는 map이라 html/template가 <script type="application/ld+json"> 안에서 JSON으로 직렬화한다.
	JSONLD map[string]any
}

var ogTemplate = template.Must(template.New("og").Parse(`<!doctype html>
<html lang="ko">
<head>
<meta charset="utf-8">
<title>{{.Title}}</title>
<link rel="canonical" href="{{.CanonicalURL}}">
<meta name="description" content="{{.Description}}">
<meta property="og:type" content="{{if .OGType}}{{.OGType}}{{else}}website{{end}}">
<meta property="og:site_name" content="힙집">
<meta property="og:locale" content="ko_KR">
<meta property="og:title" content="{{.Title}}">
<meta property="og:description" content="{{.Description}}">
<meta property="og:url" content="{{.CanonicalURL}}">
{{if .ImageURL}}<meta property="og:image" content="{{.ImageURL}}">{{end}}
<meta name="twitter:card" content="{{if .ImageURL}}summary_large_image{{else}}summary{{end}}">
<meta name="twitter:title" content="{{.Title}}">
<meta name="twitter:description" content="{{.Description}}">
{{if .ImageURL}}<meta name="twitter:image" content="{{.ImageURL}}">{{end}}
{{if .JSONLD}}<script type="application/ld+json">{{.JSONLD}}</script>{{end}}
</head>
<body>
<h1>{{if .Heading}}{{.Heading}}{{else}}{{.Title}}{{end}}</h1>
{{if .ImageURL}}<img src="{{.ImageURL}}" alt="{{if .Heading}}{{.Heading}}{{else}}{{.Title}}{{end}}" width="300" height="300">{{end}}
<p>{{.Description}}</p>
{{range .Facts}}<p>{{.}}</p>
{{end}}{{if .Links}}<h2>{{.LinksTitle}}</h2>
<ul>{{range .Links}}<li><a href="{{.URL}}">{{.Text}}</a></li>{{end}}</ul>
{{end}}{{if .Tracks}}<h2>수록곡</h2>
<ol>{{range .Tracks}}<li>{{.}}</li>{{end}}</ol>
{{end}}<p><a href="{{.CanonicalURL}}">힙집에서 보기</a></p>
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

func pick(display *string, name string) string {
	if display != nil && *display != "" {
		return *display
	}
	return name
}

func writeOG(w http.ResponseWriter, page ogPage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// 스크래퍼는 같은 링크를 여러 번 긁는다. 짧게 캐시해 크롤 폭주를 막되, 평점이
	// 오래 굳지 않을 정도로만.
	w.Header().Set("Cache-Control", "public, max-age=300")
	_ = ogTemplate.Execute(w, page)
}

type ogArtist struct {
	ID   string
	Name string
}

// GET /og/albums/{id} — 앨범 상세 프리렌더.
func (s *server) ogAlbum(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var (
		name        string
		displayName *string
		year        *int
		typeLabel   *string
		imageURL    *string
		releaseDate *string
		label       *string
		ratingAvg   *float64
		ratingCount int
		artistIDs   []string
		artistNames []string
	)
	err := s.db.QueryRow(r.Context(), `
		SELECT albums.name, albums.display_name, albums.year,
		       CASE WHEN album_type='album' THEN '정규'
		            WHEN album_type='single' AND total_tracks >= 3 THEN 'EP'
		            WHEN album_type='single' THEN '싱글'
		            ELSE album_type END,
		       albums.image_url, albums.release_date, albums.label, `+ratingAvgExpr+`, albums.rating_count,
		       COALESCE((SELECT array_agg(ar.id ORDER BY aa.position)
		                 FROM album_artists aa JOIN artists ar ON ar.id = aa.artist_id
		                 WHERE aa.album_id = albums.id), '{}'),
		       COALESCE((SELECT array_agg(COALESCE(ar.display_name, ar.name) ORDER BY aa.position)
		                 FROM album_artists aa JOIN artists ar ON ar.id = aa.artist_id
		                 WHERE aa.album_id = albums.id), '{}')
		FROM albums WHERE albums.id = $1 AND albums.deleted_at IS NULL`, id).
		Scan(&name, &displayName, &year, &typeLabel, &imageURL, &releaseDate, &label, &ratingAvg, &ratingCount, &artistIDs, &artistNames)
	if err != nil {
		// 없는/삭제된 앨범은 스크래퍼에게도 404 — 빈 카드를 만들어 두면 공유된 링크가
		// 계속 살아 있는 것처럼 보인다.
		http.NotFound(w, r)
		return
	}
	rows, err := s.db.Query(r.Context(),
		"SELECT COALESCE(display_name, name) FROM tracks WHERE album_id=$1 ORDER BY disc_number, track_number", id)
	var tracks []string
	if err == nil {
		for rows.Next() {
			var t string
			if rows.Scan(&t) == nil {
				tracks = append(tracks, t)
			}
		}
		rows.Close()
	}

	origin := siteOrigin(r)
	artists := make([]ogArtist, len(artistIDs))
	for i := range artistIDs {
		artists[i] = ogArtist{ID: artistIDs[i], Name: artistNames[i]}
	}
	page := buildAlbumOG(origin, id, pick(displayName, name), name, artists, year, typeLabel, releaseDate, label, ratingAvg, ratingCount, tracks)
	if imageURL != nil {
		page.ImageURL = *imageURL
		page.JSONLD["image"] = *imageURL
	}
	writeOG(w, page)
}

// buildAlbumOG는 DB 없이 테스트할 수 있게 페이지 조립만 맡는다.
func buildAlbumOG(origin, id, title, originalName string, artists []ogArtist, year *int, typeLabel, releaseDate, label *string,
	ratingAvg *float64, ratingCount int, tracks []string) ogPage {
	names := make([]string, len(artists))
	for i, a := range artists {
		names[i] = a.Name
	}
	canonical := origin + "/albums/" + id
	page := ogPage{
		Title:        ogTitle(title, nil, names),
		Description:  ogDescription(year, typeLabel, ratingAvg, ratingCount),
		CanonicalURL: canonical,
		OGType:       "music.album",
		Heading:      title,
		LinksTitle:   "아티스트",
		Tracks:       tracks,
	}
	if title != originalName {
		page.Facts = append(page.Facts, "원제: "+originalName)
	}
	if releaseDate != nil && *releaseDate != "" {
		page.Facts = append(page.Facts, "발매일: "+*releaseDate)
	}
	if label != nil && *label != "" {
		page.Facts = append(page.Facts, "레이블: "+*label)
	}

	byArtist := make([]map[string]any, 0, len(artists))
	for _, a := range artists {
		url := origin + "/artists/" + a.ID
		page.Links = append(page.Links, ogLink{Text: a.Name, URL: url})
		byArtist = append(byArtist, map[string]any{"@type": "MusicGroup", "name": a.Name, "url": url})
	}
	ld := map[string]any{
		"@context": "https://schema.org",
		"@type":    "MusicAlbum",
		"name":     title,
		"url":      canonical,
	}
	if title != originalName {
		ld["alternateName"] = originalName
	}
	if len(byArtist) > 0 {
		ld["byArtist"] = byArtist
	}
	if releaseDate != nil && *releaseDate != "" {
		ld["datePublished"] = *releaseDate
	}
	if label != nil && *label != "" {
		ld["recordLabel"] = map[string]any{"@type": "Organization", "name": *label}
	}
	if len(tracks) > 0 {
		ld["numTracks"] = len(tracks)
		list := make([]map[string]any, len(tracks))
		for i, t := range tracks {
			list[i] = map[string]any{"@type": "MusicRecording", "name": t, "position": i + 1}
		}
		ld["track"] = list
	}
	if ratingAvg != nil && ratingCount > 0 {
		ld["aggregateRating"] = map[string]any{
			"@type":       "AggregateRating",
			"ratingValue": strconv.FormatFloat(*ratingAvg, 'f', 1, 64),
			"ratingCount": ratingCount,
			"bestRating":  5,
			"worstRating": 0.5,
		}
	}
	page.JSONLD = ld
	return page
}

// 아티스트 프리렌더에 싣는 앨범 수. 본문이 너무 길어지지 않게 최신순으로 자른다.
const ogArtistAlbumLimit = 60

// GET /og/artists/{id} — 아티스트 상세 프리렌더.
func (s *server) ogArtist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var (
		name        *string
		displayName *string
		imageURL    *string
		genres      []string
	)
	if err := s.db.QueryRow(r.Context(),
		"SELECT name, display_name, image_url, COALESCE(genres, '{}') FROM artists WHERE id=$1", id).
		Scan(&name, &displayName, &imageURL, &genres); err != nil {
		http.NotFound(w, r)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT al.id, COALESCE(al.display_name, al.name), al.year
		FROM albums al JOIN album_artists aa ON aa.album_id = al.id
		WHERE aa.artist_id = $1 AND al.deleted_at IS NULL
		ORDER BY al.release_date DESC NULLS LAST, al.id LIMIT $2`, id, ogArtistAlbumLimit)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var albums []ogAlbumRef
	for rows.Next() {
		var a ogAlbumRef
		if rows.Scan(&a.ID, &a.Name, &a.Year) == nil {
			albums = append(albums, a)
		}
	}
	rows.Close()

	n := ""
	if name != nil {
		n = *name
	}
	page := buildArtistOG(siteOrigin(r), id, pick(displayName, n), n, genres, albums)
	if imageURL != nil {
		page.ImageURL = *imageURL
		page.JSONLD["image"] = *imageURL
	}
	writeOG(w, page)
}

type ogAlbumRef struct {
	ID   string
	Name string
	Year *int
}

func buildArtistOG(origin, id, title, originalName string, genres []string, albums []ogAlbumRef) ogPage {
	canonical := origin + "/artists/" + id
	desc := title + "의 앨범"
	if len(albums) > 0 {
		desc += " " + strconv.Itoa(len(albums)) + "장"
		if len(albums) == ogArtistAlbumLimit {
			desc += " 이상"
		}
	}
	desc += ". 힙집에서 평가와 담벼락을 확인해 보세요."
	page := ogPage{
		Title:        title + " | 힙집",
		Description:  desc,
		CanonicalURL: canonical,
		OGType:       "profile",
		Heading:      title,
		LinksTitle:   "앨범",
	}
	if title != originalName && originalName != "" {
		page.Facts = append(page.Facts, "영문 이름: "+originalName)
	}
	if len(genres) > 0 {
		page.Facts = append(page.Facts, "장르: "+strings.Join(genres, ", "))
	}
	albumLD := make([]map[string]any, 0, len(albums))
	for _, a := range albums {
		url := origin + "/albums/" + a.ID
		text := a.Name
		if a.Year != nil {
			text += " (" + strconv.Itoa(*a.Year) + ")"
		}
		page.Links = append(page.Links, ogLink{Text: text, URL: url})
		albumLD = append(albumLD, map[string]any{"@type": "MusicAlbum", "name": a.Name, "url": url})
	}
	ld := map[string]any{
		"@context": "https://schema.org",
		"@type":    "MusicGroup",
		"name":     title,
		"url":      canonical,
	}
	if title != originalName && originalName != "" {
		ld["alternateName"] = originalName
	}
	if len(genres) > 0 {
		ld["genre"] = genres
	}
	if len(albumLD) > 0 {
		ld["album"] = albumLD
	}
	page.JSONLD = ld
	return page
}

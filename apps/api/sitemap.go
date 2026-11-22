package main

import (
	"bytes"
	"encoding/xml"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// 사이트맵 한 파일의 URL 상한(sitemaps.org). 넘으면 sitemap index로 나눠야 한다.
const sitemapMaxURLs = 50000

// GET /sitemap.xml — 홈, 삭제되지 않은 앨범, 그 앨범에 크레딧된 아티스트.
// 공개 프로필은 회원이 비공개로 돌릴 수 있고 검색 노출을 기대하는 페이지가 아니라서 넣지 않는다.
func (s *server) sitemap(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT '/albums/' || id FROM albums WHERE deleted_at IS NULL
		UNION ALL
		SELECT '/artists/' || ar.id FROM artists ar
		WHERE EXISTS (SELECT 1 FROM album_artists aa JOIN albums al ON al.id = aa.album_id
		              WHERE aa.artist_id = ar.id AND al.deleted_at IS NULL)
		LIMIT $1`, sitemapMaxURLs-1)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	paths, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	body := buildSitemap(siteOrigin(r), append([]string{"/"}, paths...))
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(body)
}

func buildSitemap(origin string, paths []string) []byte {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, p := range paths {
		b.WriteString("<url><loc>")
		xml.EscapeText(&b, []byte(origin+p))
		b.WriteString("</loc></url>\n")
	}
	b.WriteString("</urlset>\n")
	return b.Bytes()
}

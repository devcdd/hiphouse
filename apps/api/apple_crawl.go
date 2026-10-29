package main

// Apple Music 크롤링 — Spotify 계정이 막힌 동안의 수집 경로 (spotify.go는 그대로 둔다).
// 기존 행은 Spotify ID가 PK라, Apple에서 처음 들어오는 앨범·아티스트는 "am"+Apple id를
// PK로 쓰고 apple_id도 함께 채운다. 이미 가진 앨범은 knownAppleAlbums가 apple_id → UPC →
// (발매일, 이름) 순으로 맞춰 중복 저장을 막는다.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const amIDPrefix = "am"

// amArtURL — Spotify images[0]과 같은 640px로 아트워크 템플릿을 채운다.
func amArtURL(a amArtwork) *string {
	if a.URL == "" {
		return nil
	}
	return strPtr(strings.NewReplacer("{w}", "640", "{h}", "640").Replace(a.URL))
}

// amAlbumType은 Spotify album_type 값 체계로 옮긴다 (EP도 Spotify처럼 single).
// 실제 EP 구분은 applyAppleAlbum이 채우는 apple_type이 맡는다.
func amAlbumType(al amAlbum) string {
	if al.Attributes.IsCompilation {
		return "compilation"
	}
	if amKind(al.Attributes.Name) == "album" {
		return "album"
	}
	return "single"
}

func amPrecision(date string) *string {
	switch len(date) {
	case 4:
		return strPtr("year")
	case 7:
		return strPtr("month")
	case 10:
		return strPtr("day")
	}
	return nil
}

// amNextPath — 응답의 next("/v1/catalog/kr/…?offset=")를 amGet용 상대 경로로 바꾼다.
// next에는 limit이 빠져 있어 다시 붙인다.
func amNextPath(next string, limit int) string {
	rest, ok := strings.CutPrefix(next, "/v1/catalog/")
	if !ok {
		return ""
	}
	_, p, ok := strings.Cut(rest, "/")
	if !ok {
		return ""
	}
	if !strings.Contains(p, "limit=") {
		p += "&limit=" + strconv.Itoa(limit)
	}
	return p
}

// amArtistAlbums pages one artist view (full-albums, singles, appears-on-albums,
// compilation-albums). 최신순으로 오므로 max>0이면 첫 max장에서 멈춘다. 뷰가 비면
// Apple이 404(40403)를 주는데 빈 목록으로 본다.
func (s *server) amArtistAlbums(ctx context.Context, artistID, view string, max int) ([]amAlbum, error) {
	limit := 100
	if max > 0 && max < limit {
		limit = max
	}
	path := "artists/" + url.PathEscape(artistID) + "/view/" + view + "?" +
		url.Values{"limit": {strconv.Itoa(limit)}, "l": {"ko"}}.Encode()
	var out []amAlbum
	for path != "" {
		var page struct {
			Data []amAlbum `json:"data"`
			Next string    `json:"next"`
		}
		if err := s.amGet(ctx, path, &page); err != nil {
			var ae *amError
			if errors.As(err, &ae) && ae.Status == 404 {
				return out, nil
			}
			return nil, err
		}
		out = append(out, page.Data...)
		if max > 0 && len(out) >= max {
			return out[:max], nil
		}
		path = amNextPath(page.Next, limit)
	}
	return out, nil
}

type knownAlbum struct {
	ID      string
	Deleted bool
}

// normTitleSQL — 앨범명 비교 키. Apple은 제목에 "(feat. …)"를 붙이고 Spotify는 빼는 등
// 표기가 갈려 피처링 괄호와 공백·구두점을 지운 소문자로 맞춘다. %s는 컬럼/파라미터.
const normTitleSQL = `regexp_replace(regexp_replace(lower(%s), '\s*[(\[](feat|with|prod)[^)\]]*[)\]]', '', 'g'), '[][[:space:].,_''"!?:;()&/+＋-]', '', 'g')`

// dayDiffSQL — 두 'YYYY-MM-DD' 문자열의 일수 차. 연·월 정밀도 날짜면 NULL (비교 불가).
const dayDiffSQL = `CASE WHEN length(%[1]s) = 10 AND length(%[2]s) = 10 THEN abs(%[1]s::date - %[2]s::date) END`

// knownAppleAlbums maps each Apple album already in the DB to its row. 소프트 삭제된
// 행도 매칭해 관리자가 지운 앨범이 되살아나지 않게 한다. apple_id·PK·UPC가 우선이고,
// Apple 연결에 실패한 Spotify 행은 퍼지 매칭으로 잡는다 — 양쪽 발매일이 며칠씩 어긋나고
// 제목이 영문/한글로 갈리는 앨범이 흔해서, 같은 날 같은 제목 키이거나, 같은 아티스트
// 크레딧에 (같은 트랙 수 ±7일, 3곡 미만은 ±3일) 또는 (같은 제목 키 ±31일)이면 같은 앨범.
// 같은 아티스트 = appleArtistID("" 가능)에 연결됐거나 이름·표시 이름·연관검색어가 Apple
// artistName에 들어 있는 아티스트.
func (s *server) knownAppleAlbums(ctx context.Context, albums []amAlbum, appleArtistID string) (map[string]knownAlbum, error) {
	out := map[string]knownAlbum{}
	if len(albums) == 0 {
		return out, nil
	}
	n := len(albums)
	aids, upcs, names, dates, artists := make([]string, 0, n), make([]string, 0, n), make([]string, 0, n), make([]string, 0, n), make([]string, 0, n)
	tracks := make([]int32, 0, n)
	for _, al := range albums {
		upc := al.Attributes.UPC
		if placeholderUPC(upc) {
			upc = ""
		}
		aids = append(aids, al.ID)
		upcs = append(upcs, upc)
		names = append(names, amTitle(al.Attributes.Name))
		dates = append(dates, al.Attributes.ReleaseDate)
		artists = append(artists, strings.ToLower(al.Attributes.ArtistName))
		tracks = append(tracks, int32(al.Attributes.TrackCount))
	}
	sameTitle := fmt.Sprintf(normTitleSQL, "c.name") + " IN (" + fmt.Sprintf(normTitleSQL, "al.name") + ", " +
		fmt.Sprintf(normTitleSQL, "COALESCE(al.display_name, '')") + ")"
	days := fmt.Sprintf(dayDiffSQL, "al.release_date", "c.rdate")
	nameIn := `(length(%[1]s) >= 2 AND position(lower(%[1]s) in c.artists) > 0)`
	rows, err := s.db.Query(ctx,
		`SELECT c.aid, m.id, m.deleted_at IS NOT NULL
		 FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::int[], $6::text[]) AS c(aid, upc, name, rdate, tracks, artists)
		 CROSS JOIN LATERAL (
		   SELECT al.id, al.deleted_at FROM albums al
		   WHERE al.apple_id = c.aid OR al.id = $7 || c.aid
		      OR (c.upc <> '' AND al.upc = c.upc)
		      OR (c.rdate <> '' AND al.release_date = c.rdate AND `+sameTitle+`)
		      OR (c.rdate <> ''
		          AND ((al.total_tracks = c.tracks AND `+days+` <= CASE WHEN c.tracks >= 3 THEN 7 ELSE 3 END)
		               OR (`+days+` <= 31 AND `+sameTitle+`))
		          AND EXISTS (SELECT 1 FROM album_artists aa JOIN artists ar ON ar.id = aa.artist_id
		                      WHERE aa.album_id = al.id
		                        AND (ar.apple_id = $8 OR `+fmt.Sprintf(nameIn, "ar.name")+` OR `+fmt.Sprintf(nameIn, "ar.display_name")+`
		                             OR EXISTS (SELECT 1 FROM unnest(ar.aliases) x WHERE `+fmt.Sprintf(nameIn, "x")+`))))
		   ORDER BY (al.apple_id = c.aid) DESC NULLS LAST, al.deleted_at NULLS FIRST
		   LIMIT 1) m`,
		aids, upcs, names, dates, tracks, artists, amIDPrefix, appleArtistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var aid string
		var k knownAlbum
		if err := rows.Scan(&aid, &k.ID, &k.Deleted); err != nil {
			return nil, err
		}
		out[aid] = k
	}
	return out, rows.Err()
}

// linkArtistByDiscography — 한/영 표기 차이로 이름 매칭이 안 되는 아티스트를, Apple
// 디스코그래피의 (발매일, 트랙 수)가 겹치는 DB 앨범의 대표 크레딧(position 0)으로 찾아
// apple_id를 붙인다. minOverlap장 이상 겹쳐야 인정하고, onlyID가 있으면 그 아티스트만 본다.
// 이미 그 apple_id를 가진 아티스트가 있으면 아무것도 하지 않는다.
func (s *server) linkArtistByDiscography(ctx context.Context, appleArtistID string, albums []amAlbum, onlyID string, minOverlap int) (string, error) {
	var taken bool
	if err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM artists WHERE apple_id=$1)", appleArtistID).Scan(&taken); err != nil || taken {
		return "", err
	}
	dates := make([]string, 0, len(albums))
	tracks := make([]int32, 0, len(albums))
	for _, al := range albums {
		if al.Attributes.ReleaseDate != "" {
			dates = append(dates, al.Attributes.ReleaseDate)
			tracks = append(tracks, int32(al.Attributes.TrackCount))
		}
	}
	var id string
	var overlap int
	err := s.db.QueryRow(ctx,
		`SELECT aa.artist_id, COUNT(DISTINCT al.id)::int
		 FROM unnest($1::text[], $2::int[]) AS c(rdate, tracks)
		 JOIN albums al ON al.release_date = c.rdate AND al.total_tracks = c.tracks
		 JOIN album_artists aa ON aa.album_id = al.id AND aa.position = 0
		 JOIN artists ar ON ar.id = aa.artist_id
		 WHERE ar.apple_id IS NULL AND ($3 = '' OR ar.id = $3)
		 GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT 1`, dates, tracks, onlyID).Scan(&id, &overlap)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && overlap < minOverlap) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(ctx, "UPDATE artists SET apple_id=$2 WHERE id=$1 AND apple_id IS NULL", id, appleArtistID)
	return id, err
}

// resolveAppleArtist returns the DB artist id for an Apple artist: apple_id로 연결된
// 행 → 아직 미연결인 기존 행 중 이름·표시 이름·연관검색어가 정확히 한 명과 맞는 행
// (apple_id를 붙인다) → 없으면 "am"+id로 새로 만든다. 같은 apple_id가 둘이면 Spotify 행 우선.
func resolveAppleArtist(ctx context.Context, tx pgx.Tx, a amArtist) (string, error) {
	img := amArtURL(a.Attributes.Artwork)
	genres := amGenres(a.Attributes.GenreNames)
	newID := amIDPrefix + a.ID
	var id string
	err := tx.QueryRow(ctx,
		"SELECT id FROM artists WHERE apple_id=$1 ORDER BY id = $2, id LIMIT 1", a.ID, newID).Scan(&id)
	if err == nil {
		_, err = tx.Exec(ctx, "UPDATE artists SET image_url=COALESCE(image_url, $2) WHERE id=$1", id, img)
		return id, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if name := strings.TrimSpace(a.Attributes.Name); name != "" {
		rows, err := tx.Query(ctx,
			`SELECT id FROM artists WHERE apple_id IS NULL
			   AND (lower(name) = lower($1) OR lower(display_name) = lower($1)
			        OR EXISTS (SELECT 1 FROM unnest(aliases) x WHERE lower(x) = lower($1)))
			 LIMIT 2`, name)
		if err != nil {
			return "", err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return "", err
		}
		if len(ids) == 1 {
			_, err := tx.Exec(ctx,
				`UPDATE artists SET apple_id=$2, image_url=COALESCE(image_url, $3),
				   genres=CASE WHEN COALESCE(cardinality(genres), 0) = 0 THEN $4 ELSE genres END
				 WHERE id=$1`, ids[0], a.ID, img, genres)
			return ids[0], err
		}
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO artists(id, name, apple_id, image_url, genres) VALUES($1, $2, $3, $4, $5)
		 ON CONFLICT(id) DO UPDATE SET apple_id=EXCLUDED.apple_id, image_url=COALESCE(artists.image_url, EXCLUDED.image_url)`,
		newID, strPtr(a.Attributes.Name), a.ID, img, genres)
	return newID, err
}

type amSaved struct {
	ID      string
	Name    string
	Artists []string // 크레딧된 DB 아티스트 id
}

// storeAppleAlbum inserts one full Apple album (include=artists,tracks) as a new row
// with credits and tracks in one tx, then runs the UPC 연결과 같은 enrich
// (apple_id·레이블·장르·유형). enrich가 실패해도 행은 남고, PK가 "am"+id라
// knownAppleAlbums가 다음 번에 같은 앨범으로 알아본다.
func (s *server) storeAppleAlbum(ctx context.Context, al amAlbum) (amSaved, error) {
	id := amIDPrefix + al.ID
	name := amTitle(al.Attributes.Name)
	saved := amSaved{ID: id, Name: name}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return saved, err
	}
	defer tx.Rollback(ctx)

	date := al.Attributes.ReleaseDate
	upc := strPtr(al.Attributes.UPC)
	if placeholderUPC(al.Attributes.UPC) {
		upc = nil
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO albums(id, name, release_date, year, album_type, total_tracks, image_url, upc,
		                    release_date_precision, tracks_synced_at)
		 VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		 ON CONFLICT(id) DO NOTHING`,
		id, name, strPtr(date), yearOf(date), amAlbumType(al), al.Attributes.TrackCount,
		amArtURL(al.Attributes.Artwork), upc, amPrecision(date)); err != nil {
		return saved, err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM album_artists WHERE album_id=$1", id); err != nil {
		return saved, err
	}
	for i, a := range al.Relationships.Artists.Data {
		aid, err := resolveAppleArtist(ctx, tx, a)
		if err != nil {
			return saved, err
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO album_artists(album_id, artist_id, position) VALUES($1, $2, $3) ON CONFLICT(album_id, artist_id) DO NOTHING",
			id, aid, i); err != nil {
			return saved, err
		}
		saved.Artists = append(saved.Artists, aid)
	}
	for _, t := range al.Relationships.Tracks.Data {
		disc := t.Attributes.DiscNumber
		if disc == 0 {
			disc = 1
		}
		// Apple 트랙 크레딧은 "A, B & C" 문자열 하나라 아티스트 id 없이 통째로 담는다.
		artists := []trackArtist{}
		if t.Attributes.ArtistName != "" {
			artists = append(artists, trackArtist{Name: t.Attributes.ArtistName})
		}
		aj, err := json.Marshal(artists)
		if err != nil {
			return saved, err
		}
		var dur *int
		if t.Attributes.DurationInMillis > 0 {
			dur = &t.Attributes.DurationInMillis
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO tracks(album_id, id, disc_number, track_number, name, duration_ms, explicit, artists)
			 VALUES($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT(album_id, id) DO NOTHING`,
			id, t.ID, disc, t.Attributes.TrackNumber, t.Attributes.Name, dur,
			t.Attributes.ContentRating == "explicit", aj); err != nil {
			return saved, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return saved, err
	}
	err = s.applyAppleAlbum(ctx, appleCand{ID: id, Name: name}, &al, &appleLinkResult{})
	return saved, err
}

// addAppleAlbums fetches full objects for Apple album ids (25개씩) and stores each.
// apiErr = Apple 조회 실패 (그 앞까지 저장된 앨범은 남는다), err = DB 장애.
func (s *server) addAppleAlbums(ctx context.Context, appleIDs []string) (saved []amSaved, apiErr, err error) {
	for start := 0; start < len(appleIDs); start += amUPCBatch {
		chunk := appleIDs[start:min(start+amUPCBatch, len(appleIDs))]
		var page struct {
			Data []amAlbum `json:"data"`
		}
		q := url.Values{"ids": {strings.Join(chunk, ",")}, "include": {amInclude}, "l": {"ko"}}
		if e := s.amGet(ctx, "albums?"+q.Encode(), &page); e != nil {
			return saved, e, nil
		}
		for _, al := range page.Data {
			sv, err := s.storeAppleAlbum(ctx, al)
			if err != nil {
				return saved, nil, err
			}
			saved = append(saved, sv)
		}
	}
	return saved, nil, nil
}

// freshAppleIDs drops albums already in the DB (and duplicates across views).
func (s *server) freshAppleIDs(ctx context.Context, albums []amAlbum, appleArtistID string) ([]string, error) {
	known, err := s.knownAppleAlbums(ctx, albums, appleArtistID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, al := range albums {
		if _, ok := known[al.ID]; ok || seen[al.ID] {
			continue
		}
		seen[al.ID] = true
		out = append(out, al.ID)
	}
	return out, nil
}

func countArtists(saved []amSaved) int {
	ids := map[string]bool{}
	for _, sv := range saved {
		for _, id := range sv.Artists {
			ids[id] = true
		}
	}
	return len(ids)
}

func requireApple(w http.ResponseWriter) bool {
	if !appleConfigured() {
		writeErr(w, 400, "APPLE_MUSIC_* 환경변수가 설정되지 않았습니다")
		return false
	}
	return true
}

type appleArtistHit struct {
	ID         string   `json:"id"` // Apple 아티스트 id
	Name       string   `json:"name"`
	ImageURL   *string  `json:"image_url"`
	Genres     []string `json:"genres"`
	ArtistID   *string  `json:"artist_id"`    // 연결된 DB 아티스트 id (없으면 null)
	AlbumsInDB int      `json:"albums_in_db"` // 그 DB 아티스트의 live 앨범 수
}

// GET /admin/apple/artists?q=
func (s *server) adminAppleArtistSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, 400, "q is required")
		return
	}
	if !requireApple(w) {
		return
	}
	var page struct {
		Results struct {
			Artists struct {
				Data []amArtist `json:"data"`
			} `json:"artists"`
		} `json:"results"`
	}
	sq := url.Values{"term": {q}, "types": {"artists"}, "limit": {"25"}, "l": {"ko"}}
	if err := s.amGet(r.Context(), "search?"+sq.Encode(), &page); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	data := page.Results.Artists.Data
	ids := make([]string, 0, len(data))
	for _, a := range data {
		ids = append(ids, a.ID)
	}
	rows, err := s.db.Query(r.Context(),
		`SELECT DISTINCT ON (a.apple_id) a.apple_id, a.id,
		   (SELECT COUNT(*) FROM album_artists aa JOIN albums al ON al.id = aa.album_id
		    WHERE aa.artist_id = a.id AND al.deleted_at IS NULL)::int
		 FROM artists a WHERE a.apple_id = ANY($1)
		 ORDER BY a.apple_id, a.id = $2 || a.apple_id, a.id`, ids, amIDPrefix)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	type link struct {
		ID     string
		Albums int
	}
	links := map[string]link{}
	for rows.Next() {
		var aid string
		var l link
		if err := rows.Scan(&aid, &l.ID, &l.Albums); err != nil {
			rows.Close()
			writeErr(w, 500, err.Error())
			return
		}
		links[aid] = l
	}
	rows.Close()
	out := make([]appleArtistHit, 0, len(data))
	for _, a := range data {
		h := appleArtistHit{ID: a.ID, Name: a.Attributes.Name, ImageURL: amArtURL(a.Attributes.Artwork),
			Genres: amGenres(a.Attributes.GenreNames)}
		if l, ok := links[a.ID]; ok {
			h.ArtistID, h.AlbumsInDB = strPtr(l.ID), l.Albums
		}
		out = append(out, h)
	}
	writeJSON(w, 200, out)
}

// POST /admin/apple/crawl {apple_artist_id | artist_id, appears_on} — 아티스트의 Apple
// 디스코그래피(정규·싱글/EP, appears_on이면 참여·컴필레이션까지) 중 DB에 없는 앨범만
// 크레딧·트랙과 함께 저장한다. artist_id(DB id)로 부르면 그 아티스트의 apple_id를 쓴다.
// 앨범이 0개면 아티스트도 저장하지 않는다 (Spotify 크롤과 같은 규칙).
func (s *server) adminAppleCrawl(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AppleArtistID string `json:"apple_artist_id"`
		ArtistID      string `json:"artist_id"`
		AppearsOn     bool   `json:"appears_on"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !requireApple(w) {
		return
	}
	ctx := r.Context()
	appleID := body.AppleArtistID
	if appleID == "" && body.ArtistID != "" {
		var aid *string
		if err := s.db.QueryRow(ctx, "SELECT apple_id FROM artists WHERE id=$1", body.ArtistID).Scan(&aid); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeErr(w, 404, "아티스트를 찾을 수 없습니다")
				return
			}
			writeErr(w, 500, err.Error())
			return
		}
		if aid == nil {
			writeErr(w, 400, "Apple Music에 연결되지 않은 아티스트입니다 — 관리자 크롤링 탭에서 Apple 아티스트를 검색해 가져오세요")
			return
		}
		appleID, err := s.verifyAppleArtist(ctx, body.ArtistID, *aid)
		if err != nil {
			writeErr(w, 502, err.Error())
			return
		}
		body.AppleArtistID = appleID
	}
	appleID = body.AppleArtistID
	if appleID == "" {
		writeErr(w, 400, "apple_artist_id 또는 artist_id가 필요합니다")
		return
	}

	views := []string{"full-albums", "singles"}
	if body.AppearsOn {
		views = append(views, "appears-on-albums", "compilation-albums")
	}
	var all []amAlbum
	for _, v := range views {
		list, err := s.amArtistAlbums(ctx, appleID, v, 0)
		if err != nil {
			writeErr(w, 502, err.Error())
			return
		}
		all = append(all, list...)
	}
	if len(all) == 0 {
		writeJSON(w, 200, map[string]any{"albums": 0, "existing": 0, "saved": false})
		return
	}

	if _, err := s.linkArtistByDiscography(ctx, appleID, all, "", 3); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 크롤 대상 본인은 앨범 크레딧과 무관하게(참여작만 있어도) 행·이미지를 맞춰 둔다.
	var artist struct {
		Data []amArtist `json:"data"`
	}
	if err := s.amGet(ctx, "artists/"+url.PathEscape(appleID)+"?l=ko", &artist); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	var artistName *string
	if len(artist.Data) == 1 {
		artistName = strPtr(artist.Data[0].Attributes.Name)
		tx, err := s.db.Begin(ctx)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		_, err = resolveAppleArtist(ctx, tx, artist.Data[0])
		if err == nil {
			err = tx.Commit(ctx)
		}
		tx.Rollback(ctx)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}

	fresh, err := s.freshAppleIDs(ctx, all, appleID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	saved, apiErr, err := s.addAppleAlbums(ctx, fresh)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if apiErr != nil {
		writeErr(w, 502, fmt.Sprintf("%d개 저장 후 중단: %v", len(saved), apiErr))
		return
	}
	writeJSON(w, 200, map[string]any{
		"albums":        len(saved),
		"existing":      len(all) - len(fresh),
		"saved":         true,
		"artist_name":   artistName,
		"artists":       countArtists(saved),
		"tracks_synced": len(saved),
	})
}

type appleAlbumHit struct {
	ID          string  `json:"id"` // Apple 앨범 id
	Name        string  `json:"name"`
	ImageURL    *string `json:"image_url"`
	ReleaseDate *string `json:"release_date"`
	AlbumType   string  `json:"album_type"` // album | ep | single (Apple 등록 유형)
	TotalTracks int     `json:"total_tracks"`
	Artists     string  `json:"artists"`
	InDB        bool    `json:"in_db"`
	Deleted     bool    `json:"deleted"`
}

// GET /admin/apple/albums?q= — Apple 앨범 검색 후보에 DB 보유 여부를 붙인다.
func (s *server) adminAppleAlbumSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, 400, "q is required")
		return
	}
	if !requireApple(w) {
		return
	}
	var page struct {
		Results struct {
			Albums struct {
				Data []amAlbum `json:"data"`
			} `json:"albums"`
		} `json:"results"`
	}
	sq := url.Values{"term": {q}, "types": {"albums"}, "limit": {"25"}, "l": {"ko"}}
	if err := s.amGet(r.Context(), "search?"+sq.Encode(), &page); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	data := page.Results.Albums.Data
	known, err := s.knownAppleAlbums(r.Context(), data, "")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]appleAlbumHit, 0, len(data))
	for _, al := range data {
		k, ok := known[al.ID]
		out = append(out, appleAlbumHit{
			ID: al.ID, Name: amTitle(al.Attributes.Name), ImageURL: amArtURL(al.Attributes.Artwork),
			ReleaseDate: strPtr(al.Attributes.ReleaseDate), AlbumType: amKind(al.Attributes.Name),
			TotalTracks: al.Attributes.TrackCount, Artists: al.Attributes.ArtistName,
			InDB: ok, Deleted: ok && k.Deleted,
		})
	}
	writeJSON(w, 200, out)
}

// POST /admin/apple/crawl-album {album_id} — Apple 앨범 1개 추가. 이미 있는 앨범이면
// 저장하지 않고 그 행을 알려준다 (existing, 소프트 삭제면 deleted).
func (s *server) adminAppleCrawlAlbum(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AlbumID string `json:"album_id"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.AlbumID == "" {
		writeErr(w, 400, "album_id is required")
		return
	}
	if !requireApple(w) {
		return
	}
	ctx := r.Context()
	var page struct {
		Data []amAlbum `json:"data"`
	}
	q := url.Values{"include": {amInclude}, "l": {"ko"}}
	if err := s.amGet(ctx, "albums/"+url.PathEscape(body.AlbumID)+"?"+q.Encode(), &page); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	if len(page.Data) != 1 {
		writeErr(w, 404, "Apple Music에서 앨범을 찾을 수 없습니다")
		return
	}
	al := page.Data[0]
	var mainArtist string
	if as := al.Relationships.Artists.Data; len(as) > 0 {
		mainArtist = as[0].ID
	}
	known, err := s.knownAppleAlbums(ctx, page.Data, mainArtist)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if k, ok := known[al.ID]; ok {
		writeJSON(w, 200, map[string]any{
			"album_id": k.ID, "album_name": amTitle(al.Attributes.Name), "saved": false, "existing": true, "deleted": k.Deleted,
		})
		return
	}
	sv, err := s.storeAppleAlbum(ctx, al)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"album_id": sv.ID, "album_name": sv.Name, "saved": true, "artists": len(sv.Artists), "tracks_synced": 1,
	})
}

// verifyAppleArtist — 저장된 apple_id가 진짜 이 아티스트인지, UPC 등으로 연결된 기존 앨범의
// Apple 크레딧으로 확인한다. 예전 이름 매칭 연결이 동명이인(해외 "Iron", "MINOS")을 붙여
// 둔 경우가 있어서, 현재 id가 앨범 절반 미만에만 있고 다른 한 명이 절반 넘게 있으면 그쪽으로
// 고쳐 돌려준다. 협업 상대에게 넘어가지 않도록 동률은 그대로 두고, 이미 다른 DB 아티스트가
// 쓰는 id(비프리 ↔ 부캐 등)로는 바꾸지 않는다. 연결 앨범이 2장 미만이면 근거가 없어 그대로 둔다.
func (s *server) verifyAppleArtist(ctx context.Context, artistID, appleID string) (string, error) {
	rows, err := s.db.Query(ctx,
		`SELECT al.apple_id FROM album_artists aa JOIN albums al ON al.id = aa.album_id
		 WHERE aa.artist_id = $1 AND al.apple_id IS NOT NULL AND NOT starts_with(al.id, $2)
		 ORDER BY aa.position, al.release_date DESC NULLS LAST LIMIT $3`, artistID, amIDPrefix, amUPCBatch)
	if err != nil {
		return "", err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(ids) < 2 {
		return appleID, err
	}
	var page struct {
		Data []amAlbum `json:"data"`
	}
	q := url.Values{"ids": {strings.Join(ids, ",")}, "include": {"artists"}, "l": {"ko"}}
	if err := s.amGet(ctx, "albums?"+q.Encode(), &page); err != nil {
		return "", err
	}
	votes := map[string]int{}
	for _, al := range page.Data {
		for _, a := range al.Relationships.Artists.Data {
			votes[a.ID]++
		}
	}
	n := len(page.Data)
	best := ""
	for id, v := range votes {
		if id != appleID && v*2 > n && (best == "" || v > votes[best]) {
			best = id
		}
	}
	if best == "" || votes[appleID]*2 >= n {
		return appleID, nil
	}
	var taken bool
	if err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM artists WHERE apple_id=$1 AND id<>$2)", best, artistID).Scan(&taken); err != nil || taken {
		return appleID, err
	}
	log.Printf("apple: 아티스트 %s apple_id 교정 %s → %s (기존 앨범 크레딧 %d/%d)", artistID, appleID, best, votes[best], len(page.Data))
	_, err = s.db.Exec(ctx, "UPDATE artists SET apple_id=$2 WHERE id=$1", artistID, best)
	return best, err
}

// findAppleArtist — apple_id 없는 감시 아티스트를 Apple에 연결한다. l=ko 응답은 한글
// 표기라 영문 name만으론 안 맞는 경우가 많아 display_name·연관검색어까지 대조하고, 이름이
// 맞는 후보가 없으면 첫 검색 결과의 최신 디스코그래피가 이 아티스트의 DB 앨범과 2장 이상
// 겹칠 때만 인정한다. 연결하면 apple_id를 저장하고 그 id를, 못 찾으면 ""를 돌려준다.
func (s *server) findAppleArtist(ctx context.Context, id string, name, displayName *string, aliases []string) (string, error) {
	want := map[string]bool{}
	var terms []string
	for _, n := range []*string{displayName, name} {
		if n != nil && strings.TrimSpace(*n) != "" {
			terms = append(terms, *n)
			want[normName(*n)] = true
		}
	}
	for _, a := range aliases {
		want[normName(a)] = true
	}
	var first string
	for _, term := range terms {
		var page struct {
			Results struct {
				Artists struct {
					Data []amArtist `json:"data"`
				} `json:"artists"`
			} `json:"results"`
		}
		sq := url.Values{"term": {term}, "types": {"artists"}, "limit": {"5"}, "l": {"ko"}}
		if err := s.amGet(ctx, "search?"+sq.Encode(), &page); err != nil {
			return "", err
		}
		for _, a := range page.Results.Artists.Data {
			if want[normName(a.Attributes.Name)] {
				_, err := s.db.Exec(ctx, "UPDATE artists SET apple_id=$2 WHERE id=$1 AND apple_id IS NULL", id, a.ID)
				return a.ID, err
			}
		}
		if first == "" && len(page.Results.Artists.Data) > 0 {
			first = page.Results.Artists.Data[0].ID
		}
	}
	if first == "" {
		return "", nil
	}
	var albums []amAlbum
	for _, v := range []string{"full-albums", "singles"} {
		list, err := s.amArtistAlbums(ctx, first, v, 10)
		if err != nil {
			return "", err
		}
		albums = append(albums, list...)
	}
	linked, err := s.linkArtistByDiscography(ctx, first, albums, id, 2)
	if err != nil || linked == "" {
		return "", err
	}
	return first, nil
}

// checkAppleReleasesBatch — checkReleasesBatch의 Apple 판. 감시 아티스트를 오래 안 본
// 순서대로 limit명 골라 full-albums·singles 뷰의 최신 10장씩을 보고 DB에 없는 앨범만
// 저장한다. apple_id가 없는 아티스트는 이름 검색으로 연결을 시도하고, 못 맞추면 스탬프만
// 찍고 Unlinked로 보고한다 (아티스트 상세에서 연결이 필요). 같은 배치 계약: Apple 오류면
// 멈추고 res.Error로 보고하며 그 아티스트는 스탬프가 안 찍혀 다음 호출이 다시 집는다.
func (s *server) checkAppleReleasesBatch(ctx context.Context, limit int) (releaseCheckResult, error) {
	res := releaseCheckResult{Artists: []releaseCheckItem{}}
	if limit < 1 || limit > 50 {
		limit = 10
	}
	rows, err := s.db.Query(ctx,
		"SELECT a.id, COALESCE(a.display_name, a.name), a.name, a.display_name, COALESCE(a.aliases, '{}'), a.apple_id "+releaseStaleSQL+
			" ORDER BY a.releases_checked_at NULLS FIRST, a.id LIMIT $1", limit)
	if err != nil {
		return res, err
	}
	type candidate struct {
		ID          string
		Label       *string
		Name        *string
		DisplayName *string
		Aliases     []string
		AppleID     *string
	}
	pending, err := pgx.CollectRows(rows, pgx.RowToStructByPos[candidate])
	if err != nil {
		return res, err
	}

	label := func(c candidate) string {
		if c.Label != nil {
			return *c.Label
		}
		return c.ID
	}
	stamp := func(id string) error {
		_, err := s.db.Exec(ctx, "UPDATE artists SET releases_checked_at=now() WHERE id=$1", id)
		return err
	}

	for _, c := range pending {
		if c.AppleID == nil {
			aid, err := s.findAppleArtist(ctx, c.ID, c.Name, c.DisplayName, c.Aliases)
			if err != nil {
				res.Error = strPtr(fmt.Sprintf("%s: %v", label(c), err))
				break
			}
			if aid == "" {
				res.Unlinked = append(res.Unlinked, label(c))
				if err := stamp(c.ID); err != nil {
					return res, err
				}
				res.Checked++
				continue
			}
			c.AppleID = &aid
		} else {
			aid, err := s.verifyAppleArtist(ctx, c.ID, *c.AppleID)
			if err != nil {
				res.Error = strPtr(fmt.Sprintf("%s: %v", label(c), err))
				break
			}
			c.AppleID = &aid
		}

		var latest []amAlbum
		var apiErr error
		for _, v := range []string{"full-albums", "singles"} {
			list, err := s.amArtistAlbums(ctx, *c.AppleID, v, 10)
			if err != nil {
				apiErr = err
				break
			}
			latest = append(latest, list...)
		}
		if apiErr != nil {
			res.Error = strPtr(fmt.Sprintf("%s: %v", label(c), apiErr))
			break
		}
		fresh, err := s.freshAppleIDs(ctx, latest, *c.AppleID)
		if err != nil {
			return res, err
		}
		saved, apiErr, err := s.addAppleAlbums(ctx, fresh)
		if err != nil {
			return res, err
		}
		if len(saved) > 0 {
			item := releaseCheckItem{ID: c.ID, Name: c.Label, Albums: make([]string, 0, len(saved))}
			for _, sv := range saved {
				item.Albums = append(item.Albums, sv.Name)
			}
			res.Artists = append(res.Artists, item)
			res.NewAlbums += len(saved)
			res.TracksSynced += len(saved)
		}
		if apiErr != nil {
			res.Error = strPtr(fmt.Sprintf("%s: %v", label(c), apiErr))
			break
		}
		if err := stamp(c.ID); err != nil {
			return res, err
		}
		res.Checked++
	}

	if err := s.db.QueryRow(ctx, "SELECT COUNT(*)::int "+releaseStaleSQL).Scan(&res.Remaining); err != nil {
		return res, err
	}
	return res, nil
}

// POST /admin/apple/check-releases {limit}
func (s *server) adminAppleCheckReleases(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Limit int `json:"limit"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !requireApple(w) {
		return
	}
	res, err := s.checkAppleReleasesBatch(r.Context(), body.Limit)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// sweepAppleReleases — 서버 스윕용. 남은 대상이 0이 될 때까지 Apple 신보 배치를 반복한다.
func (s *server) sweepAppleReleases(ctx context.Context) {
	if !appleConfigured() {
		log.Printf("release sweep: APPLE_MUSIC_* 미설정 — 건너뜀")
		return
	}
	checked, albums := 0, 0
	var unlinked []string
	for {
		res, err := s.checkAppleReleasesBatch(ctx, sweepBatch)
		checked += res.Checked
		albums += res.NewAlbums
		unlinked = append(unlinked, res.Unlinked...)
		if err != nil {
			log.Printf("release sweep: DB 오류로 중단: %v", err)
			return
		}
		for _, a := range res.Artists {
			name := a.ID
			if a.Name != nil {
				name = *a.Name
			}
			log.Printf("release sweep: %s — 새 앨범 %d개: %v", name, len(a.Albums), a.Albums)
		}
		if len(unlinked) > 0 && (res.Error != nil || res.Remaining == 0 || res.Checked == 0) {
			log.Printf("release sweep: Apple 미연결 아티스트 %d명: %v", len(unlinked), unlinked)
		}
		if res.Error != nil {
			log.Printf("release sweep: Apple 오류로 중단(%s) — %d명 확인, 새 앨범 %d개", *res.Error, checked, albums)
			return
		}
		if res.Remaining == 0 || res.Checked == 0 {
			log.Printf("release sweep: 완료 — %d명 확인, 새 앨범 %d개", checked, albums)
			return
		}
	}
}

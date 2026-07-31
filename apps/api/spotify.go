package main

// Admin-triggered Spotify crawling — the web counterpart of apps/crawler.
// Search artists, then pull a picked artist's albums straight into the DB.
// Credentials mirror the crawler's scheme: key "first"/"second" selects the
// FIRST_/SECOND_-prefixed pair (rate-limit spreading); empty key falls back
// SPOTIFY_CLIENT_* → FIRST_SPOTIFY_CLIENT_*.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

const spAPI = "https://api.spotify.com/v1"

func spotifyCreds(key string) (id, secret string, err error) {
	key = strings.ToLower(strings.TrimSpace(key))
	prefixes := []string{"", "FIRST_"}
	if key != "" {
		prefixes = []string{strings.ToUpper(key) + "_"}
	}
	for _, p := range prefixes {
		id, secret = os.Getenv(p+"SPOTIFY_CLIENT_ID"), os.Getenv(p+"SPOTIFY_CLIENT_SECRET")
		if id != "" && secret != "" {
			return id, secret, nil
		}
	}
	if key == "" {
		return "", "", errors.New("no spotify credentials: set SPOTIFY_CLIENT_ID/SECRET (or FIRST_-prefixed)")
	}
	return "", "", fmt.Errorf("no spotify credentials for key %q: set %s_SPOTIFY_CLIENT_ID/SECRET", key, strings.ToUpper(key))
}

// spotifyTokens caches one client-credentials token per key.
type spotifyTokens struct {
	mu    sync.Mutex
	byKey map[string]struct {
		value string
		exp   time.Time
	}
}

func newSpotifyTokens() *spotifyTokens {
	return &spotifyTokens{byKey: map[string]struct {
		value string
		exp   time.Time
	}{}}
}

func (t *spotifyTokens) get(ctx context.Context, key string, force bool) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if tok, ok := t.byKey[key]; ok && !force && time.Now().Before(tok.exp) {
		return tok.value, nil
	}
	id, secret, err := spotifyCreds(key)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://accounts.spotify.com/api/token", strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(id, secret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("spotify token %d: %s", res.StatusCode, body)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		return "", errors.New("spotify token response malformed")
	}
	t.byKey[key] = struct {
		value string
		exp   time.Time
	}{out.AccessToken, time.Now().Add(time.Duration(out.ExpiresIn-60) * time.Second)}
	return out.AccessToken, nil
}

// spError is a non-2xx Spotify response, kept typed so callers can branch on
// Status (the tracks backfill treats a 404 differently from a quota error).
type spError struct {
	Status int
	URL    string
	Body   string
}

func (e *spError) Error() string {
	return fmt.Sprintf("spotify %d @ %s: %.200s", e.Status, e.URL, e.Body)
}

// spGet fetches a Spotify API URL into dst, refreshing the token once on 401
// and waiting out one short 429 (long retry-after = quota exhausted → error).
func (s *server) spGet(ctx context.Context, key, rawURL string, dst any) error {
	refreshed := false
	waited := false
	for {
		tok, err := s.sp.get(ctx, key, refreshed)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		switch {
		case res.StatusCode == 401 && !refreshed:
			refreshed = true
			continue
		case res.StatusCode == 429 && !waited:
			wait, _ := strconv.Atoi(res.Header.Get("Retry-After"))
			if wait <= 0 {
				wait = 1
			}
			if wait > 30 {
				return fmt.Errorf("spotify rate limited: retry-after %ds (quota exhausted?)", wait)
			}
			waited = true
			select {
			case <-time.After(time.Duration(wait+1) * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		case res.StatusCode >= 400:
			return &spError{Status: res.StatusCode, URL: rawURL, Body: string(body)}
		}
		return json.Unmarshal(body, dst)
	}
}

// ---- Spotify payload shapes (only the fields we use) ----

type spArtistFull struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Genres []string
	Images []struct {
		URL string `json:"url"`
	}
	Followers struct {
		Total int `json:"total"`
	}
	ExternalURLs struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
}

// spCopyright is one ℗/© line. 2026-02 개편으로 label 필드가 죽은 뒤 레이블명이
// 실려오는 유일한 자리 (예: "℗ 2025 AOMG").
type spCopyright struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

// spAlbum decodes both the simplified album (/artists/{id}/albums 목록) and the
// full object (/albums/{id}). upc/copyrights/release_date_precision은 full에만
// 실려온다 — 없으면 upsert가 기존 값을 보존한다.
type spAlbum struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	AlbumType            string `json:"album_type"`
	ReleaseDate          string `json:"release_date"`
	ReleaseDatePrecision string `json:"release_date_precision"`
	TotalTracks          int    `json:"total_tracks"`
	Images               []struct {
		URL string `json:"url"`
	}
	ExternalURLs struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
	ExternalIDs struct {
		UPC string `json:"upc"`
	} `json:"external_ids"`
	Copyrights []spCopyright `json:"copyrights"`
	Artists    []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"artists"`
}

// copyrightsJSON marshals the ℗/© lines for the JSONB column; nil = 값 없음
// (simplified 앨범) → COALESCE가 기존 값을 지킨다.
func copyrightsJSON(cr []spCopyright) any {
	if len(cr) == 0 {
		return nil
	}
	b, err := json.Marshal(cr)
	if err != nil {
		return nil
	}
	return b
}

func yearOf(release string) *int {
	if len(release) >= 4 {
		if y, err := strconv.Atoi(release[:4]); err == nil {
			return &y
		}
	}
	return nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// configuredSpotifyKeys scans the environment for <PREFIX>_SPOTIFY_CLIENT_ID/SECRET
// pairs and returns the lowercase prefixes, sorted — so adding THIRD_ (or any
// name) to .env makes it selectable without code changes.
func configuredSpotifyKeys() []string {
	keys := []string{}
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		p, found := strings.CutSuffix(name, "_SPOTIFY_CLIENT_ID")
		if !found || p == "" {
			continue
		}
		if os.Getenv(p+"_SPOTIFY_CLIENT_SECRET") != "" {
			keys = append(keys, strings.ToLower(p))
		}
	}
	sort.Strings(keys)
	return keys
}

// ---- Handlers ----

// GET /admin/spotify/keys — which credential pairs exist; the web toggle renders these.
func (s *server) adminSpotifyKeys(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, configuredSpotifyKeys())
}

// GET /admin/spotify/artists?q=&key= — Spotify artist candidates plus how many
// albums each already has in OUR db (free local lookup; Spotify search carries
// no album count and per-candidate count calls would burn quota).
func (s *server) adminSpotifySearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, 400, "q is required")
		return
	}
	key := r.URL.Query().Get("key")

	var res struct {
		Artists struct {
			Items []spArtistFull `json:"items"`
		} `json:"artists"`
	}
	u := spAPI + "/search?type=artist&limit=8&q=" + url.QueryEscape(q)
	if err := s.spGet(r.Context(), key, u, &res); err != nil {
		writeErr(w, 502, err.Error())
		return
	}

	ids := make([]string, 0, len(res.Artists.Items))
	for _, a := range res.Artists.Items {
		ids = append(ids, a.ID)
	}
	counts := map[string]int{}
	if len(ids) > 0 {
		rows, err := s.db.Query(r.Context(),
			"SELECT artist_id, COUNT(*)::int FROM album_artists WHERE artist_id = ANY($1) GROUP BY artist_id", ids)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		for rows.Next() {
			var id string
			var n int
			if err := rows.Scan(&id, &n); err != nil {
				rows.Close()
				writeErr(w, 500, err.Error())
				return
			}
			counts[id] = n
		}
		rows.Close()
	}

	type hit struct {
		ID         string  `json:"id"`
		Name       string  `json:"name"`
		ImageURL   *string `json:"image_url"`
		Followers  int     `json:"followers"`
		AlbumsInDB int     `json:"albums_in_db"`
	}
	out := make([]hit, 0, len(res.Artists.Items))
	for _, a := range res.Artists.Items {
		h := hit{ID: a.ID, Name: a.Name, Followers: a.Followers.Total, AlbumsInDB: counts[a.ID]}
		if len(a.Images) > 0 {
			h.ImageURL = strPtr(a.Images[0].URL)
		}
		out = append(out, h)
	}
	writeJSON(w, 200, out)
}

// upsertAlbums writes each album row and rewrites its credited-artist join rows,
// returning every credited artist ID. Shared by the artist crawl and the
// album-search add so both store exactly the same shape.
func upsertAlbums(ctx context.Context, tx pgx.Tx, albums []spAlbum) (map[string]bool, error) {
	credited := map[string]bool{}
	for _, al := range albums {
		var img *string
		if len(al.Images) > 0 {
			img = strPtr(al.Images[0].URL)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO albums(id,name,release_date,year,album_type,total_tracks,image_url,spotify_url,
			                    upc,copyrights,release_date_precision)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			 ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name, release_date=EXCLUDED.release_date,
			   year=EXCLUDED.year, album_type=EXCLUDED.album_type, total_tracks=EXCLUDED.total_tracks,
			   image_url=EXCLUDED.image_url, spotify_url=EXCLUDED.spotify_url,
			   upc=COALESCE(EXCLUDED.upc, albums.upc),
			   copyrights=COALESCE(EXCLUDED.copyrights, albums.copyrights),
			   release_date_precision=COALESCE(EXCLUDED.release_date_precision, albums.release_date_precision)`,
			al.ID, al.Name, strPtr(al.ReleaseDate), yearOf(al.ReleaseDate), strPtr(al.AlbumType),
			al.TotalTracks, img, strPtr(al.ExternalURLs.Spotify),
			strPtr(al.ExternalIDs.UPC), copyrightsJSON(al.Copyrights), strPtr(al.ReleaseDatePrecision)); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM album_artists WHERE album_id=$1", al.ID); err != nil {
			return nil, err
		}
		for i, ar := range al.Artists {
			if ar.ID == "" {
				continue
			}
			credited[ar.ID] = true
			if _, err := tx.Exec(ctx,
				"INSERT INTO artists(id,name) VALUES($1,$2) ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name",
				ar.ID, strPtr(ar.Name)); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx,
				"INSERT INTO album_artists(album_id,artist_id,position) VALUES($1,$2,$3) ON CONFLICT(album_id,artist_id) DO UPDATE SET position=EXCLUDED.position",
				al.ID, ar.ID, i); err != nil {
				return nil, err
			}
		}
	}
	return credited, nil
}

// enrichCredited fills name/image/genres/followers for credited artists that are
// still missing an image or follower count — one request each (the batch
// /artists endpoint 403s for dev-mode apps). Best-effort: failures are skipped
// and a later enrich run picks them up.
func (s *server) enrichCredited(ctx context.Context, key string, credited map[string]bool) int {
	ids := make([]string, 0, len(credited))
	for id := range credited {
		ids = append(ids, id)
	}
	var missing []string
	rows, err := s.db.Query(ctx,
		"SELECT id FROM artists WHERE id = ANY($1) AND (image_url IS NULL OR followers IS NULL)", ids)
	if err == nil {
		missing, _ = pgx.CollectRows(rows, pgx.RowTo[string])
	}
	enriched := 0
	for _, id := range missing {
		var a spArtistFull
		if err := s.spGet(ctx, key, spAPI+"/artists/"+url.PathEscape(id), &a); err != nil {
			continue
		}
		var img *string
		if len(a.Images) > 0 {
			img = strPtr(a.Images[0].URL)
		}
		if _, err := s.db.Exec(ctx,
			`UPDATE artists SET name=COALESCE($2,name), image_url=COALESCE($3,image_url),
			   genres=COALESCE($4,genres), spotify_url=COALESCE($5,spotify_url), followers=$6 WHERE id=$1`,
			id, strPtr(a.Name), img, a.Genres, strPtr(a.ExternalURLs.Spotify), a.Followers.Total); err == nil {
			enriched++
		}
	}
	return enriched
}

// syncNewAlbumTracks pulls tracks for albums never track-synced (i.e. the newly
// added ones) so a fresh add arrives complete; re-adding an existing album costs
// nothing extra. Best-effort like enrich: a failed album keeps tracks_synced_at
// NULL and the 트랙 동기화 backfill catches it later.
func (s *server) syncNewAlbumTracks(ctx context.Context, key, market string, albumIDs []string) int {
	var unsynced []string
	rows, err := s.db.Query(ctx,
		"SELECT id FROM albums WHERE id = ANY($1) AND deleted_at IS NULL AND tracks_synced_at IS NULL", albumIDs)
	if err == nil {
		unsynced, _ = pgx.CollectRows(rows, pgx.RowTo[string])
	}
	synced := 0
	for _, id := range unsynced {
		tracks, meta, _, err := s.fetchAlbumTracks(ctx, key, id, market)
		if err != nil {
			continue
		}
		if err := s.saveAlbumTracks(ctx, id, tracks, meta); err == nil {
			synced++
		}
	}
	return synced
}

// POST /admin/spotify/crawl {artist_id, key, appears_on} — pull every album of
// the artist into the DB (same rules as the crawler: 0 albums → artist not
// saved; every credited artist gets a row + join credits; missing images get
// enriched). appears_on=true widens include_groups to 참여·컴필레이션 앨범
// (쇼미더머니류) — noisy, so it's an explicit admin toggle, never the default.
func (s *server) adminSpotifyCrawl(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ArtistID  string `json:"artist_id"`
		Key       string `json:"key"`
		AppearsOn bool   `json:"appears_on"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.ArtistID == "" {
		writeErr(w, 400, "artist_id is required")
		return
	}
	market := env("MARKET", "KR")
	groups := "album,single"
	if body.AppearsOn {
		groups += ",appears_on,compilation"
	}

	// Page through the artist's albums (limit is capped at ~10 for client-credentials apps).
	albums := map[string]spAlbum{}
	next := spAPI + "/artists/" + url.PathEscape(body.ArtistID) + "/albums?include_groups=" + url.QueryEscape(groups) + "&market=" + url.QueryEscape(market) + "&limit=10"
	for next != "" {
		var page struct {
			Items []spAlbum `json:"items"`
			Next  *string   `json:"next"`
		}
		if err := s.spGet(r.Context(), body.Key, next, &page); err != nil {
			writeErr(w, 502, err.Error())
			return
		}
		for _, al := range page.Items {
			albums[al.ID] = al
		}
		next = ""
		if page.Next != nil {
			next = *page.Next
		}
	}
	if len(albums) == 0 {
		writeJSON(w, 200, map[string]any{"albums": 0, "saved": false})
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	list := make([]spAlbum, 0, len(albums))
	albumIDs := make([]string, 0, len(albums))
	for id, al := range albums {
		list = append(list, al)
		albumIDs = append(albumIDs, id)
	}
	credited, err := upsertAlbums(r.Context(), tx, list)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	enriched := s.enrichCredited(r.Context(), body.Key, credited)
	tracksSynced := s.syncNewAlbumTracks(r.Context(), body.Key, market, albumIDs)

	var name *string
	_ = s.db.QueryRow(r.Context(), "SELECT name FROM artists WHERE id=$1", body.ArtistID).Scan(&name)
	writeJSON(w, 200, map[string]any{
		"albums":        len(albums),
		"saved":         true,
		"artist_name":   name,
		"artists":       len(credited),
		"enriched":      enriched,
		"tracks_synced": tracksSynced,
	})
}

// GET /admin/spotify/albums?q=&key= — Spotify album candidates for a title
// search, flagged with whether we already hold them. Album search has no genre
// filter (Spotify exposes none at album level), so this is a pick-by-hand list:
// the admin curates, exactly like artists.txt does for the artist crawl.
func (s *server) adminSpotifyAlbumSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, 400, "q is required")
		return
	}
	key := r.URL.Query().Get("key")
	market := env("MARKET", "KR")

	var res struct {
		Albums struct {
			Items []spAlbum `json:"items"`
		} `json:"albums"`
	}
	// limit is capped at 10 for client-credentials apps (same cap as the artist
	// album listing above); anything higher comes back 400 "Invalid limit".
	u := spAPI + "/search?type=album&limit=10&market=" + url.QueryEscape(market) + "&q=" + url.QueryEscape(q)
	if err := s.spGet(r.Context(), key, u, &res); err != nil {
		writeErr(w, 502, err.Error())
		return
	}

	ids := make([]string, 0, len(res.Albums.Items))
	for _, al := range res.Albums.Items {
		ids = append(ids, al.ID)
	}
	// deleted albums are still rows — say so, otherwise adding one looks like a
	// no-op in the feed (the add path deliberately never clears deleted_at).
	deleted := map[string]bool{}
	if len(ids) > 0 {
		rows, err := s.db.Query(r.Context(),
			"SELECT id, deleted_at IS NOT NULL FROM albums WHERE id = ANY($1)", ids)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		for rows.Next() {
			var id string
			var del bool
			if err := rows.Scan(&id, &del); err != nil {
				rows.Close()
				writeErr(w, 500, err.Error())
				return
			}
			deleted[id] = del
		}
		rows.Close()
	}

	type hit struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		ImageURL    *string `json:"image_url"`
		ReleaseDate *string `json:"release_date"`
		AlbumType   *string `json:"album_type"`
		TotalTracks int     `json:"total_tracks"`
		Artists     string  `json:"artists"`
		InDB        bool    `json:"in_db"`
		Deleted     bool    `json:"deleted"`
	}
	out := make([]hit, 0, len(res.Albums.Items))
	for _, al := range res.Albums.Items {
		names := make([]string, 0, len(al.Artists))
		for _, ar := range al.Artists {
			names = append(names, ar.Name)
		}
		del, inDB := deleted[al.ID]
		out = append(out, hit{
			ID: al.ID, Name: al.Name, ImageURL: nil,
			ReleaseDate: strPtr(al.ReleaseDate), AlbumType: strPtr(al.AlbumType),
			TotalTracks: al.TotalTracks, Artists: strings.Join(names, ", "),
			InDB: inDB, Deleted: del,
		})
		if len(al.Images) > 0 {
			out[len(out)-1].ImageURL = strPtr(al.Images[0].URL)
		}
	}
	writeJSON(w, 200, out)
}

// POST /admin/spotify/crawl-album {album_id, key} — add one album picked from
// the album search. Same storage path as the artist crawl (album row + credited
// artists + join rows, then enrich and track-sync), just scoped to one album.
func (s *server) adminSpotifyCrawlAlbum(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AlbumID string `json:"album_id"`
		Key     string `json:"key"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.AlbumID == "" {
		writeErr(w, 400, "album_id is required")
		return
	}
	market := env("MARKET", "KR")

	// Refetch by ID rather than trusting the search payload: same shape, one
	// request, and the stored row can't go stale against what the admin saw.
	var al spAlbum
	u := spAPI + "/albums/" + url.PathEscape(body.AlbumID) + "?market=" + url.QueryEscape(market)
	if err := s.spGet(r.Context(), body.Key, u, &al); err != nil {
		writeErr(w, 502, err.Error())
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	credited, err := upsertAlbums(r.Context(), tx, []spAlbum{al})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	enriched := s.enrichCredited(r.Context(), body.Key, credited)
	tracksSynced := s.syncNewAlbumTracks(r.Context(), body.Key, market, []string{al.ID})

	var deleted bool
	_ = s.db.QueryRow(r.Context(), "SELECT deleted_at IS NOT NULL FROM albums WHERE id=$1", al.ID).Scan(&deleted)
	writeJSON(w, 200, map[string]any{
		"album_id":      al.ID,
		"album_name":    al.Name,
		"saved":         true,
		"artists":       len(credited),
		"enriched":      enriched,
		"tracks_synced": tracksSynced,
		"deleted":       deleted, // true = row exists but is soft-deleted; restore it in 삭제된 앨범 탭
	})
}

// ---- 신보 체크 (release check) — 삭제된 /browse/new-releases의 로스터 기반 대체 ----

// releaseEligibleSQL: live 앨범의 대표 크레딧(position=0)을 가진 아티스트만 체크
// 대상 — 사실상의 로스터. 앨범 단위 피처링("Juicy (feat. Colde)"의 Colde,
// position≥1)이나 트랙 크레딧(tracks.artists JSONB)에만 등장하는 아티스트는
// 제외된다. position 조건이 없으면 피처링된 발라드 가수의 OST까지 신보로
// 딸려온다 (E2E에서 실제로 확인).
const releaseEligibleSQL = `FROM artists a WHERE EXISTS (
	SELECT 1 FROM album_artists aa JOIN albums al ON al.id = aa.album_id
	WHERE aa.artist_id = a.id AND aa.position = 0 AND al.deleted_at IS NULL)`

// 20시간 주기: 매일 같은 시각쯤 돌려도 어제 체크분이 다시 대상이 된다.
const releaseStaleSQL = releaseEligibleSQL +
	` AND (a.releases_checked_at IS NULL OR a.releases_checked_at < now() - interval '20 hours')`

// GET /admin/spotify/releases-status — 신보 체크 현황 (대상/이번 주기 대기).
func (s *server) adminReleasesStatus(w http.ResponseWriter, r *http.Request) {
	var artists, stale int
	if err := s.db.QueryRow(r.Context(), "SELECT COUNT(*)::int "+releaseEligibleSQL).Scan(&artists); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.db.QueryRow(r.Context(), "SELECT COUNT(*)::int "+releaseStaleSQL).Scan(&stale); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]int{"artists": artists, "stale": stale})
}

type releaseCheckItem struct {
	ID     string   `json:"id"`
	Name   *string  `json:"name"`
	Albums []string `json:"albums"` // 이번에 새로 담은 앨범 이름들
}

type releaseCheckResult struct {
	Checked      int                `json:"checked"`   // 이번 배치에서 확인한 아티스트 수
	NewAlbums    int                `json:"new_albums"`
	Enriched     int                `json:"enriched"`
	TracksSynced int                `json:"tracks_synced"`
	Remaining    int                `json:"remaining"` // 이번 주기에 아직 안 본 아티스트 수
	Artists      []releaseCheckItem `json:"artists"`   // 신보가 있던 아티스트만
	Error        *string            `json:"error,omitempty"`
}

// POST /admin/spotify/check-releases {key, limit} — DB 보유 아티스트를 오래 안 본
// 순서대로 limit명 골라 신보를 감지한다. 아티스트당 Spotify 요청 1회: 앨범 목록
// 첫 페이지(최신순 10장)만 본다 — 한 주기에 10장 넘게 내는 아티스트는 없다.
// DB에 행이 아예 없는 앨범만 저장하므로 관리자가 삭제한 앨범(soft-delete 행
// 존재)은 되살아나지 않는다. 트랙 백필과 같은 배치 계약: Spotify 오류가 나면
// 멈추고 부분 진행을 보고하며, 실패한 아티스트는 스탬프가 안 찍혀 다음 호출이
// 다시 집는다. 아티스트 자체가 Spotify에서 사라진 404는 스탬프만 찍고 넘어간다.
func (s *server) adminCheckReleases(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key   string `json:"key"`
		Limit int    `json:"limit"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Limit < 1 || body.Limit > 50 {
		body.Limit = 10
	}
	market := env("MARKET", "KR")

	rows, err := s.db.Query(r.Context(),
		"SELECT a.id, COALESCE(a.display_name, a.name) "+releaseStaleSQL+
			" ORDER BY a.releases_checked_at NULLS FIRST, a.id LIMIT $1", body.Limit)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	type candidate struct {
		ID   string
		Name *string
	}
	pending, err := pgx.CollectRows(rows, pgx.RowToStructByPos[candidate])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	res := releaseCheckResult{Artists: []releaseCheckItem{}}
	credited := map[string]bool{}
	var newAlbumIDs []string
	for _, ar := range pending {
		var page struct {
			Items []spAlbum `json:"items"`
		}
		u := spAPI + "/artists/" + url.PathEscape(ar.ID) + "/albums?include_groups=album,single&market=" + url.QueryEscape(market) + "&limit=10"
		if err := s.spGet(r.Context(), body.Key, u, &page); err != nil {
			var se *spError
			if !errors.As(err, &se) || se.Status != 404 {
				name := ar.ID
				if ar.Name != nil {
					name = *ar.Name
				}
				res.Error = strPtr(fmt.Sprintf("%s: %v", name, err))
				break
			}
			page.Items = nil // 아티스트가 Spotify에서 사라짐 — 볼 신보 없음
		}

		ids := make([]string, 0, len(page.Items))
		for _, al := range page.Items {
			ids = append(ids, al.ID)
		}
		existing := map[string]bool{}
		if len(ids) > 0 {
			erows, err := s.db.Query(r.Context(), "SELECT id FROM albums WHERE id = ANY($1)", ids)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			got, err := pgx.CollectRows(erows, pgx.RowTo[string])
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			for _, id := range got {
				existing[id] = true
			}
		}
		fresh := make([]spAlbum, 0, len(page.Items))
		for _, al := range page.Items {
			if !existing[al.ID] {
				fresh = append(fresh, al)
			}
		}

		if len(fresh) > 0 {
			tx, err := s.db.Begin(r.Context())
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			cr, err := upsertAlbums(r.Context(), tx, fresh)
			if err != nil {
				tx.Rollback(r.Context())
				writeErr(w, 500, err.Error())
				return
			}
			if err := tx.Commit(r.Context()); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			for id := range cr {
				credited[id] = true
			}
			item := releaseCheckItem{ID: ar.ID, Name: ar.Name, Albums: make([]string, 0, len(fresh))}
			for _, al := range fresh {
				newAlbumIDs = append(newAlbumIDs, al.ID)
				item.Albums = append(item.Albums, al.Name)
			}
			res.Artists = append(res.Artists, item)
			res.NewAlbums += len(fresh)
		}

		if _, err := s.db.Exec(r.Context(), "UPDATE artists SET releases_checked_at=now() WHERE id=$1", ar.ID); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		res.Checked++
	}

	if len(credited) > 0 {
		res.Enriched = s.enrichCredited(r.Context(), body.Key, credited)
	}
	if len(newAlbumIDs) > 0 {
		res.TracksSynced = s.syncNewAlbumTracks(r.Context(), body.Key, market, newAlbumIDs)
	}

	if err := s.db.QueryRow(r.Context(), "SELECT COUNT(*)::int "+releaseStaleSQL).Scan(&res.Remaining); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

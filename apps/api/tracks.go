package main

// Admin 트랙 동기화 — backfill each album's track list from Spotify, batch by
// batch. GET /v1/albums/{id} already carries the first 50 tracks, so nearly
// every album costs exactly one request; longer albums follow tracks.next.
// The web admin tab calls /backfill repeatedly until `remaining` hits 0.
//
// 같은 full object에 실려오는 앨범 메타(upc/copyrights/release_date_precision)도
// 여기서 같이 저장한다 — 이미 트랙이 동기화됐지만 메타가 없는 앨범(과거 수집분)도
// 백필 대상에 들어가므로, 한 번의 요청으로 트랙 재확인과 메타 채움을 겸한다.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/jackc/pgx/v5"
)

// trackArtist is one credited artist on a track (features included), stored
// verbatim in the tracks.artists JSONB column.
type trackArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Track is the public wire shape of one row in the tracks table.
type Track struct {
	ID          string `json:"id" db:"id"`
	DiscNumber  int    `json:"disc_number" db:"disc_number"`
	TrackNumber int    `json:"track_number" db:"track_number"`
	Name        string `json:"name" db:"name"`
	// 한글 표시 이름 — admin-curated, null by default; 동기화가 덮어쓰지 않는다.
	DisplayName *string       `json:"display_name" db:"display_name"`
	DurationMS  *int          `json:"duration_ms" db:"duration_ms"`
	Explicit    bool          `json:"explicit" db:"explicit"`
	SpotifyURL  *string       `json:"spotify_url" db:"spotify_url"`
	Artists     []trackArtist `json:"artists" db:"artists"`
}

// ---- Spotify payload shapes (only the fields we use) ----

type spTrack struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	DiscNumber   int    `json:"disc_number"`
	TrackNumber  int    `json:"track_number"`
	DurationMS   int    `json:"duration_ms"`
	Explicit     bool   `json:"explicit"`
	ExternalURLs struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
	Artists []trackArtist `json:"artists"`
}

type spTrackPage struct {
	Items []spTrack `json:"items"`
	Next  *string   `json:"next"`
}

// GET /albums/{id}/tracks — 관리자 전용 tracklist, disc/track order. 일반
// 사용자에겐 앨범 상세가 Spotify 임베드를 보여준다.
func (s *server) listAlbumTracks(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(),
		`SELECT id,disc_number,track_number,name,display_name,duration_ms,explicit,spotify_url,artists
		 FROM tracks WHERE album_id=$1 ORDER BY disc_number, track_number, id`, r.PathValue("id"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	tracks, err := pgx.CollectRows(rows, pgx.RowToStructByName[Track])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if tracks == nil {
		tracks = []Track{} // never-synced album: [] not null
	}
	writeJSON(w, 200, tracks)
}

// GET /admin/tracks/status — 동기화 진행 현황 (live albums only). "완료"는 트랙과
// 앨범 메타가 모두 있는 상태 — release_date_precision은 full object에 항상 실려
// 오므로 NULL이면 아직 메타를 못 받은 앨범이다.
func (s *server) adminTracksStatus(w http.ResponseWriter, r *http.Request) {
	var albums, synced, tracks int
	if err := s.db.QueryRow(r.Context(),
		`SELECT COUNT(*)::int,
		        COUNT(*) FILTER (WHERE tracks_synced_at IS NOT NULL AND release_date_precision IS NOT NULL)::int
		 FROM albums WHERE deleted_at IS NULL`).
		Scan(&albums, &synced); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.db.QueryRow(r.Context(), "SELECT COUNT(*)::int FROM tracks").Scan(&tracks); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]int{
		"albums":  albums,
		"synced":  synced,
		"missing": albums - synced,
		"tracks":  tracks,
	})
}

type trackBackfillItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Tracks   int    `json:"tracks"`
	NotFound bool   `json:"not_found,omitempty"` // gone from Spotify → saved as empty
}

type trackBackfillResult struct {
	Synced    int                 `json:"synced"`    // albums written this batch (incl. not-found)
	NotFound  int                 `json:"not_found"` // of synced, albums Spotify 404'd
	Tracks    int                 `json:"tracks"`    // track rows written this batch
	Remaining int                 `json:"remaining"` // albums still pending after this batch
	Albums    []trackBackfillItem `json:"albums"`    // per-album log for the admin UI
	Error     *string             `json:"error,omitempty"` // batch stopped early (quota, network, …)
}

// POST /admin/tracks/backfill {key, limit} — sync up to limit never-synced live
// albums, newest release first. Each album commits in its own tx, so a stopped
// batch keeps every finished album and the next call resumes after it. A 404
// marks the album synced-with-0-tracks (it left Spotify — retrying is useless);
// any other Spotify error ends the batch and is reported with partial progress.
func (s *server) adminTracksBackfill(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key   string `json:"key"`
		Limit int    `json:"limit"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Limit < 1 || body.Limit > 50 {
		body.Limit = 20
	}
	market := env("MARKET", "KR")

	rows, err := s.db.Query(r.Context(),
		`SELECT id, name FROM albums
		 WHERE deleted_at IS NULL AND (tracks_synced_at IS NULL OR release_date_precision IS NULL)
		 ORDER BY release_date DESC NULLS LAST, id LIMIT $1`, body.Limit)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	type candidate struct{ ID, Name string }
	pending, err := pgx.CollectRows(rows, pgx.RowToStructByPos[candidate])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	res := trackBackfillResult{Albums: []trackBackfillItem{}}
	for _, al := range pending {
		tracks, meta, notFound, err := s.fetchAlbumTracks(r.Context(), body.Key, al.ID, market)
		if err != nil {
			res.Error = strPtr(fmt.Sprintf("%s (%s): %v", al.Name, al.ID, err))
			break
		}
		if err := s.saveAlbumTracks(r.Context(), al.ID, tracks, meta); err != nil {
			res.Error = strPtr(fmt.Sprintf("%s (%s): %v", al.Name, al.ID, err))
			break
		}
		res.Synced++
		res.Tracks += len(tracks)
		if notFound {
			res.NotFound++
		}
		res.Albums = append(res.Albums, trackBackfillItem{ID: al.ID, Name: al.Name, Tracks: len(tracks), NotFound: notFound})
	}

	if err := s.db.QueryRow(r.Context(),
		`SELECT COUNT(*)::int FROM albums
		 WHERE deleted_at IS NULL AND (tracks_synced_at IS NULL OR release_date_precision IS NULL)`).
		Scan(&res.Remaining); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// fetchAlbumTracks pulls the album's full track list plus the full-object 메타
// (upc/copyrights/precision — 같은 응답에 실려오므로 공짜). notFound=true means
// the album no longer exists on Spotify (or left the market) — caller saves it
// as synced with no tracks instead of retrying forever.
func (s *server) fetchAlbumTracks(ctx context.Context, key, albumID, market string) (tracks []spTrack, meta *spAlbum, notFound bool, err error) {
	var full struct {
		spAlbum
		Tracks spTrackPage `json:"tracks"`
	}
	u := spAPI + "/albums/" + url.PathEscape(albumID) + "?market=" + url.QueryEscape(market)
	if err := s.spGet(ctx, key, u, &full); err != nil {
		var se *spError
		if errors.As(err, &se) && se.Status == 404 {
			return nil, nil, true, nil
		}
		return nil, nil, false, err
	}
	tracks = full.Tracks.Items
	// >50 tracks is rare, but page through the rest when it happens.
	for next := full.Tracks.Next; next != nil && *next != ""; {
		var page spTrackPage
		if err := s.spGet(ctx, key, *next, &page); err != nil {
			return nil, nil, false, err
		}
		tracks = append(tracks, page.Items...)
		next = page.Next
	}
	return tracks, &full.spAlbum, false, nil
}

// saveAlbumTracks rewrites the album's tracks, 앨범 메타, and tracks_synced_at in
// one tx — an album is either fully synced or untouched. Existing rows are
// upserted (not delete-all'd) so the admin-curated display_name survives a
// re-sync; only tracks gone from Spotify are dropped. meta nil = 404였던 앨범;
// precision에 'unknown'을 채워 메타 백필 대상에서 빠지게 한다 (재시도 무의미).
func (s *server) saveAlbumTracks(ctx context.Context, albumID string, tracks []spTrack, meta *spAlbum) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	keep := make([]string, 0, len(tracks))
	for _, t := range tracks {
		if t.ID != "" {
			keep = append(keep, t.ID)
		}
	}
	if _, err := tx.Exec(ctx, "DELETE FROM tracks WHERE album_id=$1 AND NOT (id = ANY($2))", albumID, keep); err != nil {
		return err
	}
	for _, t := range tracks {
		if t.ID == "" {
			continue
		}
		artists := t.Artists
		if artists == nil {
			artists = []trackArtist{}
		}
		aj, err := json.Marshal(artists)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO tracks(album_id,id,disc_number,track_number,name,duration_ms,explicit,spotify_url,artists)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
			 ON CONFLICT(album_id,id) DO UPDATE SET disc_number=EXCLUDED.disc_number,
			 track_number=EXCLUDED.track_number, name=EXCLUDED.name, duration_ms=EXCLUDED.duration_ms,
			 explicit=EXCLUDED.explicit, spotify_url=EXCLUDED.spotify_url, artists=EXCLUDED.artists`,
			albumID, t.ID, t.DiscNumber, t.TrackNumber, t.Name, t.DurationMS, t.Explicit,
			strPtr(t.ExternalURLs.Spotify), aj); err != nil {
			return err
		}
	}
	if meta != nil {
		// COALESCE($n, …, 'unknown'): full object인데도 필드가 빈 앨범이 영원히
		// 백필 후보로 남지 않게 precision만은 반드시 채운다.
		if _, err := tx.Exec(ctx,
			`UPDATE albums SET tracks_synced_at=now(),
			   upc=COALESCE($2, upc), copyrights=COALESCE($3, copyrights),
			   release_date_precision=COALESCE($4, release_date_precision, 'unknown')
			 WHERE id=$1`,
			albumID, strPtr(meta.ExternalIDs.UPC), copyrightsJSON(meta.Copyrights),
			strPtr(meta.ReleaseDatePrecision)); err != nil {
			return err
		}
	} else if _, err := tx.Exec(ctx,
		`UPDATE albums SET tracks_synced_at=now(),
		   release_date_precision=COALESCE(release_date_precision, 'unknown')
		 WHERE id=$1`, albumID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// updateTrackDisplayName replaces only the 트랙 한글 표시 이름 (admin-only) — same
// partial-update shape as updateArtistDisplayName; 동기화는 이 값을 건드리지 않는다.
func (s *server) updateTrackDisplayName(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DisplayName *string `json:"display_name"`
	}
	if !decode(w, r, &body) {
		return
	}
	tag, err := s.db.Exec(r.Context(), "UPDATE tracks SET display_name=$3 WHERE album_id=$1 AND id=$2",
		r.PathValue("id"), r.PathValue("trackId"), nilIfBlank(body.DisplayName))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, 404, "track not found")
		return
	}
	w.WriteHeader(204)
}

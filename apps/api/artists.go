package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Artist struct {
	ID   string  `json:"id" db:"id"`
	Name *string `json:"name" db:"name"`
	// 한글 표시 이름 — admin-curated, null by default. Spotify hands back only the
	// name the label registered (usually English); the UI falls back to Name.
	DisplayName *string  `json:"display_name" db:"display_name"`
	ImageURL    *string  `json:"image_url" db:"image_url"`
	Genres      []string `json:"genres" db:"genres"`
	SpotifyURL  *string  `json:"spotify_url" db:"spotify_url"`
	// 연관검색어 — admin-curated search keywords (e.g. Korean spellings of an
	// English artist name). Never written by the crawler.
	Aliases []string `json:"aliases" db:"aliases"`
	// Spotify follower count, crawler/enrich-owned; null until first enriched.
	// Kept for the admin refresh flow — the UI shows FollowerCount instead.
	Followers *int `json:"followers" db:"followers"`
	// 신보 감시 대상 여부 — 신보 체크가 이 플래그 켜진 아티스트만 본다. 관리자
	// 토글 소유(크롤러는 안 건드림); 마이그레이션이 대표 크레딧 앨범 수로 시드.
	ReleasesWatch bool `json:"releases_watch" db:"releases_watch"`
	// 서비스 내부 팔로워 수 — follows 집계. Read-only output column.
	FollowerCount int `json:"follower_count" db:"follower_count"`
}

const artistCols = "id,name,display_name,image_url,genres,spotify_url,aliases,followers,releases_watch"

// artistSelectCols is artistCols plus the computed follower count; every artist
// read uses it, while artistCols alone stays valid as an INSERT column list.
// The subquery alias is fl (not f) so it survives inside listFollows' join.
const artistSelectCols = artistCols +
	",(SELECT COUNT(*) FROM follows fl WHERE fl.artist_id=artists.id)::int AS follower_count"

func (s *server) listArtists(w http.ResponseWriter, r *http.Request) {
	limit, offset := clampPage(r)
	sql := "SELECT " + artistSelectCols + " FROM artists WHERE 1=1"
	var args []any
	// q matches the artist name, the 한글 표시 이름, OR any admin-curated alias
	// (연관검색어), so a Korean query finds artists stored under an English name.
	if q := r.URL.Query().Get("q"); q != "" {
		// Same matching rules as the album search (word-start for Latin queries,
		// infix for 한글) — see searchMatch.
		op, val := searchMatch(q)
		args = append(args, val)
		p := "$" + strconv.Itoa(len(args))
		sql += " AND (name" + op + p + " OR display_name" + op + p +
			" OR EXISTS (SELECT 1 FROM unnest(COALESCE(aliases,'{}'::text[])) AS al WHERE al" + op + p + "))"
	}
	args = append(args, limit)
	// Sort by what the UI actually shows (한글명이 있으면 그 이름).
	sql += " ORDER BY COALESCE(display_name, name) LIMIT $" + strconv.Itoa(len(args))
	args = append(args, offset)
	sql += " OFFSET $" + strconv.Itoa(len(args))
	rows, err := s.db.Query(r.Context(), sql, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	artists, err := pgx.CollectRows(rows, pgx.RowToStructByName[Artist])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, artists)
}

func (s *server) getArtist(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), "SELECT "+artistSelectCols+" FROM artists WHERE id=$1", r.PathValue("id"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Artist])
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 404, "artist not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, a)
}

func (s *server) createArtist(w http.ResponseWriter, r *http.Request) {
	var a Artist
	if !decode(w, r, &a) {
		return
	}
	if a.ID == "" {
		writeErr(w, 400, "id is required")
		return
	}
	_, err := s.db.Exec(r.Context(), "INSERT INTO artists("+artistCols+") VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)",
		a.ID, a.Name, nilIfBlank(a.DisplayName), a.ImageURL, a.Genres, a.SpotifyURL, normalizeAliases(a.Aliases), a.Followers, a.ReleasesWatch)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		writeErr(w, 409, "artist id already exists")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, a)
}

func (s *server) updateArtist(w http.ResponseWriter, r *http.Request) {
	var a Artist
	if !decode(w, r, &a) {
		return
	}
	a.ID = r.PathValue("id")
	tag, err := s.db.Exec(r.Context(), "UPDATE artists SET name=$2,display_name=$3,image_url=$4,genres=$5,spotify_url=$6,aliases=$7 WHERE id=$1",
		a.ID, a.Name, nilIfBlank(a.DisplayName), a.ImageURL, a.Genres, a.SpotifyURL, normalizeAliases(a.Aliases))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, 404, "artist not found")
		return
	}
	writeJSON(w, 200, a)
}

// nilIfBlank trims a display name; blank (or missing) means "no override" → NULL,
// so the UI falls back to the crawler-supplied Spotify name.
func nilIfBlank(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// updateArtistDisplayName replaces only the 한글 표시 이름 (admin-only) — same
// partial-update shape as updateArtistAliases, so crawler-owned fields survive.
func (s *server) updateArtistDisplayName(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DisplayName *string `json:"display_name"`
	}
	if !decode(w, r, &body) {
		return
	}
	tag, err := s.db.Exec(r.Context(), "UPDATE artists SET display_name=$2 WHERE id=$1",
		r.PathValue("id"), nilIfBlank(body.DisplayName))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, 404, "artist not found")
		return
	}
	w.WriteHeader(204)
}

// updateArtistReleasesWatch flips only the 신보 감시 플래그 (admin-only) — same
// partial-update shape as updateArtistDisplayName; 크롤러·동기화는 안 건드린다.
func (s *server) updateArtistReleasesWatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Watch bool `json:"watch"`
	}
	if !decode(w, r, &body) {
		return
	}
	tag, err := s.db.Exec(r.Context(), "UPDATE artists SET releases_watch=$2 WHERE id=$1",
		r.PathValue("id"), body.Watch)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, 404, "artist not found")
		return
	}
	w.WriteHeader(204)
}

// normalizeAliases trims, drops empties, and dedups while keeping order.
func normalizeAliases(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, a := range in {
		if a = strings.TrimSpace(a); a != "" && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}

// updateArtistAliases replaces only the aliases (연관검색어) — the admin UI edits
// these without clobbering crawler-owned fields (name/image/genres/spotify_url).
func (s *server) updateArtistAliases(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Aliases []string `json:"aliases"`
	}
	if !decode(w, r, &body) {
		return
	}
	rows, err := s.db.Query(r.Context(),
		"UPDATE artists SET aliases=$2 WHERE id=$1 RETURNING "+artistSelectCols,
		r.PathValue("id"), normalizeAliases(body.Aliases))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Artist])
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 404, "artist not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, a)
}

// deleteArtist has two modes, picked with ?mode= (no default — the two outcomes
// are too different to guess at):
//
//	soft: keep the artist row, soft-delete only the albums they are the ONLY
//	      credited artist on. Reversible from 관리자 삭제 목록.
//	hard: delete the artist and EVERY album they are credited on — including
//	      albums that are really someone else's and only feature them — plus the
//	      favorites/ratings/comments hanging off those albums. Irreversible.
func (s *server) deleteArtist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	mode := r.URL.Query().Get("mode")
	if mode != "soft" && mode != "hard" {
		writeErr(w, 400, "mode must be 'soft' or 'hard'")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	var exists bool
	if err := tx.QueryRow(r.Context(), "SELECT EXISTS (SELECT 1 FROM artists WHERE id=$1)", id).Scan(&exists); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !exists {
		writeErr(w, 404, "artist not found")
		return
	}

	var albums int64
	if mode == "soft" {
		// "Solo" = the album has exactly one credit row and it points at this
		// artist, so nothing else on the album is lost.
		tag, err := tx.Exec(r.Context(), `
			UPDATE albums SET deleted_at = now()
			WHERE deleted_at IS NULL AND id IN (
				SELECT aa.album_id FROM album_artists aa
				GROUP BY aa.album_id
				HAVING count(*) = 1 AND min(aa.artist_id) = $1
			)`, id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		albums = tag.RowsAffected()
	} else {
		// Collect the album ids up front: deleting from album_artists would
		// otherwise erase the very rows that define the set.
		rows, err := tx.Query(r.Context(), "SELECT album_id FROM album_artists WHERE artist_id=$1", id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if len(ids) > 0 {
			// favorites/ratings/comments reference album_id without an FK, and
			// album_artists still holds the co-credits of the doomed albums —
			// all of it has to go by hand or it lingers as orphan rows.
			for _, q := range []string{
				"DELETE FROM favorites WHERE album_id = ANY($1)",
				"DELETE FROM ratings WHERE album_id = ANY($1)",
				"DELETE FROM comments WHERE album_id = ANY($1)",
				"DELETE FROM not_hiphop_reports WHERE album_id = ANY($1)",
				"DELETE FROM rename_requests WHERE album_id = ANY($1)",
				"DELETE FROM album_artists WHERE album_id = ANY($1)",
			} {
				if _, err := tx.Exec(r.Context(), q, ids); err != nil {
					writeErr(w, 500, err.Error())
					return
				}
			}
			tag, err := tx.Exec(r.Context(), "DELETE FROM albums WHERE id = ANY($1)", ids)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			albums = tag.RowsAffected()
		}
		// follows has no FK to artists either.
		if _, err := tx.Exec(r.Context(), "DELETE FROM follows WHERE artist_id=$1", id); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if _, err := tx.Exec(r.Context(), "DELETE FROM artists WHERE id=$1", id); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"mode": mode, "albums": albums})
}

// mergeArtists folds duplicate artists into one: album credits move to the
// master, the duplicates' names+aliases become the master's aliases (so old
// search terms keep matching), then the duplicate rows are deleted.
func (s *server) mergeArtists(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MasterID  string   `json:"master_id"`
		MergedIDs []string `json:"merged_ids"`
	}
	if !decode(w, r, &body) {
		return
	}
	var merged []string
	for _, id := range body.MergedIDs {
		if id != "" && id != body.MasterID {
			merged = append(merged, id)
		}
	}
	if body.MasterID == "" || len(merged) == 0 {
		writeErr(w, 400, "master_id and at least one other merged id are required")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	var masterName *string
	var masterAliases []string
	err = tx.QueryRow(r.Context(), "SELECT name, aliases FROM artists WHERE id=$1", body.MasterID).
		Scan(&masterName, &masterAliases)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 404, "master artist not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	// Absorbed artists' names + aliases → master aliases (minus the master's own name).
	rows, err := tx.Query(r.Context(), "SELECT name, aliases FROM artists WHERE id = ANY($1)", merged)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	extra := append([]string{}, masterAliases...)
	for rows.Next() {
		var name *string
		var aliases []string
		if err := rows.Scan(&name, &aliases); err != nil {
			rows.Close()
			writeErr(w, 500, err.Error())
			return
		}
		if name != nil {
			extra = append(extra, *name)
		}
		extra = append(extra, aliases...)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	aliases := normalizeAliases(extra)
	if masterName != nil {
		kept := aliases[:0]
		for _, a := range aliases {
			if a != *masterName {
				kept = append(kept, a)
			}
		}
		aliases = kept
	}

	// Move credits one duplicate at a time: the guard skips albums where the
	// master is already credited, and the leftover duplicate rows are dropped.
	for _, id := range merged {
		if _, err := tx.Exec(r.Context(),
			`UPDATE album_artists SET artist_id=$1 WHERE artist_id=$2
			 AND NOT EXISTS (SELECT 1 FROM album_artists m WHERE m.album_id = album_artists.album_id AND m.artist_id = $1)`,
			body.MasterID, id); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if _, err := tx.Exec(r.Context(), "DELETE FROM album_artists WHERE artist_id=$1", id); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}

	if _, err := tx.Exec(r.Context(), "UPDATE artists SET aliases=$2 WHERE id=$1", body.MasterID, aliases); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if _, err := tx.Exec(r.Context(), "DELETE FROM artists WHERE id = ANY($1)", merged); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	out, err := s.db.Query(r.Context(), "SELECT "+artistSelectCols+" FROM artists WHERE id=$1", body.MasterID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a, err := pgx.CollectExactlyOneRow(out, pgx.RowToStructByName[Artist])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, a)
}

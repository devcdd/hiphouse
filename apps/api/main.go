package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type server struct {
	db          *pgxpool.Pool
	jwtSecret   []byte
	kakaoKey    string
	kakaoSecret string
	adminIDs    map[string]bool
	sp          *spotifyTokens
}

func main() {
	// Single source of truth: repo-root .env (dev cwd is apps/api). Real env wins.
	loadDotenv(env("ENV_FILE", ""), "../../.env", ".env")

	dsn := env("DATABASE_URL", "postgres://hiphouse:hiphouse@localhost:5432/hiphouse")
	port := env("PORT", "8080")

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("db pool: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("db ping (is `docker compose up -d db` running?): %v", err)
	}

	s := &server{
		db:          pool,
		jwtSecret:   []byte(env("JWT_SECRET", "dev-insecure-secret-change-me")),
		kakaoKey:    os.Getenv("KAKAO_REST_API_KEY"),
		kakaoSecret: os.Getenv("KAKAO_CLIENT_SECRET"),
		adminIDs:    parseAdminIDs(os.Getenv("ADMIN_KAKAO_IDS")),
		sp:          newSpotifyTokens(),
	}
	if err := s.ensureAuthSchema(ctx); err != nil {
		log.Fatalf("auth schema: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	// Auth
	mux.HandleFunc("POST /auth/kakao", s.loginKakao)
	mux.HandleFunc("POST /auth/refresh", s.refreshSession)
	mux.HandleFunc("POST /auth/logout", s.logout)
	mux.HandleFunc("GET /me", s.requireAuth(s.me))
	mux.HandleFunc("PUT /me", s.requireAuth(s.updateMe))

	// Favorites (auth required)
	mux.HandleFunc("GET /favorites", s.requireAuth(s.listFavorites))
	mux.HandleFunc("POST /favorites", s.requireAuth(s.addFavorite))
	mux.HandleFunc("DELETE /favorites/{albumId}", s.requireAuth(s.removeFavorite))

	// Artist follows (auth required)
	mux.HandleFunc("GET /follows", s.requireAuth(s.listFollows))
	mux.HandleFunc("POST /follows", s.requireAuth(s.addFollow))
	mux.HandleFunc("DELETE /follows/{artistId}", s.requireAuth(s.removeFollow))

	// Ratings (auth required) — the caller's own scores, half-star granularity
	mux.HandleFunc("GET /ratings", s.requireAuth(s.listRatings))
	mux.HandleFunc("GET /ratings/albums", s.requireAuth(s.listRatedAlbums))
	mux.HandleFunc("PUT /ratings/{albumId}", s.requireAuth(s.putRating))
	mux.HandleFunc("DELETE /ratings/{albumId}", s.requireAuth(s.deleteRating))

	// "힙합이 아니에요" 신고 / "앨범명 좀 바꿔주세요" 요청 — 둘 다 사용자당 앨범 1건 (auth required)
	mux.HandleFunc("GET /not-hiphop", s.requireAuth(s.listFlags(tblNotHiphop)))
	mux.HandleFunc("POST /albums/{id}/not-hiphop", s.requireAuth(s.addFlag(tblNotHiphop)))
	mux.HandleFunc("DELETE /albums/{id}/not-hiphop", s.requireAuth(s.removeFlag(tblNotHiphop)))
	mux.HandleFunc("GET /rename-requests", s.requireAuth(s.listFlags(tblRename)))
	mux.HandleFunc("POST /albums/{id}/rename-request", s.requireAuth(s.addFlag(tblRename)))
	mux.HandleFunc("DELETE /albums/{id}/rename-request", s.requireAuth(s.removeFlag(tblRename)))

	// Comments — reads public, writing/deleting requires auth
	mux.HandleFunc("GET /albums/{id}/comments", s.listComments)
	mux.HandleFunc("POST /albums/{id}/comments", s.requireAuth(s.addComment))
	mux.HandleFunc("PUT /comments/{id}", s.requireAuth(s.updateComment))
	mux.HandleFunc("DELETE /comments/{id}", s.requireAuth(s.deleteComment))

	// Albums — reads public, writes admin-only
	mux.HandleFunc("GET /albums", s.listAlbums)
	mux.HandleFunc("GET /albums/years", s.listYears)
	mux.HandleFunc("POST /albums", s.requireAdmin(s.createAlbum))
	mux.HandleFunc("GET /albums/{id}", s.getAlbum)
	// 일반 사용자에겐 Spotify 임베드만 노출한다 — 우리 DB 트랙 목록은 관리자 편집용.
	mux.HandleFunc("GET /albums/{id}/tracks", s.requireAdmin(s.listAlbumTracks))
	mux.HandleFunc("PUT /albums/{id}/tracks/{trackId}/display-name", s.requireAdmin(s.updateTrackDisplayName))
	mux.HandleFunc("PUT /albums/{id}", s.requireAdmin(s.updateAlbum))
	mux.HandleFunc("PUT /albums/{id}/display-name", s.requireAdmin(s.updateAlbumDisplayName))
	mux.HandleFunc("DELETE /albums/{id}", s.requireAdmin(s.deleteAlbum))
	mux.HandleFunc("POST /albums/{id}/restore", s.requireAdmin(s.restoreAlbum))

	// Artists — reads public, writes admin-only
	mux.HandleFunc("GET /artists", s.listArtists)
	mux.HandleFunc("POST /artists", s.requireAdmin(s.createArtist))
	mux.HandleFunc("GET /artists/{id}", s.getArtist)
	mux.HandleFunc("PUT /artists/{id}", s.requireAdmin(s.updateArtist))
	mux.HandleFunc("PUT /artists/{id}/aliases", s.requireAdmin(s.updateArtistAliases))
	mux.HandleFunc("PUT /artists/{id}/display-name", s.requireAdmin(s.updateArtistDisplayName))
	mux.HandleFunc("POST /artists/merge", s.requireAdmin(s.mergeArtists))
	mux.HandleFunc("DELETE /artists/{id}", s.requireAdmin(s.deleteArtist))

	mux.HandleFunc("GET /admin/stats", s.requireAdmin(s.adminStats))
	mux.HandleFunc("GET /admin/not-hiphop", s.requireAdmin(s.adminListFlagged(tblNotHiphop)))
	mux.HandleFunc("DELETE /admin/not-hiphop/{id}", s.requireAdmin(s.adminClearFlags(tblNotHiphop)))
	mux.HandleFunc("GET /admin/rename-requests", s.requireAdmin(s.adminListFlagged(tblRename)))
	mux.HandleFunc("DELETE /admin/rename-requests/{id}", s.requireAdmin(s.adminClearFlags(tblRename)))

	// Admin crawling — Spotify search + pull a picked artist's albums into the DB
	mux.HandleFunc("GET /admin/spotify/keys", s.requireAdmin(s.adminSpotifyKeys))
	mux.HandleFunc("GET /admin/spotify/artists", s.requireAdmin(s.adminSpotifySearch))
	mux.HandleFunc("POST /admin/spotify/crawl", s.requireAdmin(s.adminSpotifyCrawl))

	// Admin 트랙 동기화 — batch-backfill track lists for albums that lack them
	mux.HandleFunc("GET /admin/tracks/status", s.requireAdmin(s.adminTracksStatus))
	mux.HandleFunc("POST /admin/tracks/backfill", s.requireAdmin(s.adminTracksBackfill))

	mux.HandleFunc("GET /openapi.json", serveSpec)
	mux.HandleFunc("GET /swagger/", swaggerUI)

	log.Printf("listening on :%s  (swagger: http://localhost:%s/swagger/)", port, port)
	srv := &http.Server{Addr: ":" + port, Handler: logRequests(mux), ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

// ensureAuthSchema creates the users/favorites tables and runs idempotent
// migrations on the crawler-owned albums/artists tables (safe to run on every boot).
func (s *server) ensureAuthSchema(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			nickname TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT 'user',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		-- Only the SHA-256 of each refresh token is stored; rotation revokes the
		-- old row on every use so a replayed token is detectable.
		CREATE TABLE IF NOT EXISTS refresh_tokens (
			id BIGSERIAL PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TIMESTAMPTZ NOT NULL,
			revoked_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);
		CREATE TABLE IF NOT EXISTS favorites (
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			album_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (user_id, album_id)
		);
		-- score is in half-stars: 1..10 == 0.5..5.0 stars.
		CREATE TABLE IF NOT EXISTS ratings (
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			album_id TEXT NOT NULL,
			score SMALLINT NOT NULL CHECK (score BETWEEN 1 AND 10),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (user_id, album_id)
		);
		-- PK is (user_id, album_id); the rating_avg/rating_count aggregates look up by album.
		CREATE INDEX IF NOT EXISTS idx_ratings_album ON ratings(album_id);
		-- artist_id has no FK: the crawler owns artists and rewrites it freely.
		CREATE TABLE IF NOT EXISTS follows (
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			artist_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (user_id, artist_id)
		);
		-- One row per user per album: the count is people, not clicks.
		CREATE TABLE IF NOT EXISTS not_hiphop_reports (
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			album_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (user_id, album_id)
		);
		CREATE INDEX IF NOT EXISTS idx_not_hiphop_album ON not_hiphop_reports(album_id);
		-- "앨범명 좀 바꿔주세요" — 같은 모양의 플래그 테이블. 한글 표시 이름이 필요한
		-- 앨범을 사용자가 눌러 쌓아두면 관리자가 모아 보고 display_name을 채운다.
		CREATE TABLE IF NOT EXISTS rename_requests (
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			album_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (user_id, album_id)
		);
		CREATE INDEX IF NOT EXISTS idx_rename_requests_album ON rename_requests(album_id);
		-- parent_id NULL = top-level; replies are capped at one level in the handler.
		CREATE TABLE IF NOT EXISTS comments (
			id BIGSERIAL PRIMARY KEY,
			album_id TEXT NOT NULL,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			parent_id BIGINT REFERENCES comments(id) ON DELETE CASCADE,
			body TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			deleted_at TIMESTAMPTZ
		);
		CREATE INDEX IF NOT EXISTS idx_comments_album ON comments(album_id, id);
		CREATE INDEX IF NOT EXISTS idx_comments_parent ON comments(parent_id);
		ALTER TABLE IF EXISTS comments ADD COLUMN IF NOT EXISTS edited_at TIMESTAMPTZ;
		ALTER TABLE IF EXISTS albums ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

		-- Normalize album↔artist into a many-to-many join (was albums.artist_id/artist_name).
		CREATE TABLE IF NOT EXISTS album_artists (
			album_id  TEXT NOT NULL,
			artist_id TEXT NOT NULL,
			position  INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (album_id, artist_id)
		);
		CREATE INDEX IF NOT EXISTS idx_album_artists_artist ON album_artists(artist_id);

		-- Per-album track lists pulled from Spotify (관리자 트랙 동기화). artists is a
		-- JSONB array [{id,name}] in credit order, features included — kept inline
		-- instead of normalized so feature-only artists don't pollute the artists table.
		CREATE TABLE IF NOT EXISTS tracks (
			album_id TEXT NOT NULL,
			id TEXT NOT NULL,
			disc_number INTEGER NOT NULL DEFAULT 1,
			track_number INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			duration_ms INTEGER,
			explicit BOOLEAN NOT NULL DEFAULT false,
			spotify_url TEXT,
			artists JSONB NOT NULL DEFAULT '[]',
			PRIMARY KEY (album_id, id)
		);
		CREATE INDEX IF NOT EXISTS idx_tracks_album ON tracks(album_id, disc_number, track_number);
		-- Set once a sync attempt completed (even with 0 tracks); NULL = 백필 대기.
		ALTER TABLE IF EXISTS albums ADD COLUMN IF NOT EXISTS tracks_synced_at TIMESTAMPTZ;
		-- 트랙 한글 표시 이름: admin-curated, 동기화(백필)는 절대 덮어쓰지 않는다.
		ALTER TABLE IF EXISTS tracks ADD COLUMN IF NOT EXISTS display_name TEXT;

		ALTER TABLE IF EXISTS artists ADD COLUMN IF NOT EXISTS image_url TEXT;
		ALTER TABLE IF EXISTS artists ADD COLUMN IF NOT EXISTS genres TEXT[];
		ALTER TABLE IF EXISTS artists ADD COLUMN IF NOT EXISTS spotify_url TEXT;
		ALTER TABLE IF EXISTS artists ADD COLUMN IF NOT EXISTS aliases TEXT[];
		ALTER TABLE IF EXISTS artists ADD COLUMN IF NOT EXISTS followers INTEGER;
		-- 한글 표시 이름: admin-curated, never written by the crawler (Spotify only
		-- returns the label-registered, usually English, name).
		ALTER TABLE IF EXISTS artists ADD COLUMN IF NOT EXISTS display_name TEXT;
		ALTER TABLE IF EXISTS albums ADD COLUMN IF NOT EXISTS display_name TEXT;

		-- Denormalized aggregates on albums. The list query sorts by these, so as
		-- correlated subqueries they were computed for every matching row before
		-- LIMIT; as columns the sort reads them straight off the row (and the index
		-- below). rating_sum is in half-stars, like ratings.score.
		ALTER TABLE IF EXISTS albums ADD COLUMN IF NOT EXISTS rating_count INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE IF EXISTS albums ADD COLUMN IF NOT EXISTS rating_sum INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE IF EXISTS albums ADD COLUMN IF NOT EXISTS comment_count INTEGER NOT NULL DEFAULT 0;
		CREATE INDEX IF NOT EXISTS idx_albums_popular ON albums (rating_count DESC, id);

		-- Counters are maintained by triggers, NOT by the handlers: ratings/comments
		-- also disappear through paths the API never sees — the users FK cascade and
		-- the bulk deletes in deleteArtist(mode=hard). A trigger catches all of them.
		CREATE OR REPLACE FUNCTION albums_rating_counters() RETURNS trigger AS $fn$
		BEGIN
			IF TG_OP = 'INSERT' THEN
				UPDATE albums SET rating_count = rating_count + 1, rating_sum = rating_sum + NEW.score
				 WHERE id = NEW.album_id;
			ELSIF TG_OP = 'UPDATE' THEN -- re-rating: the row stays, only the score moves
				UPDATE albums SET rating_sum = rating_sum - OLD.score + NEW.score
				 WHERE id = NEW.album_id;
			ELSE
				UPDATE albums SET rating_count = rating_count - 1, rating_sum = rating_sum - OLD.score
				 WHERE id = OLD.album_id;
			END IF;
			RETURN NULL;
		END $fn$ LANGUAGE plpgsql;

		-- Comments are soft-deleted, so "removed" is an UPDATE of deleted_at.
		CREATE OR REPLACE FUNCTION albums_comment_counters() RETURNS trigger AS $fn$
		BEGIN
			IF TG_OP = 'INSERT' THEN
				IF NEW.deleted_at IS NULL THEN
					UPDATE albums SET comment_count = comment_count + 1 WHERE id = NEW.album_id;
				END IF;
			ELSIF TG_OP = 'UPDATE' THEN
				IF OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL THEN
					UPDATE albums SET comment_count = comment_count - 1 WHERE id = NEW.album_id;
				ELSIF OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL THEN
					UPDATE albums SET comment_count = comment_count + 1 WHERE id = NEW.album_id;
				END IF;
			ELSIF OLD.deleted_at IS NULL THEN
				UPDATE albums SET comment_count = comment_count - 1 WHERE id = OLD.album_id;
			END IF;
			RETURN NULL;
		END $fn$ LANGUAGE plpgsql;

		CREATE OR REPLACE TRIGGER trg_ratings_counters
			AFTER INSERT OR UPDATE OF score OR DELETE ON ratings
			FOR EACH ROW EXECUTE FUNCTION albums_rating_counters();
		CREATE OR REPLACE TRIGGER trg_comments_counters
			AFTER INSERT OR UPDATE OF deleted_at OR DELETE ON comments
			FOR EACH ROW EXECUTE FUNCTION albums_comment_counters();

		-- Reconcile on boot: backfills the columns the first time and self-heals any
		-- drift afterwards (a trigger added mid-flight, a manual SQL fix). Touches
		-- only rows that are actually wrong, so a healthy DB writes nothing.
		-- ponytail: full albums scan per boot — move behind a version flag if the
		-- table ever grows past "starts in well under a second".
		UPDATE albums a SET rating_count = t.rc, rating_sum = t.rs, comment_count = t.cc
		FROM (
			SELECT al.id,
			       (SELECT count(*) FROM ratings r WHERE r.album_id = al.id)::int AS rc,
			       (SELECT COALESCE(sum(r.score), 0) FROM ratings r WHERE r.album_id = al.id)::int AS rs,
			       (SELECT count(*) FROM comments c WHERE c.album_id = al.id AND c.deleted_at IS NULL)::int AS cc
			FROM albums al
		) t
		WHERE a.id = t.id
		  AND (a.rating_count, a.rating_sum, a.comment_count) IS DISTINCT FROM (t.rc, t.rs, t.cc);

		-- Backfill the legacy single-artist column into the join table. We DON'T drop
		-- the old artist_id/artist_name columns: artist_name still holds the only record
		-- of featured artists (they never had IDs), so keeping it avoids data loss and
		-- lets an old API image roll back. We just relax NOT NULL so the normalized
		-- crawler/API can ignore them. A re-crawl repopulates album_artists with real
		-- IDs for every credited artist; the dead columns can be dropped later.
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.columns
			           WHERE table_name = 'albums' AND column_name = 'artist_id') THEN
				INSERT INTO album_artists(album_id, artist_id, position)
				SELECT id, artist_id, 0 FROM albums WHERE artist_id IS NOT NULL AND artist_id <> ''
				ON CONFLICT DO NOTHING;
				ALTER TABLE albums ALTER COLUMN artist_id DROP NOT NULL;
				ALTER TABLE albums ALTER COLUMN artist_name DROP NOT NULL;
			END IF;
		END $$;`)
	return err
}

func parseAdminIDs(csv string) map[string]bool {
	ids := map[string]bool{}
	for _, id := range strings.Split(csv, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids[id] = true
		}
	}
	return ids
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decode reads a JSON body into dst; on failure it writes 400 and returns false.
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

// clampPage reads limit/offset query params with sane defaults and bounds.
func clampPage(r *http.Request) (limit, offset int) {
	limit = queryInt(r, "limit", 50)
	if limit < 1 || limit > 200 {
		limit = 50
	}
	offset = queryInt(r, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	return
}

func queryInt(r *http.Request, key string, def int) int {
	if n, err := strconv.Atoi(r.URL.Query().Get(key)); err == nil {
		return n
	}
	return def
}

func queryIntPtr(r *http.Request, key string) *int {
	if n, err := strconv.Atoi(r.URL.Query().Get(key)); err == nil {
		return &n
	}
	return nil
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		log.Printf("%s %s", r.Method, r.URL.Path)
	})
}

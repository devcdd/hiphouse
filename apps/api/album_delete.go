package main

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// hardDeleteAlbums는 앨범 행과 album_id로 걸린 행을 모두 지우고 album_tombstones에 남긴다.
// favorites, ratings, comments 등은 albums FK가 없어서 직접 지워야 한다. awards만 FK CASCADE다.
// 반환값은 실제로 지운 앨범 수다.
func hardDeleteAlbums(ctx context.Context, tx pgx.Tx, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO album_tombstones(id, apple_id, name, artists)
		SELECT al.id, al.apple_id, COALESCE(al.display_name, al.name),
		       COALESCE((SELECT string_agg(COALESCE(ar.display_name, ar.name), ', ' ORDER BY aa.position)
		                 FROM album_artists aa JOIN artists ar ON ar.id = aa.artist_id
		                 WHERE aa.album_id = al.id), '')
		FROM albums al WHERE al.id = ANY($1)
		ON CONFLICT (id) DO NOTHING`, ids); err != nil {
		return 0, err
	}
	for _, q := range []string{
		"DELETE FROM favorites WHERE album_id = ANY($1)",
		"DELETE FROM ratings WHERE album_id = ANY($1)",
		"DELETE FROM comments WHERE album_id = ANY($1)",
		"DELETE FROM not_hiphop_reports WHERE album_id = ANY($1)",
		"DELETE FROM rename_requests WHERE album_id = ANY($1)",
		"DELETE FROM tracks WHERE album_id = ANY($1)",
		"DELETE FROM album_artists WHERE album_id = ANY($1)",
	} {
		if _, err := tx.Exec(ctx, q, ids); err != nil {
			return 0, err
		}
	}
	tag, err := tx.Exec(ctx, "DELETE FROM albums WHERE id = ANY($1)", ids)
	return tag.RowsAffected(), err
}

type albumTombstone struct {
	ID        string  `json:"id" db:"id"`
	AppleID   *string `json:"apple_id" db:"apple_id"`
	Name      string  `json:"name" db:"name"`
	Artists   string  `json:"artists" db:"artists"`
	DeletedAt string  `json:"deleted_at" db:"deleted_at"`
}

// GET /admin/album-tombstones — 최근에 지운 순.
func (s *server) listAlbumTombstones(w http.ResponseWriter, r *http.Request) {
	limit, offset := clampPage(r)
	rows, err := s.db.Query(r.Context(),
		`SELECT id, apple_id, name, artists, deleted_at::text AS deleted_at FROM album_tombstones
		 ORDER BY deleted_at DESC, id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByName[albumTombstone])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

// DELETE /admin/album-tombstones/{id} — 차단만 푼다. 앨범은 다음 크롤이나 수동 추가로 다시 들어온다.
func (s *server) deleteAlbumTombstone(w http.ResponseWriter, r *http.Request) {
	tag, err := s.db.Exec(r.Context(), "DELETE FROM album_tombstones WHERE id=$1", r.PathValue("id"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, 404, "tombstone not found")
		return
	}
	w.WriteHeader(204)
}

// clearTombstone은 관리자가 지웠던 앨범을 직접 다시 추가할 때 차단을 먼저 푼다.
func (s *server) clearTombstone(ctx context.Context, id, appleID string) error {
	_, err := s.db.Exec(ctx,
		"DELETE FROM album_tombstones WHERE id = $1 OR ($2 <> '' AND apple_id = $2)", id, appleID)
	return err
}

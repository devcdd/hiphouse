package main

import "net/http"

// AdminStats is a one-row snapshot of the catalogue and the activity on it.
type AdminStats struct {
	Albums        int64 `json:"albums"`         // 노출 중인 앨범
	DeletedAlbums int64 `json:"deleted_albums"` // 소프트 삭제된 앨범
	Tracks        int64 `json:"tracks"`         // 동기화된 트랙 (삭제 앨범 포함)
	Artists       int64 `json:"artists"`
	Users         int64 `json:"users"`
	Ratings       int64 `json:"ratings"`
	Comments      int64 `json:"comments"` // 삭제되지 않은 댓글
	Favorites     int64 `json:"favorites"`
	Follows       int64 `json:"follows"`
}

// adminStats: every count in one round trip — these are small tables and the
// page shows them together anyway.
func (s *server) adminStats(w http.ResponseWriter, r *http.Request) {
	var st AdminStats
	err := s.db.QueryRow(r.Context(), `
		SELECT (SELECT count(*) FROM albums WHERE deleted_at IS NULL),
		       (SELECT count(*) FROM albums WHERE deleted_at IS NOT NULL),
		       (SELECT count(*) FROM tracks),
		       (SELECT count(*) FROM artists),
		       (SELECT count(*) FROM users),
		       (SELECT count(*) FROM ratings),
		       (SELECT count(*) FROM comments WHERE deleted_at IS NULL),
		       (SELECT count(*) FROM favorites),
		       (SELECT count(*) FROM follows)`).
		Scan(&st.Albums, &st.DeletedAlbums, &st.Tracks, &st.Artists, &st.Users, &st.Ratings, &st.Comments, &st.Favorites, &st.Follows)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, st)
}

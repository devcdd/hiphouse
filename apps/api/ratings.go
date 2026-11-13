package main

import (
	"net/http"

	"github.com/jackc/pgx/v5"
)

// Rating is the current user's own score for an album. score counts half-stars
// (1..10), so 7 renders as 3.5 stars — an integer keeps the CHECK constraint and
// the client math trivial, and dodges float rounding on a 0.5 step.
type Rating struct {
	AlbumID string `json:"album_id" db:"album_id"`
	Score   int    `json:"score" db:"score"`
}

func (s *server) listRatings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(),
		"SELECT album_id, score FROM ratings WHERE user_id=$1", currentUser(r).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ratings, err := pgx.CollectRows(rows, pgx.RowToStructByName[Rating])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ratings)
}

// ratedOrderBy는 평가 목록(내 목록, 공개 프로필)의 정렬이다. 어느 정렬이든 최근 평가순으로 끝나고,
// 그래도 같으면 앨범 id로 가른다. 순서가 완전히 정해져야 offset으로 넘길 때 앨범이 겹치거나 빠지지 않는다.
// release_date는 ISO 텍스트라 문자열 정렬이 곧 날짜 정렬이다.
func ratedOrderBy(sort string) string {
	const recent = "rt.updated_at DESC, albums.id"
	switch sort {
	case "score":
		return "rt.score DESC, " + recent
	case "release":
		return "albums.release_date DESC NULLS LAST, albums.year DESC NULLS LAST, " + recent
	}
	return recent
}

// listRatedAlbums returns the full album rows the user has rated plus the score
// they gave — mirrors listFavorites so the my-page can render album cards.
// GET /ratings/albums?sort=
func (s *server) listRatedAlbums(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(),
		"SELECT "+albumSelectCols+", rt.score FROM albums JOIN ratings rt ON rt.album_id=albums.id "+
			"WHERE rt.user_id=$1 ORDER BY "+ratedOrderBy(r.URL.Query().Get("sort")), currentUser(r).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	albums, err := pgx.CollectRows(rows, pgx.RowToStructByName[RatedAlbum])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, albums)
}

func (s *server) putRating(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Score int `json:"score"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Score < 1 || body.Score > 10 {
		writeErr(w, 400, "score must be 1..10 (half-stars)")
		return
	}
	_, err := s.db.Exec(r.Context(),
		`INSERT INTO ratings(user_id,album_id,score) VALUES($1,$2,$3)
		 ON CONFLICT (user_id,album_id) DO UPDATE SET score=EXCLUDED.score, updated_at=now()`,
		currentUser(r).ID, r.PathValue("albumId"), body.Score)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *server) deleteRating(w http.ResponseWriter, r *http.Request) {
	_, err := s.db.Exec(r.Context(), "DELETE FROM ratings WHERE user_id=$1 AND album_id=$2",
		currentUser(r).ID, r.PathValue("albumId"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

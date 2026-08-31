package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Award는 관리자가 직접 넣는 수상 경력 한 건 — 앨범 또는 아티스트 중 정확히 하나에 붙는다.
type Award struct {
	ID   int64  `json:"id" db:"id"`
	Host string `json:"host" db:"host"` // 주최 (한국힙합어워즈, KMA …)
	Name string `json:"name" db:"name"` // 수상명 (올해의 앨범 …)
	Year *int   `json:"year" db:"year"`
}

// col은 "album_id" | "artist_id". SQL에 그대로 박히므로 라우트 등록부의 상수로만 넘긴다.
func (s *server) listAwards(col string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.db.Query(r.Context(),
			"SELECT id, host, name, year FROM awards WHERE "+col+"=$1 ORDER BY year DESC NULLS LAST, id",
			r.PathValue("id"))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		list, err := pgx.CollectRows(rows, pgx.RowToStructByName[Award])
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if list == nil {
			list = []Award{}
		}
		writeJSON(w, 200, list)
	}
}

func (s *server) addAward(col string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Host string `json:"host"`
			Name string `json:"name"`
			Year *int   `json:"year"`
		}
		if !decode(w, r, &body) {
			return
		}
		host, name := strings.TrimSpace(body.Host), strings.TrimSpace(body.Name)
		switch {
		case host == "":
			writeErr(w, 400, "host is required")
			return
		case name == "":
			writeErr(w, 400, "name is required")
			return
		case body.Year != nil && (*body.Year < 1900 || *body.Year > 2100):
			writeErr(w, 400, "year out of range")
			return
		}
		var a Award
		err := s.db.QueryRow(r.Context(),
			"INSERT INTO awards("+col+", host, name, year) VALUES($1,$2,$3,$4) RETURNING id, host, name, year",
			r.PathValue("id"), host, name, body.Year).Scan(&a.ID, &a.Host, &a.Name, &a.Year)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" { // FK — 없는 앨범/아티스트
			writeErr(w, 404, "not found")
			return
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 201, a)
	}
}

func (s *server) deleteAward(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid award id")
		return
	}
	tag, err := s.db.Exec(r.Context(), "DELETE FROM awards WHERE id=$1", id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, 404, "award not found")
		return
	}
	w.WriteHeader(204)
}

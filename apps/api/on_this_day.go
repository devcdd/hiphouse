package main

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

const onThisDayLimit = 30

var seoul = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// onThisDayDate는 ?date=YYYY-MM-DD를 읽고, 없으면 한국 날짜로 오늘을 쓴다.
// 서버는 UTC로 돌아서 기본값을 그대로 쓰면 KST 오전 9시 전까지 하루가 밀린다.
func onThisDayDate(raw string, now time.Time) (time.Time, bool) {
	if raw == "" {
		return now.In(seoul), true
	}
	d, err := time.Parse(time.DateOnly, raw)
	return d, err == nil
}

// onThisDayQuery는 같은 월·일에 나온 지난해 앨범을 수상작, 평가 수, 평균 별점 순으로 고른다.
// 평가가 없는 앨범이 대부분이라 그다음은 정규, EP, 싱글 순으로 세워 무게 있는 작품이 앞에 오게 한다.
// 일자까지 아는 앨범(release_date가 YYYY-MM-DD)만 대상이고, 올해 앨범은 기념일이 아니라서 뺀다.
// release_date는 ISO 문자열이라 "2016-11-18" < "2026"처럼 연도 문자열과 바로 비교된다.
func onThisDayQuery(d time.Time) (string, []any) {
	sql := "SELECT " + albumSelectCols + ` FROM albums
		WHERE deleted_at IS NULL
		  AND length(release_date) = 10
		  AND substr(release_date, 6) = $1
		  AND release_date < $2
		ORDER BY ` + awardedExpr + ` DESC, rating_count DESC, ` + ratingAvgExpr + ` DESC NULLS LAST,
		  CASE ` + effTypeExpr + ` WHEN 'album' THEN 0 WHEN 'ep' THEN 1 WHEN 'compilation' THEN 2 ELSE 3 END,
		  release_date DESC, name
		LIMIT $3`
	return sql, []any{d.Format("01-02"), d.Format("2006"), onThisDayLimit}
}

func (s *server) listOnThisDay(w http.ResponseWriter, r *http.Request) {
	d, ok := onThisDayDate(r.URL.Query().Get("date"), time.Now())
	if !ok {
		writeErr(w, 400, "date must be YYYY-MM-DD")
		return
	}
	sql, args := onThisDayQuery(d)
	rows, err := s.db.Query(r.Context(), sql, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	albums, err := pgx.CollectRows(rows, pgx.RowToStructByName[Album])
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, albums)
}

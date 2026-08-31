package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// DB 없이 검증 분기만 — 유효한 본문은 nil db에 닿아 패닉하므로 여기서 다루지 않는다.
func TestUpdateAlbumInfoRejectsBadBody(t *testing.T) {
	bad := []string{
		`{"name":""}`,
		`{"name":"x","release_date":"2020/01/01"}`,
		`{"name":"x","release_date":"20200101"}`,
		`{"name":"x","album_type":"ep"}`,
		`{"name":"x","total_tracks":0}`,
		`{"name":"x","unknown":1}`,
	}
	for _, body := range bad {
		req := httptest.NewRequest("PUT", "/albums/a1/info", strings.NewReader(body))
		req.SetPathValue("id", "a1")
		rec := httptest.NewRecorder()
		(&server{}).updateAlbumInfo(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", body, rec.Code)
		}
	}
	for _, ok := range []string{"2020", "2020-01", "2020-01-31"} {
		if !releaseDateRe.MatchString(ok) {
			t.Errorf("%s should be accepted", ok)
		}
	}
}

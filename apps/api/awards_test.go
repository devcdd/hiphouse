package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// DB 없이 검증 분기만 — 유효한 본문은 nil db에 닿아 패닉하므로 다루지 않는다.
func TestAddAwardRejectsBadBody(t *testing.T) {
	bad := []string{
		`{"host":"","name":"x"}`,
		`{"host":"x","name":" "}`,
		`{"host":"x","name":"y","year":1800}`,
		`{"host":"x","name":"y","year":2101}`,
		`{"host":"x","name":"y","extra":1}`,
	}
	h := (&server{}).addAward("album_id")
	for _, body := range bad {
		req := httptest.NewRequest("POST", "/albums/a1/awards", strings.NewReader(body))
		req.SetPathValue("id", "a1")
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", body, rec.Code)
		}
	}
	req := httptest.NewRequest("DELETE", "/awards/abc", nil)
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()
	(&server{}).deleteAward(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-numeric award id: got %d, want 400", rec.Code)
	}
}

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOnThisDayDateDefaultsToSeoul(t *testing.T) {
	// UTC 11/17 20:00은 서울로 11/18 05:00이다.
	now := time.Date(2026, 11, 17, 20, 0, 0, 0, time.UTC)
	d, ok := onThisDayDate("", now)
	if !ok || d.Format(time.DateOnly) != "2026-11-18" {
		t.Fatalf("got %v %v, want 2026-11-18", d, ok)
	}
}

func TestOnThisDayQueryArgs(t *testing.T) {
	d, ok := onThisDayDate("2026-03-05", time.Now())
	if !ok {
		t.Fatal("valid date rejected")
	}
	_, args := onThisDayQuery(d)
	if args[0] != "03-05" || args[1] != "2026" {
		t.Fatalf("got %v", args)
	}
}

func TestListOnThisDayRejectsBadDate(t *testing.T) {
	for _, raw := range []string{"2026-13-01", "11-18", "2026/11/18"} {
		req := httptest.NewRequest("GET", "/albums/on-this-day?date="+raw, nil)
		rec := httptest.NewRecorder()
		(&server{}).listOnThisDay(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", raw, rec.Code)
		}
	}
}

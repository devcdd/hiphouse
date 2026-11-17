package main

import (
	"testing"
	"time"
)

func TestNextSweep(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	at := func(h, m int) time.Time { return time.Date(2026, 11, 17, h, m, 0, 0, loc) }
	if got := nextSweep(at(6, 5)); !got.Equal(at(18, 10)) {
		t.Fatalf("06:05 다음은 18:10이어야 함: %s", got)
	}
	if got := nextSweep(at(18, 10)); !got.Equal(at(6, 5).AddDate(0, 0, 1)) {
		t.Fatalf("18:10 다음은 다음 날 06:05여야 함: %s", got)
	}
	prev := time.Time{}
	for i := range 20 {
		now := time.Date(2026, 8, 31, 0, 0, 0, 0, loc).Add(time.Duration(i) * 7 * time.Hour)
		got := nextSweep(now)
		if !got.After(now) {
			t.Fatalf("now=%s: 과거 시각 %s", now, got)
		}
		hm := got.Format("15:04")
		if hm != "06:05" && hm != "18:10" {
			t.Fatalf("now=%s: %s는 스윕 시각이 아님", now, got)
		}
		if d := got.Sub(now); d > 12*time.Hour+5*time.Minute {
			t.Fatalf("now=%s: 대기 %s가 스윕 간격 초과", now, d)
		}
		if !prev.IsZero() && got.Before(prev) {
			t.Fatalf("단조 증가 위반: %s < %s", got, prev)
		}
		prev = got
	}
}

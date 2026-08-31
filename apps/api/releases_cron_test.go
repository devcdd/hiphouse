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
	prev := time.Time{}
	for i := range 20 {
		now := time.Date(2026, 8, 31, 0, 0, 0, 0, loc).Add(time.Duration(i) * 7 * time.Hour)
		got := nextSweep(now)
		if !got.After(now) {
			t.Fatalf("now=%s: 과거 시각 %s", now, got)
		}
		if got.Hour() != sweepHour || got.Minute() != sweepMinute {
			t.Fatalf("now=%s: %s는 06:05가 아님", now, got)
		}
		if (got.Unix()/86400)%sweepDays != 0 {
			t.Fatalf("now=%s: %s는 이틀 주기가 아님", now, got)
		}
		if d := got.Sub(now); d > 48*time.Hour {
			t.Fatalf("now=%s: 대기 %s가 2일 초과", now, d)
		}
		if !prev.IsZero() && got.Before(prev) {
			t.Fatalf("단조 증가 위반: %s < %s", got, prev)
		}
		prev = got
	}
}

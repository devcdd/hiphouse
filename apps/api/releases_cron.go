package main

import (
	"context"
	"log"
	"time"
	_ "time/tzdata" // distroless 이미지엔 zoneinfo가 없다 — Asia/Seoul을 바이너리에 넣는다
)

// 서버 자체 신보 스윕: 이틀에 한 번 06:05 KST (신보가 00시 정각에 다 올라오진
// 않아 5분 여유). 관리자가 신보 체크 탭을 열지 않아도 돌게 하는 것이 목적이고,
// 탭은 수동 실행용으로 그대로 둔다.
const (
	sweepHour   = 6
	sweepMinute = 5
	sweepDays   = 2
	sweepBatch  = 20
)

// scheduleReleaseSweep은 다음 실행 시각까지 자고 스윕을 돌리길 반복한다.
func (s *server) scheduleReleaseSweep(ctx context.Context) {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		log.Printf("release sweep: Asia/Seoul 로드 실패, UTC로 진행: %v", err)
		loc = time.UTC
	}
	for {
		next := nextSweep(time.Now().In(loc))
		log.Printf("release sweep: 다음 실행 %s", next.Format(time.RFC3339))
		t := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		s.sweepReleases(ctx)
	}
}

// nextSweep은 now 이후 첫 sweepHour:sweepMinute 중 UTC 일련일이 sweepDays로
// 나눠떨어지는 날을 고른다. 재시작해도 같은 날짜에 걸리도록 벽시계만 보고 정한다 (상태 없음).
func nextSweep(now time.Time) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), sweepHour, sweepMinute, 0, 0, now.Location())
	for !t.After(now) || (t.Unix()/86400)%sweepDays != 0 {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// sweepReleases는 남은 대상이 0이 될 때까지 배치를 반복한다. 키 하나가 쿼터를
// 다 쓰면 (배치가 error를 달고 돌아옴) 다음 키로 넘어가 이어서 채운다.
func (s *server) sweepReleases(ctx context.Context) {
	keys := configuredSpotifyKeys()
	if len(keys) == 0 {
		keys = []string{""} // 접두사 없는 기본 자격증명 한 쌍
	}
	checked, albums := 0, 0
	for _, key := range keys {
		for {
			res, err := s.checkReleasesBatch(ctx, key, sweepBatch)
			checked += res.Checked
			albums += res.NewAlbums
			if err != nil {
				log.Printf("release sweep: DB 오류로 중단: %v", err)
				return
			}
			for _, a := range res.Artists {
				name := a.ID
				if a.Name != nil {
					name = *a.Name
				}
				log.Printf("release sweep: %s — 새 앨범 %d개: %v", name, len(a.Albums), a.Albums)
			}
			if res.Error != nil {
				log.Printf("release sweep: 키 %q 중단(%s) — 다음 키로", key, *res.Error)
				break
			}
			if res.Remaining == 0 {
				log.Printf("release sweep: 완료 — %d명 확인, 새 앨범 %d개", checked, albums)
				return
			}
			if res.Checked == 0 {
				break // 진행이 없으면 같은 키로 더 돌려봐야 소용없다
			}
		}
	}
	log.Printf("release sweep: 키 소진 — %d명 확인, 새 앨범 %d개", checked, albums)
}

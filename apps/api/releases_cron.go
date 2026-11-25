package main

import (
	"context"
	"log"
	"time"
	_ "time/tzdata" // distroless 이미지엔 zoneinfo가 없다 — Asia/Seoul을 바이너리에 넣는다
)

// 서버 자체 신보 스윕: 매일 18:10, 18:20 KST. 국내 음원은 대부분 18시에 공개되는데
// Apple 카탈로그에 늦게 올라오는 앨범이 있어서 10분 뒤에 한 번 더 본다. 관리자가 신보 체크
// 탭을 열지 않아도 돌게 하는 것이 목적이고, 탭은 수동 실행용으로 그대로 둔다.
const sweepBatch = 20

var sweepTimes = [][2]int{{18, 10}, {18, 20}}

// scheduleReleaseSweep은 다음 실행 시각까지 자고 스윕을 돌리길 반복한다.
func (s *server) scheduleReleaseSweep(ctx context.Context) {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		log.Printf("release sweep: Asia/Seoul 로드 실패, UTC로 진행: %v", err)
		loc = time.UTC
	}
	// 다음 시각은 직전 예정 시각에서 고른다. 18:10 스윕이 18:20을 넘겨 끝나도 18:20 스윕을 건너뛰지 않고 바로 돈다.
	next := time.Now().In(loc)
	// 앞선 스윕이 찾은 앨범은 모아 두었다가 그날 마지막 스윕에서 메일 한 통으로 보낸다.
	// 메모리에만 두므로 그 사이에 재시작하면 앞선 스윕분은 메일에서 빠진다.
	var pending []string
	for {
		next = nextSweep(next)
		log.Printf("release sweep: 다음 실행 %s", next.Format(time.RFC3339))
		t := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		// Spotify 계정이 막혀 신보 감지는 Apple로 돈다. 복구되면 sweepReleases로 되돌린다.
		pending = append(pending, s.sweepAppleReleases(ctx)...)
		s.sweepApple(ctx)
		if isLastSweep(next) {
			s.notifyNewAlbums(ctx, pending)
			pending = nil
		}
	}
}

func isLastSweep(t time.Time) bool {
	last := sweepTimes[len(sweepTimes)-1]
	return t.Hour() == last[0] && t.Minute() == last[1]
}

// nextSweep은 now 이후 가장 가까운 sweepTimes 시각을 고른다. 벽시계만 보고 정해서
// 재시작해도 실행 시각이 밀리지 않는다 (상태 없음).
func nextSweep(now time.Time) time.Time {
	var best time.Time
	for _, hm := range sweepTimes {
		t := time.Date(now.Year(), now.Month(), now.Day(), hm[0], hm[1], 0, 0, now.Location())
		if !t.After(now) {
			t = t.AddDate(0, 0, 1)
		}
		if best.IsZero() || t.Before(best) {
			best = t
		}
	}
	return best
}

// sweepReleases는 남은 대상이 0이 될 때까지 배치를 반복한다. 키 하나가 쿼터를
// 다 쓰면 (배치가 error를 달고 돌아옴) 다음 키로 넘어가 이어서 채운다.
func (s *server) sweepReleases(ctx context.Context) {
	keys := configuredSpotifyKeys()
	if len(keys) == 0 {
		keys = []string{""} // 접두사 없는 기본 자격증명 한 쌍
	}
	start := time.Now()
	checked, albums := 0, 0
	for _, key := range keys {
		for {
			res, err := s.checkReleasesBatch(ctx, key, sweepBatch, start)
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

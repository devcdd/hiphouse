package main

import (
	"strings"
	"testing"
)

// 프로필 비공개의 강제 지점은 이 가드 한 줄뿐이다. 쿼리를 손보다 조건을 흘리면
// 비공개로 돌린 사용자의 평가가 그대로 공개 프로필에 다시 뜬다.
func TestPublicRatedAlbumsGuardsPrivacy(t *testing.T) {
	if !strings.Contains(publicRatedAlbumsSQL, profilePrivacyGuard) {
		t.Fatalf("privacy guard missing from the public ratings query: %s", publicRatedAlbumsSQL)
	}
	// 가드는 프로필 주인($1)에 걸려야 한다 — 다른 자리 값에 걸리면 항상 통과한다.
	if !strings.Contains(profilePrivacyGuard, "pu.id = $1") || !strings.Contains(profilePrivacyGuard, "pu.profile_public") {
		t.Fatalf("guard must bind the profile owner to their own flag: %s", profilePrivacyGuard)
	}
	// AND로 묶여야 한다. OR면 가드가 필터를 넓혀 삭제된 앨범까지 새어나온다.
	if !strings.Contains(publicRatedAlbumsSQL, "albums.deleted_at IS NULL AND "+profilePrivacyGuard) {
		t.Fatalf("guard must be ANDed into the WHERE clause: %s", publicRatedAlbumsSQL)
	}
}

// 헤더의 활동 집계도 전부 플래그 뒤에 있어야 한다. 하나라도 CASE 밖으로 새면
// 목록은 비어 있는데 "평가 37개"가 찍히는, 비공개가 안 된 프로필이 된다.
func TestPublicUserHidesAggregatesWhenPrivate(t *testing.T) {
	for _, agg := range []string{"rating_count", "rating_avg", "comment_count"} {
		i := strings.Index(publicUserSQL, "AS "+agg)
		if i < 0 {
			t.Fatalf("publicUserSQL missing %s", agg)
		}
		// 이 집계 앞에 가장 가까운 CASE가 플래그를 보고 있어야 한다.
		before := publicUserSQL[:i]
		if j := strings.LastIndex(before, "CASE WHEN"); j < 0 || !strings.Contains(before[j:], "u.profile_public") {
			t.Errorf("%s is not gated by profile_public: %s", agg, before[max(0, len(before)-200):])
		}
	}
	// 닉네임·가입일은 반대로 항상 나가야 한다 — 프로필이 통째로 404가 되면 안 된다.
	for _, always := range []string{"u.nickname", "AS created_at", "u.profile_public"} {
		if !strings.Contains(publicUserSQL, always) {
			t.Errorf("publicUserSQL must always select %s", always)
		}
	}
}

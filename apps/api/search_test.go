package main

import (
	"regexp"
	"strings"
	"testing"
)

// pgToRE2는 Postgres 정규식을 Go regexp로 옮긴다. POSIX 문자 클래스와 \m은 RE2와 문법이 달라서 바꿔 끼운다.
func pgToRE2(t *testing.T, v string) *regexp.Regexp {
	t.Helper()
	v = strings.ReplaceAll(v, searchSep, `[\s\p{P}\p{S}]*`)
	v = strings.ReplaceAll(v, `[[:space:]]`, `\s`)
	v = strings.ReplaceAll(v, `\m`, `(?:^|[^\pL\pN])`)
	re, err := regexp.Compile("(?i)" + v)
	if err != nil {
		t.Fatalf("pattern %q: %v", v, err)
	}
	return re
}

func TestChosungClass(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		want string
	}{
		{'ㄱ', "[ㄱ가-깋]"},
		{'ㅂ', "[ㅂ바-빟]"},
		{'ㅎ', "[ㅎ하-힣]"},
	} {
		if got, ok := chosungClass(tc.r); !ok || got != tc.want {
			t.Errorf("chosungClass(%q) = %q %v, want %q", tc.r, got, ok, tc.want)
		}
	}
	// 종성에만 쓰는 겹자음과 모음, 완성된 음절은 초성이 아니다.
	for _, r := range "ㄳㅏ빈a" {
		if _, ok := chosungClass(r); ok {
			t.Errorf("chosungClass(%q) should not be a 초성", r)
		}
	}
}

func TestSearchMatchChosung(t *testing.T) {
	for _, tc := range []struct {
		q, name string
		want    bool
	}{
		{"ㅂㅈㄴ", "빈지노", true},
		{"ㅂㅈㄴ", "빈 지노", true},
		{"ㅈㅈㅍ", "재지팩트", true},
		{"빈지ㄴ", "빈지노", true},
		{"ㅂㅈㄴ", "빈지", false},
		{"ㅂㅈㄴ", "박재범", false},
		{"ㅋㅋ", "ㅋㅋ", true},
		{"ㅋㅋ", "쿠키", true},
		// 한 글자뿐이면 초성으로 풀지 않는다.
		{"ㅂ", "빈지노", false},
		{"ㅂ", "ㅂ", true},
	} {
		_, val := searchMatch(tc.q)
		if got := pgToRE2(t, val).MatchString(tc.name); got != tc.want {
			t.Errorf("%q vs %q = %v, want %v (pattern %q)", tc.q, tc.name, got, tc.want, val)
		}
	}
}

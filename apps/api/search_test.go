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

func TestSearchAnchors(t *testing.T) {
	for _, tc := range []struct {
		q, name       string
		exact, prefix bool
	}{
		{"nowitzki", "NOWITZKI", true, true},
		{"sweat", "SWEAT (Prod. JINBO)", true, true},
		{"24:26", "24:26 (5th Anniversary Remaster Edition)", true, true},
		{"ss pop", "SS-POP 3", false, true},
		{"sspop3", "SS-POP 3", true, true},
		{"빈지노", "빈지노", true, true},
		{"사랑", "사랑은 은하수 다방에서", false, true},
		{"심야", "김심야", false, false},
		// \m은 단어 시작이면 어디서든 맞지만 순위는 이름 맨 앞에서만 올린다.
		{"dragon", "G-DRAGON", false, false},
		{"ㅂㅈㄴ", "빈지노", true, true},
		{"!!!", "!!!", true, true},
	} {
		_, match := searchMatch(tc.q)
		exact, prefix := searchAnchors(match)
		if got := pgToRE2(t, exact).MatchString(tc.name); got != tc.exact {
			t.Errorf("exact %q vs %q = %v, want %v", tc.q, tc.name, got, tc.exact)
		}
		if got := pgToRE2(t, prefix).MatchString(tc.name); got != tc.prefix {
			t.Errorf("prefix %q vs %q = %v, want %v", tc.q, tc.name, got, tc.prefix)
		}
	}
}

func TestBuildArtistListQuery(t *testing.T) {
	sql, args := buildArtistListQuery("", 20, 40)
	if !eq(args, []any{20, 40}) || !strings.HasSuffix(sql, "FROM artists ORDER BY COALESCE(display_name, name) LIMIT $1 OFFSET $2") {
		t.Errorf("no query: %v %s", args, sql)
	}

	sql, args = buildArtistListQuery("dre", 20, 0)
	_, match := searchMatch("dre")
	exact, prefix := searchAnchors(match)
	if !eq(args, []any{match, exact, prefix, "dre", 20, 0}) {
		t.Fatalf("args = %v", args)
	}
	for _, frag := range []string{
		// 이름, 한글 표시 이름, 연관검색어를 모두 본다.
		"WHERE ar.name ~* $1 OR ar.display_name ~* $1 OR EXISTS (SELECT 1 FROM unnest(COALESCE(ar.aliases,'{}'::text[])) AS al WHERE al ~* $1)",
		"CASE WHEN ar.name ~* $2", "THEN 1 WHEN ar.name ~* $3", "THEN 3 ELSE 5 END AS rank",
		// 연관검색어로만 맞은 아티스트는 제 이름으로 맞은 아티스트 뒤에 온다.
		"(ar.name ~* $1 OR ar.display_name ~* $1) IS TRUE AS own",
		"JOIN hit ON hit.artist_id = artists.id ORDER BY hit.rank, hit.dist, hit.own DESC, COALESCE(display_name, name) LIMIT $5 OFFSET $6",
		"FROM q, artists ar WHERE q.j <% ar.search_jamo AND NOT EXISTS (SELECT 1 FROM matched)",
	} {
		if !strings.Contains(sql, frag) {
			t.Errorf("missing %q in %s", frag, sql)
		}
	}
}

// 저장된 열과 검색 패턴이 같은 문자를 무시해야 한다. 한쪽만 바뀌면 오타 대비가 조용히 어긋난다.
func TestSearchDDLUsesSearchSep(t *testing.T) {
	if !strings.Contains(searchDDL, "regexp_replace(s, '"+strings.TrimSuffix(searchSep, "*")+"+', '', 'g')") {
		t.Errorf("to_search_jamo should strip the searchSep class: %s", searchDDL)
	}
}

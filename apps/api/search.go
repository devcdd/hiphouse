package main

import (
	"fmt"
	"strconv"
	"strings"
)

// chosung은 초성 열아홉 자를 유니코드 한글 음절이 놓인 순서대로 적은 것이다.
var chosung = []rune("ㄱㄲㄴㄷㄸㄹㅁㅂㅃㅅㅆㅇㅈㅉㅊㅋㅌㅍㅎ")

// 한글 음절은 U+AC00부터 초성마다 588자씩(중성 21 × 종성 28) 이어진다.
const (
	hangulBase      = 0xAC00
	syllablesPerCho = 588
)

// chosungClass는 초성 자모를 그 초성으로 시작하는 모든 음절과 맞는 문자 클래스로 바꾼다.
// "ㅂㅈㄴ"으로 "빈지노"를 찾는 초성 검색에 쓴다. 자모를 그대로 쓴 이름("ㅋㅋ")도 맞도록 자모 자신도 넣는다.
func chosungClass(r rune) (string, bool) {
	for i, c := range chosung {
		if c == r {
			first := rune(hangulBase + i*syllablesPerCho)
			return "[" + string(r) + string(first) + "-" + string(first+syllablesPerCho-1) + "]", true
		}
	}
	return "", false
}

// 검색 결과 순위. 작을수록 위에 온다. 이름이 통째로 맞은 것이 앞부분만 맞은 것보다, 앨범과 아티스트
// 이름이 수록곡 이름보다 앞선다. 인기순만으로 세우면 제목이 정확히 맞는 앨범이 수록곡 이름만 스친
// 유명 앨범 뒤로 밀린다. 같은 순위 안에서는 수상 여부와 요청한 정렬을 따른다.
const (
	rankAlbumExact = iota
	rankArtistExact
	rankAlbumPrefix
	rankArtistPrefix
	rankTrackExact
	rankNamePartial
	rankTrackPartial
)

// searchAnchors는 searchMatch 패턴을 이름 맨 앞에 고정한 패턴 둘로 바꾼다. exact는 이름이 거기서 끝나야
// 하고 prefix는 뒤에 무엇이 와도 된다. 뒤에 괄호가 붙은 이름("SWEAT (Prod. JINBO)", "24:26 (Remaster)")은
// 같은 이름으로 친다.
func searchAnchors(match string) (exact, prefix string) {
	prefix = "^" + searchSep + strings.TrimPrefix(match, `\m`)
	return prefix + "(" + searchSep + `$|[[:space:]]*[[(])`, prefix
}

// searchPats는 검색어 하나에서 나온 패턴 셋의 자리표시자다.
type searchPats struct{ op, match, exact, prefix string }

func newSearchPats(q string, args *[]any) searchPats {
	op, match := searchMatch(q)
	exact, prefix := searchAnchors(match)
	*args = append(*args, match, exact, prefix)
	n := len(*args)
	return searchPats{op, "$" + strconv.Itoa(n-2), "$" + strconv.Itoa(n-1), "$" + strconv.Itoa(n)}
}

// names는 원본명이나 한글 표시 이름이 pat과 맞는지 보는 조건이다.
func (s searchPats) names(table, pat string) string {
	return table + ".name" + s.op + pat + " OR " + table + ".display_name" + s.op + pat
}

// artist는 names에 관리자가 넣은 연관검색어까지 본다. "블랙넛"으로 "Black Nut"을 찾는 길이다.
func (s searchPats) artist(pat string) string {
	return s.names("ar", pat) +
		" OR EXISTS (SELECT 1 FROM unnest(COALESCE(ar.aliases,'{}'::text[])) AS al WHERE al" + s.op + pat + ")"
}

func (s searchPats) rank(cond func(pat string) string, exact, prefix, partial int) string {
	return fmt.Sprintf("CASE WHEN %s THEN %d WHEN %s THEN %d ELSE %d END", cond(s.exact), exact, cond(s.prefix), prefix, partial)
}

// albumHitsCTE는 검색어와 맞는 앨범에 순위를 매긴 hit(album_id, rank)를 만드는 WITH 절이다.
// 앨범 이름, 크레딧 아티스트 이름, 수록곡 이름을 따로 찾아 앨범마다 가장 좋은 순위를 남긴다.
func albumHitsCTE(q string, args *[]any) string {
	s := newSearchPats(q, args)
	album := func(pat string) string { return s.names("albums", pat) }
	track := func(pat string) string { return s.names("t", pat) }
	return "WITH matched AS (" +
		"SELECT albums.id AS album_id, " + s.rank(album, rankAlbumExact, rankAlbumPrefix, rankNamePartial) + " AS rank" +
		" FROM albums WHERE " + album(s.match) +
		" UNION ALL SELECT aa.album_id, " + s.rank(s.artist, rankArtistExact, rankArtistPrefix, rankNamePartial) +
		" FROM artists ar JOIN album_artists aa ON aa.artist_id = ar.id WHERE " + s.artist(s.match) +
		" UNION ALL SELECT t.album_id, " + s.rank(track, rankTrackExact, rankTrackPartial, rankTrackPartial) +
		" FROM tracks t WHERE " + track(s.match) +
		"), hit AS (SELECT album_id, min(rank) AS rank FROM matched GROUP BY album_id)"
}

// artistHitsCTE는 검색어와 맞는 아티스트에 순위를 매긴 hit(artist_id, rank, own)를 만드는 WITH 절이다.
// own은 연관검색어가 아닌 제 이름으로 맞았는지다. "빈지노"를 연관검색어로 둔 소속 크루와 레이블이
// 빈지노 본인보다 앞에 오지 않게 한다.
func artistHitsCTE(q string, args *[]any) string {
	s := newSearchPats(q, args)
	return "WITH hit AS (" +
		"SELECT ar.id AS artist_id, " + s.rank(s.artist, rankArtistExact, rankArtistPrefix, rankNamePartial) + " AS rank" +
		", (" + s.names("ar", s.match) + ") IS TRUE AS own" +
		" FROM artists ar WHERE " + s.artist(s.match) + ")"
}

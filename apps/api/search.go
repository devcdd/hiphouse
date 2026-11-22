package main

import (
	"context"
	"crypto/sha256"
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
	// 아래 둘은 오타 대비용이다. 위 순위로 맞은 것이 하나도 없을 때만 쓴다.
	rankFuzzyName
	rankFuzzyTrack
)

// 오타로 볼 최소 유사도(pg_trgm word_similarity). 기본값 0.6으로는 "이쎈스"나 "씨젬"처럼 짧은 이름의
// 한 글자 오타를 놓친다. 이보다 낮추면 짧은 영문 검색어에 무관한 이름이 딸려 온다.
const searchFuzzyThreshold = "0.45"

// 이보다 짧은 검색어는 오타 대비를 하지 않는다. 자모 네 개 이하는 트라이그램이 너무 적어서
// 첫 두 글자만 같은 이름이 전부 맞는다.
const searchFuzzyMinLen = 5

// searchAnchors는 searchMatch 패턴을 이름 맨 앞에 고정한 패턴 둘로 바꾼다. exact는 이름이 거기서 끝나야
// 하고 prefix는 뒤에 무엇이 와도 된다. 뒤에 괄호가 붙은 이름("SWEAT (Prod. JINBO)", "24:26 (Remaster)")은
// 같은 이름으로 친다.
func searchAnchors(match string) (exact, prefix string) {
	prefix = "^" + searchSep + strings.TrimPrefix(match, `\m`)
	return prefix + "(" + searchSep + `$|[[:space:]]*[[(])`, prefix
}

// searchPats는 검색어 하나에서 나온 패턴 셋과 검색어 원문의 자리표시자다.
type searchPats struct{ op, match, exact, prefix, raw string }

func newSearchPats(q string, args *[]any) searchPats {
	op, match := searchMatch(q)
	exact, prefix := searchAnchors(match)
	*args = append(*args, match, exact, prefix, q)
	n := len(*args)
	ph := func(i int) string { return "$" + strconv.Itoa(i) }
	return searchPats{op, ph(n - 3), ph(n - 2), ph(n - 1), ph(n)}
}

// jamoCTE는 검색어를 저장된 search_jamo 열과 같은 꼴로 바꿔 둔다. 행마다 다시 계산하지 않게 한 번만 만든다.
func (s searchPats) jamoCTE() string {
	return "q AS MATERIALIZED (SELECT to_search_jamo(ARRAY[" + s.raw + "::text]) AS j)"
}

// fuzzyGate는 오타 대비를 켤 조건이다. 제대로 맞은 결과가 있으면 비슷한 이름을 섞지 않는다.
var fuzzyGate = "NOT EXISTS (SELECT 1 FROM matched) AND (SELECT length(j) >= " + strconv.Itoa(searchFuzzyMinLen) + " FROM q)"

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

// albumHitsCTE는 검색어와 맞는 앨범에 순위를 매긴 hit(album_id, rank, dist)를 만드는 WITH 절이다.
// 앨범 이름, 크레딧 아티스트 이름, 수록곡 이름을 따로 찾아 앨범마다 가장 좋은 순위를 남긴다.
// 하나도 못 찾으면 오타로 보고 자모 단위로 비슷한 이름을 찾는다. dist는 그때의 거리(0이 같음)다.
func albumHitsCTE(q string, args *[]any) string {
	s := newSearchPats(q, args)
	album := func(pat string) string { return s.names("albums", pat) }
	track := func(pat string) string { return s.names("t", pat) }
	near := func(table string, rank int) string {
		return fmt.Sprintf(", %d, q.j <<-> %s.search_jamo FROM q, ", rank, table)
	}
	return "WITH " + s.jamoCTE() + ", matched AS (" +
		"SELECT albums.id AS album_id, " + s.rank(album, rankAlbumExact, rankAlbumPrefix, rankNamePartial) + " AS rank, 0::real AS dist" +
		" FROM albums WHERE " + album(s.match) +
		" UNION ALL SELECT aa.album_id, " + s.rank(s.artist, rankArtistExact, rankArtistPrefix, rankNamePartial) + ", 0" +
		" FROM artists ar JOIN album_artists aa ON aa.artist_id = ar.id WHERE " + s.artist(s.match) +
		" UNION ALL SELECT t.album_id, " + s.rank(track, rankTrackExact, rankTrackPartial, rankTrackPartial) + ", 0" +
		" FROM tracks t WHERE " + track(s.match) +
		"), fuzzy AS (" +
		"SELECT albums.id" + near("albums", rankFuzzyName) + "albums WHERE q.j <% albums.search_jamo" +
		" UNION ALL SELECT aa.album_id" + near("ar", rankFuzzyName) + "artists ar JOIN album_artists aa ON aa.artist_id = ar.id WHERE q.j <% ar.search_jamo" +
		" UNION ALL SELECT t.album_id" + near("t", rankFuzzyTrack) + "tracks t WHERE q.j <% t.search_jamo" +
		"), hit AS (SELECT album_id, min(rank) AS rank, min(dist) AS dist FROM (" +
		"SELECT * FROM matched UNION ALL SELECT * FROM fuzzy WHERE " + fuzzyGate +
		") found GROUP BY album_id)"
}

// artistHitsCTE는 검색어와 맞는 아티스트에 순위를 매긴 hit(artist_id, rank, own, dist)를 만드는 WITH 절이다.
// own은 연관검색어가 아닌 제 이름으로 맞았는지다. "빈지노"를 연관검색어로 둔 소속 크루와 레이블이
// 빈지노 본인보다 앞에 오지 않게 한다.
func artistHitsCTE(q string, args *[]any) string {
	s := newSearchPats(q, args)
	return "WITH " + s.jamoCTE() + ", matched AS (" +
		"SELECT ar.id AS artist_id, " + s.rank(s.artist, rankArtistExact, rankArtistPrefix, rankNamePartial) + " AS rank" +
		", (" + s.names("ar", s.match) + ") IS TRUE AS own, 0::real AS dist" +
		" FROM artists ar WHERE " + s.artist(s.match) +
		"), hit AS (SELECT * FROM matched UNION ALL " +
		fmt.Sprintf("SELECT ar.id, %d, false, q.j <<-> ar.search_jamo FROM q, artists ar", rankFuzzyName) +
		" WHERE q.j <% ar.search_jamo AND " + fuzzyGate + ")"
}

// searchDDL은 오타 대비에 쓰는 search_jamo 열을 만든다. 이름들에서 공백과 문장부호를 빼고 소문자로 바꾼 뒤
// 한글 음절을 자모로 풀어 공백으로 이은 값이다. 한글은 음절 단위로 비교하면 한 글자만 틀려도 겹치는
// 트라이그램이 거의 없어서("빈지노"와 "빈지뇨") 자모로 풀어야 오타가 잡힌다. 겹모음과 겹받침은
// 두벌식으로 치는 순서대로 한 번 더 푼다.
var searchDDL = `
	CREATE EXTENSION IF NOT EXISTS pg_trgm;
	ALTER TABLE albums  DROP COLUMN IF EXISTS search_jamo;
	ALTER TABLE artists DROP COLUMN IF EXISTS search_jamo;
	ALTER TABLE tracks  DROP COLUMN IF EXISTS search_jamo;

	CREATE OR REPLACE FUNCTION to_search_jamo(names text[]) RETURNS text AS $fn$
	DECLARE
		cho  CONSTANT text[] := ARRAY['ㄱ','ㄲ','ㄴ','ㄷ','ㄸ','ㄹ','ㅁ','ㅂ','ㅃ','ㅅ','ㅆ','ㅇ','ㅈ','ㅉ','ㅊ','ㅋ','ㅌ','ㅍ','ㅎ'];
		jung CONSTANT text[] := ARRAY['ㅏ','ㅐ','ㅑ','ㅒ','ㅓ','ㅔ','ㅕ','ㅖ','ㅗ','ㅗㅏ','ㅗㅐ','ㅗㅣ','ㅛ','ㅜ','ㅜㅓ','ㅜㅔ','ㅜㅣ','ㅠ','ㅡ','ㅡㅣ','ㅣ'];
		jong CONSTANT text[] := ARRAY['','ㄱ','ㄲ','ㄱㅅ','ㄴ','ㄴㅈ','ㄴㅎ','ㄷ','ㄹ','ㄹㄱ','ㄹㅁ','ㄹㅂ','ㄹㅅ','ㄹㅌ','ㄹㅍ','ㄹㅎ','ㅁ','ㅂ','ㅂㅅ','ㅅ','ㅆ','ㅇ','ㅈ','ㅊ','ㅋ','ㅌ','ㅍ','ㅎ'];
		doc text := '';
		s text;
		c text;
		code int;
	BEGIN
		FOREACH s IN ARRAY names LOOP
			s := lower(regexp_replace(s, '` + strings.TrimSuffix(searchSep, "*") + `+', '', 'g'));
			CONTINUE WHEN s IS NULL OR s = '';
			doc := doc || ' ';
			-- 한글이 없는 이름이 더 많아서 글자마다 도는 일을 건너뛴다.
			IF s !~ '[가-힣]' THEN
				doc := doc || s;
				CONTINUE;
			END IF;
			FOREACH c IN ARRAY string_to_array(s, NULL) LOOP
				code := ascii(c) - 44032;
				IF code BETWEEN 0 AND 11171 THEN
					doc := doc || cho[code / 588 + 1] || jung[code % 588 / 28 + 1] || jong[code % 28 + 1];
				ELSE
					doc := doc || c;
				END IF;
			END LOOP;
		END LOOP;
		RETURN ltrim(doc);
	END $fn$ LANGUAGE plpgsql IMMUTABLE;

	ALTER TABLE albums  ADD COLUMN search_jamo text GENERATED ALWAYS AS (to_search_jamo(ARRAY[name, display_name])) STORED;
	ALTER TABLE artists ADD COLUMN search_jamo text GENERATED ALWAYS AS (to_search_jamo(ARRAY[name, display_name] || aliases)) STORED;
	ALTER TABLE tracks  ADD COLUMN search_jamo text GENERATED ALWAYS AS (to_search_jamo(ARRAY[name, display_name])) STORED;
	CREATE INDEX idx_albums_search_jamo  ON albums  USING gin (search_jamo gin_trgm_ops);
	CREATE INDEX idx_artists_search_jamo ON artists USING gin (search_jamo gin_trgm_ops);
	CREATE INDEX idx_tracks_search_jamo  ON tracks  USING gin (search_jamo gin_trgm_ops);
`

// ensureSearchSchema는 searchDDL이 바뀌었을 때만 search_jamo 열을 다시 만든다. 생성 열은 함수 본문을
// 고쳐도 저장된 값이 그대로라서, DDL의 해시를 함수 주석에 적어 두고 다르면 열을 지우고 새로 계산한다.
func (s *server) ensureSearchSchema(ctx context.Context) error {
	want := fmt.Sprintf("%x", sha256.Sum256([]byte(searchDDL)))[:16]
	var have *string
	err := s.db.QueryRow(ctx, `SELECT obj_description(to_regprocedure('to_search_jamo(text[])'), 'pg_proc')`).Scan(&have)
	if err != nil {
		return err
	}
	if have != nil && *have == want {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, searchDDL); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "COMMENT ON FUNCTION to_search_jamo(text[]) IS '"+want+"'"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

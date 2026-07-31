package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The album_artists subquery returns json shaped like this; pgx's json codec
// json.Unmarshals it straight into []AlbumArtist. Lock that contract + the wire shape.
func TestAlbumArtistsJSON(t *testing.T) {
	var got []AlbumArtist
	if err := json.Unmarshal([]byte(`[{"id":"a1","name":"X"},{"id":"a2","name":null}]`), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "a1" || got[0].Name == nil || *got[0].Name != "X" || got[1].Name != nil {
		t.Fatalf("unmarshal mismatch: %+v", got)
	}

	b, err := json.Marshal(Album{ID: "x", Name: "n", Artists: got})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "artist_id") || strings.Contains(s, "artist_name") {
		t.Fatalf("legacy artist fields leaked into wire JSON: %s", s)
	}
	if !strings.Contains(s, `"artists":[`) {
		t.Fatalf("artists array missing from wire JSON: %s", s)
	}
}

func TestBuildAlbumListQuery(t *testing.T) {
	// year + artist filters present: placeholders must be $1..$4 in order.
	y := 2024
	// Multi-type (single OR ep): predicates inlined (no args), so placeholders stay year/artist/q + limit/offset.
	sql, args := buildAlbumListQuery(&y, "abc", "dre", []string{"single", "ep"}, "tracks", "hide", 10, 5)
	if want := []any{2024, "abc", `\mdre`, 10, 5}; !eq(args, want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for _, frag := range []string{
		"year = $1", "aa.artist_id = $2",
		// q must match the album name, its 한글 display name, credited artist names
		// (English + 한글), aliases, and track names (English + 한글) — one arg,
		// reused placeholder. Latin queries use the word-start regex operator.
		"albums.name ~* $3", "albums.display_name ~* $3",
		"ar.name ~* $3", "ar.display_name ~* $3", "al ~* $3",
		"t.name ~* $3", "t.display_name ~* $3",
		"(album_type='single' AND total_tracks < 3)", "(album_type='single' AND total_tracks >= 3)", " OR ",
		"total_tracks DESC", "LIMIT $4", "OFFSET $5",
	} {
		if !strings.Contains(sql, frag) {
			t.Errorf("missing %q in %s", frag, sql)
		}
	}
	// public view hides soft-deleted rows.
	if !strings.Contains(sql, "albums.deleted_at IS NULL") {
		t.Errorf("expected deleted_at filter: %s", sql)
	}
	// Two-phase: filters/sort/LIMIT run on an id-only inner query, and the outer
	// one (the expensive artists json_agg) sorts the surviving page again.
	if !strings.Contains(sql, "WHERE id IN (SELECT id FROM albums") {
		t.Errorf("expected the id-only inner query: %s", sql)
	}
	if strings.Count(sql, "ORDER BY total_tracks DESC") != 2 {
		t.Errorf("both phases must carry the same ORDER BY: %s", sql)
	}
	// The aggregates are plain columns now — no per-row subqueries left.
	if strings.Contains(sql, "FROM ratings") || strings.Contains(sql, "FROM comments") {
		t.Errorf("rating/comment aggregates should come from albums columns: %s", sql)
	}

	// no filters: limit/offset shift to $1/$2 (the bug this guards against), default sort by year.
	sql, args = buildAlbumListQuery(nil, "", "", nil, "", "include", 50, 0)
	if want := []any{50, 0}; !eq(args, want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	if !strings.Contains(sql, "LIMIT $1") || !strings.Contains(sql, "OFFSET $2") {
		t.Errorf("placeholders misaligned: %s", sql)
	}
	if !strings.Contains(sql, "release_date DESC NULLS LAST") {
		t.Errorf("default sort should be by release_date: %s", sql)
	}
	// admin browsing ("include") must NOT filter deleted_at either way.
	// (albums.-qualified: the comment_count subquery filters its own deleted_at.)
	if strings.Contains(sql, "albums.deleted_at IS NULL") || strings.Contains(sql, "albums.deleted_at IS NOT NULL") {
		t.Errorf("include mode should not filter deleted_at: %s", sql)
	}

	// admin 삭제 목록 ("only") returns exclusively soft-deleted rows.
	sql, _ = buildAlbumListQuery(nil, "", "", nil, "", "only", 50, 0)
	if !strings.Contains(sql, "albums.deleted_at IS NOT NULL") {
		t.Errorf("only mode should filter to deleted rows: %s", sql)
	}
}

// Latin queries anchor at word starts; 한글 queries keep infix matching.
func TestSearchMatch(t *testing.T) {
	for _, tc := range []struct{ q, op, val string }{
		// ASCII → case-insensitive word-start regex: "ander" must not match
		// mid-word ("wandering", "Neanderthal", alias "Kim Ximya X D. Sanders").
		{"ander", " ~* ", `\mander`},
		// Regex metacharacters in the query are literals, not syntax.
		{"d. sanders", " ~* ", `\md\. sanders`},
		// Non-ASCII (한글) → infix ILIKE: "심야" must still find "김심야".
		{"심야", " ILIKE ", "%심야%"},
		// A leading non-word char can't sit after \m (it never matches) → infix.
		{".paak", " ILIKE ", "%.paak%"},
		// User-typed LIKE wildcards are escaped to literals on the infix path.
		{"백%_문", " ILIKE ", `%백\%\_문%`},
	} {
		op, val := searchMatch(tc.q)
		if op != tc.op || val != tc.val {
			t.Errorf("searchMatch(%q) = %q %q, want %q %q", tc.q, op, val, tc.op, tc.val)
		}
	}
}

func TestOrderClause(t *testing.T) {
	// Both rating sorts must still fall back to the date order so unrated albums
	// (rating_count 0 / rating_avg NULL) stay newest-first at the bottom.
	for _, tc := range []struct{ sort, wantPrefix string }{
		{"rating", ratingAvgExpr + " DESC NULLS LAST, rating_count DESC, "},
		{"popular", "rating_count DESC, " + ratingAvgExpr + " DESC NULLS LAST, "},
	} {
		got := orderClause(tc.sort)
		if !strings.HasPrefix(got, tc.wantPrefix) {
			t.Errorf("orderClause(%q) = %q, want prefix %q", tc.sort, got, tc.wantPrefix)
		}
		if !strings.HasSuffix(got, byDate) {
			t.Errorf("orderClause(%q) = %q, want date fallback tail", tc.sort, got)
		}
	}
	if orderClause("nonsense") != byDate {
		t.Errorf("unknown sort should fall back to date order")
	}
	// The columns the sorts (and the JSON response) rely on must be selected —
	// rating_avg is derived from the two stored counters, the rest are raw.
	for _, col := range []string{ratingAvgExpr + " AS rating_avg", "rating_count", "comment_count"} {
		if !strings.Contains(albumSelectCols, col) {
			t.Errorf("albumSelectCols missing %q", col)
		}
	}
}

func TestSpotifyCreds(t *testing.T) {
	t.Setenv("SPOTIFY_CLIENT_ID", "plain")
	t.Setenv("SPOTIFY_CLIENT_SECRET", "s")
	t.Setenv("FIRST_SPOTIFY_CLIENT_ID", "one")
	t.Setenv("FIRST_SPOTIFY_CLIENT_SECRET", "s1")
	t.Setenv("SECOND_SPOTIFY_CLIENT_ID", "two")
	t.Setenv("SECOND_SPOTIFY_CLIENT_SECRET", "s2")

	// no key → unprefixed wins
	if id, _, _ := spotifyCreds(""); id != "plain" {
		t.Fatalf("default key = %q, want plain", id)
	}
	// explicit keys pick their prefix
	if id, _, _ := spotifyCreds("first"); id != "one" {
		t.Fatalf("first = %q", id)
	}
	if id, _, _ := spotifyCreds("SECOND"); id != "two" { // case-insensitive
		t.Fatalf("second = %q", id)
	}
	// unknown key → error naming the expected vars
	if _, _, err := spotifyCreds("third"); err == nil || !strings.Contains(err.Error(), "THIRD_SPOTIFY_CLIENT_ID") {
		t.Fatalf("unknown key error = %v", err)
	}

	// unprefixed absent → FIRST_ fallback for the default key
	t.Setenv("SPOTIFY_CLIENT_ID", "")
	if id, _, _ := spotifyCreds(""); id != "one" {
		t.Fatalf("fallback = %q, want one", id)
	}

	// any prefix works — THIRD_ (or anything) is selectable once its pair exists
	t.Setenv("THIRD_SPOTIFY_CLIENT_ID", "three")
	t.Setenv("THIRD_SPOTIFY_CLIENT_SECRET", "s3")
	if id, _, _ := spotifyCreds("third"); id != "three" {
		t.Fatalf("third = %q", id)
	}
	keys := configuredSpotifyKeys()
	joined := strings.Join(keys, ",")
	for _, want := range []string{"first", "second", "third"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("configuredSpotifyKeys() = %v, missing %q", keys, want)
		}
	}
}

func TestYearOf(t *testing.T) {
	if y := yearOf("2023-05-01"); y == nil || *y != 2023 {
		t.Fatalf("yearOf full date = %v", y)
	}
	if y := yearOf("2019"); y == nil || *y != 2019 {
		t.Fatalf("yearOf year-only = %v", y)
	}
	if yearOf("") != nil || yearOf("abc") != nil {
		t.Fatal("yearOf junk should be nil")
	}
}

func TestNormalizeAliases(t *testing.T) {
	got := normalizeAliases([]string{" 블랙넛 ", "", "블넛", "블랙넛", "  "})
	if len(got) != 2 || got[0] != "블랙넛" || got[1] != "블넛" {
		t.Fatalf("normalizeAliases = %v", got)
	}
	if got := normalizeAliases(nil); len(got) != 0 {
		t.Fatalf("nil input should yield empty slice, got %v", got)
	}
}

// A blank 한글 이름 must clear the override (NULL), not store "" — otherwise the
// UI would render an empty title instead of falling back to the Spotify name.
func TestNilIfBlank(t *testing.T) {
	if nilIfBlank(nil) != nil {
		t.Fatal("nil input should stay nil")
	}
	for _, blank := range []string{"", "   ", "\t\n"} {
		if got := nilIfBlank(&blank); got != nil {
			t.Fatalf("nilIfBlank(%q) = %q, want nil", blank, *got)
		}
	}
	padded := "  지코  "
	if got := nilIfBlank(&padded); got == nil || *got != "지코" {
		t.Fatalf("nilIfBlank(%q) = %v, want 지코", padded, got)
	}
}

// Both flag tables run through the same parameterized SQL; the table name must
// land in both the count subquery and the EXISTS filter, or one flag type would
// silently list the other's albums.
func TestFlaggedAlbumsSQL(t *testing.T) {
	for _, table := range []string{tblNotHiphop, tblRename} {
		sql := flaggedAlbumsSQL(table)
		if strings.Count(sql, "FROM "+table+" rp") != 2 {
			t.Errorf("flaggedAlbumsSQL(%q) should reference the table twice: %s", table, sql)
		}
		if !strings.Contains(sql, "AS report_count") || !strings.Contains(sql, "ORDER BY report_count DESC") {
			t.Errorf("flaggedAlbumsSQL(%q) missing count alias/order: %s", table, sql)
		}
	}
	if tblNotHiphop == tblRename {
		t.Fatal("flag tables must be distinct")
	}
}

func eq(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRefreshTokenMinting(t *testing.T) {
	tok, hash, err := newRefreshToken()
	if err != nil {
		t.Fatalf("newRefreshToken: %v", err)
	}
	// The stored value must not be the token itself — a DB dump can't be replayed.
	if hash == tok {
		t.Fatal("stored hash equals the raw token")
	}
	if hashToken(tok) != hash {
		t.Fatal("hashToken disagrees with the hash issued alongside the token")
	}
	if len(tok) < 32 {
		t.Fatalf("token too short to be unguessable: %d chars", len(tok))
	}
	other, _, err := newRefreshToken()
	if err != nil {
		t.Fatalf("newRefreshToken: %v", err)
	}
	if other == tok {
		t.Fatal("two mints produced the same token")
	}
	// A short-lived access token is the whole point of adding refresh tokens.
	if accessTokenTTL > time.Hour {
		t.Fatalf("accessTokenTTL = %v, want <= 1h", accessTokenTTL)
	}
	if refreshTokenTTL <= accessTokenTTL {
		t.Fatal("refresh token must outlive the access token")
	}
}

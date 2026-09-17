package main

// Apple Music 카탈로그 연동. Spotify ID는 그대로 PK로 두고, UPC로 Apple 앨범을
// 찾아 apple_id·레이블·장르(앨범/아티스트)를 채운다 — 2026-02 Spotify 개편으로
// 죽은 label/genres의 대체 소스. 카탈로그 조회는 developer token(ES256 JWT)만
// 있으면 되고 사용자 토큰·구독 상태와 무관하다.

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
)

const amAPI = "https://api.music.apple.com/v1/catalog/"

// amUPCBatch — filter[upc]에 한 번에 넘기는 UPC 수. 문서상 ids 상한 100이지만
// 응답에 UPC당 중복 앨범이 섞여 오므로 여유 있게 잡는다.
const amUPCBatch = 25

func appleConfigured() bool {
	return os.Getenv("APPLE_MUSIC_TEAM_ID") != "" && os.Getenv("APPLE_MUSIC_KEY_ID") != "" &&
		os.Getenv("APPLE_MUSIC_PRIVATE_KEY_PATH") != ""
}

// appleAuth caches one signed developer token; 서명은 로컬 연산이라 짧게(1h) 잡고
// 만료 전에 다시 서명한다.
type appleAuth struct {
	mu    sync.Mutex
	token string
	exp   time.Time
}

func (a *appleAuth) get() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.token != "" && time.Now().Before(a.exp.Add(-time.Minute)) {
		return a.token, nil
	}
	key, err := loadAppleKey(os.Getenv("APPLE_MUSIC_PRIVATE_KEY_PATH"))
	if err != nil {
		return "", err
	}
	now := time.Now()
	tok, err := signAppleJWT(key, os.Getenv("APPLE_MUSIC_TEAM_ID"), os.Getenv("APPLE_MUSIC_KEY_ID"), now, time.Hour)
	if err != nil {
		return "", err
	}
	a.token, a.exp = tok, now.Add(time.Hour)
	return tok, nil
}

func loadAppleKey(path string) (*ecdsa.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("apple key: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("apple key: PEM 형식이 아님")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("apple key: %w", err)
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("apple key: ECDSA(P-256) 키가 아님")
	}
	return ec, nil
}

// signAppleJWT — Apple은 ES256만 받는다. JOSE 서명은 DER이 아니라 r‖s 고정폭 64바이트.
func signAppleJWT(key *ecdsa.PrivateKey, teamID, keyID string, now time.Time, ttl time.Duration) (string, error) {
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	input := enc(map[string]string{"alg": "ES256", "kid": keyID}) + "." +
		enc(map[string]any{"iss": teamID, "iat": now.Unix(), "exp": now.Add(ttl).Unix()})
	h := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, key, h[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

type amError struct {
	Status int
	URL    string
	Body   string
}

func (e *amError) Error() string {
	return fmt.Sprintf("apple %d @ %s: %.200s", e.Status, e.URL, e.Body)
}

// amGet fetches a storefront-relative catalog path (e.g. "albums?filter[upc]=…")
// into dst, waiting out one short 429. Apple은 레이트 리밋 수치를 공개하지 않고
// "잠시 후 풀린다"고만 하므로 한 번만 기다리고 그 다음은 배치를 끊는다.
func (s *server) amGet(ctx context.Context, path string, dst any) error {
	rawURL := amAPI + url.PathEscape(env("APPLE_MUSIC_STOREFRONT", "kr")) + "/" + path
	waited := false
	for {
		tok, err := s.apple.get()
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode == 429 && !waited {
			wait, _ := strconv.Atoi(res.Header.Get("Retry-After"))
			if wait <= 0 {
				wait = 2
			}
			if wait > 30 {
				return &amError{Status: 429, URL: rawURL, Body: string(body)}
			}
			waited = true
			select {
			case <-time.After(time.Duration(wait+1) * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}
		if res.StatusCode >= 400 {
			return &amError{Status: res.StatusCode, URL: rawURL, Body: string(body)}
		}
		return json.Unmarshal(body, dst)
	}
}

// ---- Apple payload shapes (only the fields we use) ----

type amArtist struct {
	ID         string `json:"id"`
	Attributes struct {
		Name       string   `json:"name"`
		GenreNames []string `json:"genreNames"`
	} `json:"attributes"`
}

type amTrack struct {
	ID         string `json:"id"`
	Attributes struct {
		Name        string `json:"name"`
		DiscNumber  int    `json:"discNumber"`
		TrackNumber int    `json:"trackNumber"`
	} `json:"attributes"`
}

type amAlbum struct {
	ID         string `json:"id"`
	Attributes struct {
		Name           string   `json:"name"`
		UPC            string   `json:"upc"`
		RecordLabel    string   `json:"recordLabel"`
		GenreNames     []string `json:"genreNames"`
		ReleaseDate    string   `json:"releaseDate"`
		TrackCount     int      `json:"trackCount"`
		Copyright      string   `json:"copyright"`
		ContentRating  string   `json:"contentRating"`
		EditorialNotes struct {
			Short    string `json:"short"`
			Standard string `json:"standard"`
		} `json:"editorialNotes"`
	} `json:"attributes"`
	Relationships struct {
		Artists struct {
			Data []amArtist `json:"data"`
		} `json:"artists"`
		Tracks struct {
			Data []amTrack `json:"data"`
		} `json:"tracks"`
	} `json:"relationships"`
}

// placeholderUPC — 유통사가 UPC를 안 넣으면 Apple/Spotify 모두 0으로 채운 값이
// 온다. filter[upc]에 넣으면 전 세계 잡동사니 앨범이 매칭되므로 조회 자체를 막는다.
func placeholderUPC(upc string) bool {
	return strings.Trim(upc, "0") == ""
}

// amGenres drops Apple's catch-all root genre ("음악"/"Music") that rides on
// every album — 필터 값으로 쓸모없다.
func amGenres(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n == "음악" || n == "Music" {
			continue
		}
		out = append(out, n)
	}
	return out
}

// normName — Spotify 크레딧 이름과 Apple 아티스트 이름을 맞출 때의 키. 대소문자·
// 공백·구두점만 무시한다 (한/영 표기 차이는 못 맞추며, 그런 아티스트는 미연결로 남는다).
func normName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r == ' ' || r == '.' || r == '-' || r == '_' || r == '\'' || r == '’' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func matchAppleArtist(byName map[string]amArtist, names []string) (amArtist, bool) {
	for _, n := range names {
		if n == "" {
			continue
		}
		if a, ok := byName[normName(n)]; ok {
			return a, true
		}
	}
	return amArtist{}, false
}

// amLabel — 유통사가 레이블을 안 넣으면 "/" 같은 껍데기가 온다. 글자/숫자가 하나도
// 없으면 값 없음으로 본다.
func amLabel(s string) *string {
	if !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
		return nil
	}
	return strPtr(strings.TrimSpace(s))
}

// matchByUPC picks, for each wanted UPC, the first Apple album carrying exactly
// that UPC (filter[upc] 응답은 요청 순서를 지키지 않고 같은 UPC의 재발매판이 여럿
// 섞여 온다).
func matchByUPC(albums []amAlbum, upcs []string) map[string]amAlbum {
	want := map[string]bool{}
	for _, u := range upcs {
		want[u] = true
	}
	out := map[string]amAlbum{}
	for _, al := range albums {
		u := al.Attributes.UPC
		if want[u] {
			if _, seen := out[u]; !seen {
				out[u] = al
			}
		}
	}
	return out
}

// koreanName returns s only when it carries Hangul — display_name은 한글 표시
// 이름 자리라 "Work (feat. Swings)" 같은 영문 변형은 채우지 않는다.
func koreanName(s string) *string {
	s = strings.TrimSpace(s)
	if !strings.ContainsFunc(s, func(r rune) bool { return unicode.Is(unicode.Hangul, r) }) {
		return nil
	}
	return &s
}

// amNotes picks the fuller editorial note; Apple wraps them in light HTML
// (<i>, <br>) that the detail page renders as text, so tags are stripped.
func amNotes(standard, short string) *string {
	s := standard
	if s == "" {
		s = short
	}
	s = strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n").Replace(s)
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strPtr(strings.TrimSpace(b.String()))
}

// amTitle strips Apple's type suffix ("도모 - EP", "Train - Single") so the
// display name reads like the album title, not the store listing.
func amTitle(name string) string {
	for _, suf := range []string{" - EP", " - Single"} {
		if t, ok := strings.CutSuffix(name, suf); ok {
			return strings.TrimSpace(t)
		}
	}
	return strings.TrimSpace(name)
}

// amKind — Apple 스토어 등록 유형. isSingle 플래그는 1트랙일 때만 true라 못 쓰고,
// 이름 접미어가 등록 규칙(1~3트랙 Single, 4~6트랙 EP, 그 외 Album)을 그대로 담는다.
func amKind(name string) string {
	switch {
	case strings.HasSuffix(name, " - Single"):
		return "single"
	case strings.HasSuffix(name, " - EP"):
		return "ep"
	}
	return "album"
}

type appleLinkResult struct {
	Checked       int     `json:"checked"`        // 이번에 Apple에 조회한 앨범 수
	Linked        int     `json:"linked"`         // apple_id가 새로 붙은 앨범 수
	ArtistsLinked int     `json:"artists_linked"` // apple_id가 새로 붙은 아티스트 수
	Named         int     `json:"named"`          // 한글 표시 이름이 채워진 앨범·아티스트·트랙 수
	Remaining     int     `json:"remaining"`      // 아직 조회 안 한 앨범 수
	Error         *string `json:"error,omitempty"`
}

// appleLinkPendingSQL — 아직 Apple 조회를 안 한 live 앨범.
const appleLinkPendingSQL = "FROM albums WHERE deleted_at IS NULL AND apple_id IS NULL AND apple_checked_at IS NULL"

type appleCand struct {
	ID          string
	Name        string
	UPC         *string
	ReleaseDate *string
	TotalTracks *int
}

const amInclude = "artists,tracks"

// linkAppleAlbums resolves the given albums on Apple — UPC가 있으면 filter[upc]
// 배치, 없으면 아티스트+앨범명 검색(발매일·트랙 수 일치 검증) — and writes
// apple_id/label/genres, links credited artists, and fills empty display_name on
// the album, its artists and its tracks with the Korean catalog names. 조회한
// 앨범은 결과와 무관하게 apple_checked_at을 찍어 다시 집지 않는다. Apple 오류가
// 나면 거기서 멈추고 진행분을 돌려준다 (안 찍힌 앨범은 다음에 다시).
// ponytail: 미발견 앨범은 영구 스킵 — Apple에 늦게 올라오는 앨범이 눈에 띄면
// apple_checked_at 기준 N일 후 재시도로 바꾼다. 트랙 이름은 연결 시점에 트랙이
// 동기화돼 있어야 채워진다 (나중에 동기화된 트랙은 안 돌아본다).
func (s *server) linkAppleAlbums(ctx context.Context, albumIDs []string) (appleLinkResult, error) {
	res := appleLinkResult{}
	rows, err := s.db.Query(ctx,
		"SELECT id, name, upc, release_date, total_tracks "+appleLinkPendingSQL+" AND id = ANY($1)", albumIDs)
	if err != nil {
		return res, err
	}
	pending, err := pgx.CollectRows(rows, pgx.RowToStructByPos[appleCand])
	if err != nil {
		return res, err
	}

	var byUPC, bySearch []appleCand
	for _, c := range pending {
		if c.UPC != nil && !placeholderUPC(*c.UPC) {
			byUPC = append(byUPC, c)
		} else {
			bySearch = append(bySearch, c)
		}
	}

	for start := 0; start < len(byUPC); start += amUPCBatch {
		chunk := byUPC[start:min(start+amUPCBatch, len(byUPC))]
		upcs := make([]string, 0, len(chunk))
		for _, c := range chunk {
			upcs = append(upcs, *c.UPC)
		}
		var page struct {
			Data []amAlbum `json:"data"`
		}
		q := url.Values{"filter[upc]": {strings.Join(upcs, ",")}, "include": {amInclude}, "l": {"ko"}}
		if err := s.amGet(ctx, "albums?"+q.Encode(), &page); err != nil {
			res.Error = strPtr(err.Error())
			return res, nil
		}
		found := matchByUPC(page.Data, upcs)
		for _, c := range chunk {
			al, ok := found[*c.UPC]
			var alp *amAlbum
			if ok {
				alp = &al
			}
			if err := s.applyAppleAlbum(ctx, c, alp, &res); err != nil {
				return res, err
			}
		}
	}

	for _, c := range bySearch {
		al, err := s.searchAppleAlbum(ctx, c)
		if err != nil {
			res.Error = strPtr(err.Error())
			return res, nil
		}
		if err := s.applyAppleAlbum(ctx, c, al, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

// searchAppleAlbum — UPC 없는 앨범의 폴백. "대표 아티스트 앨범명"으로 검색해
// 발매일과 트랙 수가 모두 일치하는 첫 결과를 고른다 (Apple 검색은 영문 term으로
// 한글 등록 앨범도 찾는다). 발매일·트랙 수가 없으면 검증 불가라 시도하지 않는다.
func (s *server) searchAppleAlbum(ctx context.Context, c appleCand) (*amAlbum, error) {
	if c.ReleaseDate == nil || c.TotalTracks == nil {
		return nil, nil
	}
	var artist string
	_ = s.db.QueryRow(ctx,
		`SELECT COALESCE(ar.name, '') FROM album_artists aa JOIN artists ar ON ar.id = aa.artist_id
		 WHERE aa.album_id=$1 ORDER BY aa.position LIMIT 1`, c.ID).Scan(&artist)
	var page struct {
		Results struct {
			Albums struct {
				Data []amAlbum `json:"data"`
			} `json:"albums"`
		} `json:"results"`
	}
	q := url.Values{"term": {strings.TrimSpace(artist + " " + c.Name)}, "types": {"albums"}, "limit": {"5"}, "l": {"ko"}}
	if err := s.amGet(ctx, "search?"+q.Encode(), &page); err != nil {
		return nil, err
	}
	for _, hit := range page.Results.Albums.Data {
		if hit.Attributes.ReleaseDate != *c.ReleaseDate || hit.Attributes.TrackCount != *c.TotalTracks {
			continue
		}
		var full struct {
			Data []amAlbum `json:"data"`
		}
		q := url.Values{"include": {amInclude}, "l": {"ko"}}
		if err := s.amGet(ctx, "albums/"+url.PathEscape(hit.ID)+"?"+q.Encode(), &full); err != nil {
			return nil, err
		}
		if len(full.Data) == 1 {
			return &full.Data[0], nil
		}
	}
	return nil, nil
}

// applyAppleAlbum writes one resolved (or unresolved: al == nil) album.
func (s *server) applyAppleAlbum(ctx context.Context, c appleCand, al *amAlbum, res *appleLinkResult) error {
	res.Checked++
	if al == nil {
		_, err := s.db.Exec(ctx, "UPDATE albums SET apple_checked_at=now() WHERE id=$1", c.ID)
		return err
	}
	var named bool
	title := koreanName(amTitle(al.Attributes.Name))
	if err := s.db.QueryRow(ctx,
		`UPDATE albums SET apple_id=$2, label=$3, genres=$4, apple_checked_at=now(),
		   copyright=$6, content_rating=$7, editorial_notes=$8, apple_type=$9,
		   display_name = CASE WHEN display_name IS NULL AND lower($5::text) <> lower(name) THEN $5::text ELSE display_name END
		 WHERE id=$1 RETURNING COALESCE(display_name = $5::text AND lower($5::text) <> lower(name), false)`,
		c.ID, al.ID, amLabel(al.Attributes.RecordLabel), amGenres(al.Attributes.GenreNames), title,
		strPtr(strings.TrimSpace(al.Attributes.Copyright)), strPtr(al.Attributes.ContentRating), amNotes(al.Attributes.EditorialNotes.Standard, al.Attributes.EditorialNotes.Short),
		amKind(al.Attributes.Name)).Scan(&named); err != nil {
		return err
	}
	res.Linked++
	if named {
		res.Named++
	}
	n, named2, err := s.linkAppleArtists(ctx, c.ID, al.Relationships.Artists.Data)
	if err != nil {
		return err
	}
	res.ArtistsLinked += n
	res.Named += named2
	n, err = s.nameAppleTracks(ctx, c.ID, al.Relationships.Tracks.Data)
	if err != nil {
		return err
	}
	res.Named += n
	return nil
}

// nameAppleTracks fills empty track display_name by (disc, track) position —
// Spotify 트랙엔 ISRC가 없어 위치로 맞추며, 양쪽 트랙 수가 같을 때만 신뢰한다.
func (s *server) nameAppleTracks(ctx context.Context, albumID string, tracks []amTrack) (int, error) {
	if len(tracks) == 0 {
		return 0, nil
	}
	var have int
	if err := s.db.QueryRow(ctx, "SELECT COUNT(*)::int FROM tracks WHERE album_id=$1", albumID).Scan(&have); err != nil {
		return 0, err
	}
	if have != len(tracks) {
		return 0, nil
	}
	named := 0
	for _, t := range tracks {
		disc := t.Attributes.DiscNumber
		if disc == 0 {
			disc = 1
		}
		tag, err := s.db.Exec(ctx,
			`UPDATE tracks SET display_name=$4 WHERE album_id=$1 AND disc_number=$2 AND track_number=$3
			   AND display_name IS NULL AND lower(name) <> lower($4::text)`,
			albumID, disc, t.Attributes.TrackNumber, koreanName(t.Attributes.Name))
		if err != nil {
			return named, err
		}
		named += int(tag.RowsAffected())
	}
	return named, nil
}

// linkAppleArtists matches an album's Spotify credits to its Apple artists and
// fills an empty display_name with the Korean catalog name; 이미 연결된
// 아티스트는 건드리지 않는다. Returns (linked, named).
func (s *server) linkAppleArtists(ctx context.Context, albumID string, apple []amArtist) (int, int, error) {
	if len(apple) == 0 {
		return 0, 0, nil
	}
	// 총 크레딧 수도 세어 둔다 — 단독 크레딧 ↔ 단독 Apple 아티스트는 이름이 달라도 같은 사람이다.
	rows, err := s.db.Query(ctx,
		`SELECT ar.id, COALESCE(ar.name, ''), COALESCE(ar.display_name, ''), COALESCE(ar.aliases, '{}'), ar.apple_id IS NULL,
		        (SELECT COUNT(*) FROM album_artists t WHERE t.album_id = aa.album_id)::int
		 FROM album_artists aa JOIN artists ar ON ar.id = aa.artist_id WHERE aa.album_id=$1`, albumID)
	if err != nil {
		return 0, 0, err
	}
	type credit struct {
		ID          string
		Name        string
		DisplayName string
		Aliases     []string
		Unlinked    bool
		Total       int
	}
	credits, err := pgx.CollectRows(rows, pgx.RowToStructByPos[credit])
	if err != nil {
		return 0, 0, err
	}
	byName := map[string]amArtist{}
	for _, a := range apple {
		byName[normName(a.Attributes.Name)] = a
	}
	linked, named := 0, 0
	for _, c := range credits {
		if !c.Unlinked {
			continue
		}
		// l=ko 응답은 한글 표기(디핵)라 Spotify 영문명(D-Hack)과 안 맞는다 — 관리자가
		// 등록한 한글 표시 이름·연관검색어까지 대조한다.
		a, ok := matchAppleArtist(byName, append([]string{c.Name, c.DisplayName}, c.Aliases...))
		if !ok && c.Total == 1 && len(apple) == 1 {
			a, ok = apple[0], true
		}
		if !ok {
			continue
		}
		var gotName bool
		err := s.db.QueryRow(ctx,
			`UPDATE artists SET apple_id=$2, genres=CASE WHEN cardinality($3::text[]) > 0 THEN $3 ELSE genres END,
			   display_name = CASE WHEN display_name IS NULL AND lower($4::text) <> lower(COALESCE(name, '')) THEN $4::text ELSE display_name END
			 WHERE id=$1 AND apple_id IS NULL
			 RETURNING COALESCE(display_name = $4::text AND lower($4::text) <> lower(COALESCE(name, '')), false)`,
			c.ID, a.ID, amGenres(a.Attributes.GenreNames), koreanName(a.Attributes.Name)).Scan(&gotName)
		if errors.Is(err, pgx.ErrNoRows) {
			continue // 같은 배치의 다른 앨범이 먼저 연결함
		}
		if err != nil {
			return linked, named, err
		}
		linked++
		if gotName {
			named++
		}
	}
	return linked, named, nil
}

// appleLinkBatch — 미조회 앨범을 최신 발매순으로 최대 limit개 처리한다. 관리자
// 탭과 서버 스윕이 같이 쓴다.
func (s *server) appleLinkBatch(ctx context.Context, limit int) (appleLinkResult, error) {
	rows, err := s.db.Query(ctx,
		"SELECT id "+appleLinkPendingSQL+" ORDER BY release_date DESC NULLS LAST, id LIMIT $1", limit)
	if err != nil {
		return appleLinkResult{}, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return appleLinkResult{}, err
	}
	res, err := s.linkAppleAlbums(ctx, ids)
	if err != nil {
		return res, err
	}
	err = s.db.QueryRow(ctx, "SELECT COUNT(*)::int "+appleLinkPendingSQL).Scan(&res.Remaining)
	return res, err
}

// backfillAppleTypes — apple_type 컬럼 추가 전에 연결된 앨범의 유형을 채운다.
// 이름만 필요하니 include 없이 ids 조회. 다 차면 0을 돌려주고 이후엔 no-op.
func (s *server) backfillAppleTypes(ctx context.Context, limit int) (int, error) {
	rows, err := s.db.Query(ctx,
		"SELECT id, apple_id FROM albums WHERE apple_id IS NOT NULL AND apple_type IS NULL ORDER BY release_date DESC NULLS LAST LIMIT $1", limit)
	if err != nil {
		return 0, err
	}
	pairs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ ID, AppleID string }])
	if err != nil || len(pairs) == 0 {
		return 0, err
	}
	done := 0
	for start := 0; start < len(pairs); start += amUPCBatch {
		chunk := pairs[start:min(start+amUPCBatch, len(pairs))]
		ids := make([]string, 0, len(chunk))
		for _, p := range chunk {
			ids = append(ids, p.AppleID)
		}
		var page struct {
			Data []amAlbum `json:"data"`
		}
		q := url.Values{"ids": {strings.Join(ids, ",")}, "l": {"ko"}}
		if err := s.amGet(ctx, "albums?"+q.Encode(), &page); err != nil {
			return done, err
		}
		kind := make(map[string]string, len(page.Data))
		for _, al := range page.Data {
			kind[al.ID] = amKind(al.Attributes.Name)
		}
		for _, p := range chunk {
			k, ok := kind[p.AppleID]
			if !ok {
				// Apple에서 사라진 id — 휴리스틱으로 남기되 매번 재조회하지 않도록 빈 문자열 대신 건너뜀
				continue
			}
			if _, err := s.db.Exec(ctx, "UPDATE albums SET apple_type=$2 WHERE id=$1", p.ID, k); err != nil {
				return done, err
			}
			done++
		}
	}
	return done, nil
}

// linkNewAlbums — 크롤/신보 체크가 방금 넣은 앨범을 best-effort로 연결한다.
// Apple 미설정이면 조용히 건너뛴다.
func (s *server) linkNewAlbums(ctx context.Context, albumIDs []string) {
	if !appleConfigured() || len(albumIDs) == 0 {
		return
	}
	res, err := s.linkAppleAlbums(ctx, albumIDs)
	if err != nil {
		log.Printf("apple link: DB 오류: %v", err)
	} else if res.Error != nil {
		log.Printf("apple link: 중단: %s", *res.Error)
	}
}

// appleSweepBatches × appleSweepBatch = 신보 스윕 한 번에 처리할 미조회 앨범 상한.
// 초기 백로그(수천 장)는 며칠에 걸쳐 빠지고, 그 뒤엔 새 앨범만 남는다.
const (
	appleSweepBatch   = 50
	appleSweepBatches = 20
)

// sweepApple drains the Apple link backlog after the release sweep. 오류가
// 나면 다음 스윕에서 이어서 한다 (스탬프 안 찍힌 앨범은 그대로 남는다).
func (s *server) sweepApple(ctx context.Context) {
	if !appleConfigured() {
		return
	}
	if n, err := s.backfillAppleTypes(ctx, appleSweepBatch*appleSweepBatches); err != nil {
		log.Printf("apple sweep: 유형 백필 중단(%d개 채움): %v", n, err)
	} else if n > 0 {
		log.Printf("apple sweep: 유형 백필 %d개", n)
	}
	total := appleLinkResult{}
	for i := 0; i < appleSweepBatches; i++ {
		res, err := s.appleLinkBatch(ctx, appleSweepBatch)
		if err != nil {
			log.Printf("apple sweep: DB 오류로 중단: %v", err)
			return
		}
		total.Checked += res.Checked
		total.Linked += res.Linked
		total.ArtistsLinked += res.ArtistsLinked
		total.Named += res.Named
		if res.Error != nil {
			log.Printf("apple sweep: 중단(%s) — %d개 조회, 앨범 %d개·아티스트 %d명 연결, 이름 %d개", *res.Error, total.Checked, total.Linked, total.ArtistsLinked, total.Named)
			return
		}
		if res.Remaining == 0 || res.Checked == 0 {
			log.Printf("apple sweep: 완료 — %d개 조회, 앨범 %d개·아티스트 %d명 연결, 이름 %d개", total.Checked, total.Linked, total.ArtistsLinked, total.Named)
			return
		}
	}
	log.Printf("apple sweep: 상한 도달 — %d개 조회, 앨범 %d개·아티스트 %d명 연결, 이름 %d개 (다음 스윕에 계속)", total.Checked, total.Linked, total.ArtistsLinked, total.Named)
}

// GET /admin/apple/status
func (s *server) adminAppleStatus(w http.ResponseWriter, r *http.Request) {
	var albums, linked, pending, artists int
	if err := s.db.QueryRow(r.Context(),
		`SELECT COUNT(*)::int, COUNT(apple_id)::int,
		        COUNT(*) FILTER (WHERE apple_id IS NULL AND apple_checked_at IS NULL)::int
		 FROM albums WHERE deleted_at IS NULL`).Scan(&albums, &linked, &pending); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.db.QueryRow(r.Context(), "SELECT COUNT(apple_id)::int FROM artists").Scan(&artists); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"configured": appleConfigured(), "albums": albums, "linked": linked, "pending": pending, "artists_linked": artists,
	})
}

// POST /admin/apple/link {limit} — 트랙 백필과 같은 배치 계약: remaining이 0이
// 될 때까지 반복 호출.
func (s *server) adminAppleLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Limit int `json:"limit"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !appleConfigured() {
		writeErr(w, 400, "APPLE_MUSIC_* 환경변수가 설정되지 않았습니다")
		return
	}
	if body.Limit < 1 || body.Limit > 100 {
		body.Limit = 50
	}
	if _, err := s.backfillAppleTypes(r.Context(), body.Limit); err != nil {
		writeErr(w, 502, "apple_type 백필 실패: "+err.Error())
		return
	}
	res, err := s.appleLinkBatch(r.Context(), body.Limit)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

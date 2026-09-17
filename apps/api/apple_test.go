package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSignAppleJWT(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	tok, err := signAppleJWT(key, "TEAM123456", "KEY1234567", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("want 3 parts, got %d", len(parts))
	}
	var hdr map[string]string
	b, _ := base64.RawURLEncoding.DecodeString(parts[0])
	json.Unmarshal(b, &hdr)
	if hdr["alg"] != "ES256" || hdr["kid"] != "KEY1234567" {
		t.Errorf("header = %v", hdr)
	}
	var claims map[string]any
	b, _ = base64.RawURLEncoding.DecodeString(parts[1])
	json.Unmarshal(b, &claims)
	if claims["iss"] != "TEAM123456" || claims["iat"] != float64(now.Unix()) || claims["exp"] != float64(now.Add(time.Hour).Unix()) {
		t.Errorf("claims = %v", claims)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		t.Fatalf("signature must be raw r||s 64 bytes, got %d (%v)", len(sig), err)
	}
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(&key.PublicKey, h[:], r, s) {
		t.Error("signature does not verify")
	}
}

func TestNormName(t *testing.T) {
	cases := map[string]string{
		"Beenzino":           "beenzino",
		"C JAMM":             "cjamm",
		"pH-1":               "ph1",
		"Lil' Boi":           "lilboi",
		"빈지노":                "빈지노",
		"JUSTHIS & Paloalto": "justhis&paloalto",
	}
	for in, want := range cases {
		if got := normName(in); got != want {
			t.Errorf("normName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlaceholderUPC(t *testing.T) {
	for upc, want := range map[string]bool{"": true, "0000000000000": true, "000000000000": true, "197189280801": false, "8809534465994": false} {
		if got := placeholderUPC(upc); got != want {
			t.Errorf("placeholderUPC(%q) = %v", upc, got)
		}
	}
}

func TestAmGenres(t *testing.T) {
	got := amGenres([]string{"얼터너티브 랩", "음악", "힙합/랩", "Music"})
	if want := []string{"얼터너티브 랩", "힙합/랩"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	if got := amGenres(nil); got == nil || len(got) != 0 {
		t.Errorf("nil in must give empty (not nil) slice for the text[] column, got %#v", got)
	}
}

func TestMatchByUPC(t *testing.T) {
	mk := func(id, upc string) amAlbum {
		var a amAlbum
		a.ID, a.Attributes.UPC = id, upc
		return a
	}
	// filter[upc] 응답: 순서 뒤섞임 + 같은 UPC 재발매판 중복 + 요청 안 한 UPC 섞임
	albums := []amAlbum{mk("x", "999"), mk("b2", "222"), mk("a1", "111"), mk("b1", "222")}
	got := matchByUPC(albums, []string{"111", "222", "333"})
	if got["111"].ID != "a1" || got["222"].ID != "b2" {
		t.Errorf("got %v", got)
	}
	if _, ok := got["333"]; ok {
		t.Error("unrequested/missing UPC must not match")
	}
	if _, ok := got["999"]; ok {
		t.Error("UPC not in the wanted list must be dropped")
	}
}

// TestAppleLive hits the real catalog with the .env credentials — APPLE_LIVE=1일
// 때만 돈다 (키 로딩·서명·amGet 경로 검증용).
func TestAppleLive(t *testing.T) {
	if os.Getenv("APPLE_LIVE") == "" {
		t.Skip("APPLE_LIVE unset")
	}
	loadDotenv("../../.env")
	if !appleConfigured() {
		t.Fatal("APPLE_MUSIC_* not configured")
	}
	s := &server{apple: &appleAuth{}}
	var page struct {
		Data []amAlbum `json:"data"`
	}
	if err := s.amGet(context.Background(), "albums?filter[upc]=197189280801&include=artists&l=ko", &page); err != nil {
		t.Fatal(err)
	}
	m := matchByUPC(page.Data, []string{"197189280801"})
	al, ok := m["197189280801"]
	if !ok || al.Attributes.Name != "NOWITZKI" || al.Attributes.RecordLabel == "" || len(al.Relationships.Artists.Data) == 0 {
		t.Fatalf("unexpected: %+v", page.Data)
	}
	t.Logf("%s | %s | %v | %s", al.Attributes.Name, al.Attributes.RecordLabel, amGenres(al.Attributes.GenreNames), al.Relationships.Artists.Data[0].Attributes.Name)
}

func TestAmLabel(t *testing.T) {
	for in, want := range map[string]string{"/": "", " ": "", "BANA": "BANA", " H1GHR MUSIC RECORDS ": "H1GHR MUSIC RECORDS", "℗ 2026 /": "℗ 2026 /"} {
		got := amLabel(in)
		if (want == "" && got != nil) || (want != "" && (got == nil || *got != want)) {
			t.Errorf("amLabel(%q) = %v, want %q", in, got, want)
		}
	}
}

func TestMatchAppleArtist(t *testing.T) {
	mk := func(id, name string) amArtist {
		var a amArtist
		a.ID, a.Attributes.Name = id, name
		return a
	}
	byName := map[string]amArtist{normName("디핵"): mk("1", "디핵"), normName("BIG Naughty"): mk("2", "BIG Naughty")}
	if a, ok := matchAppleArtist(byName, []string{"D-Hack", "", "디핵"}); !ok || a.ID != "1" {
		t.Errorf("alias/display_name should match: %v %v", a, ok)
	}
	if a, ok := matchAppleArtist(byName, []string{"big naughty"}); !ok || a.ID != "2" {
		t.Errorf("case-insensitive name should match: %v %v", a, ok)
	}
	if _, ok := matchAppleArtist(byName, []string{"D-Hack", ""}); ok {
		t.Error("no candidate should not match")
	}
}

func TestAmTitle(t *testing.T) {
	for in, want := range map[string]string{"도모 - EP": "도모", "Train (feat. C JAMM) - Single": "Train (feat. C JAMM)", "NOWITZKI": "NOWITZKI", " 12 ": "12", "Single - Single": "Single"} {
		if got := amTitle(in); got != want {
			t.Errorf("amTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAmKind(t *testing.T) {
	for in, want := range map[string]string{"WORTHY - EP": "ep", "내리고 (feat. JUSTHIS) - Single": "single", "Drunk Night / Not Yours - Single": "single", "Dogma": "album", "Single": "album"} {
		if got := amKind(in); got != want {
			t.Errorf("amKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKoreanName(t *testing.T) {
	if got := koreanName("Work (feat. Swings)"); got != nil {
		t.Errorf("latin-only title must not become a display name, got %q", *got)
	}
	if got := koreanName(" 4랑을 4랑해 "); got == nil || *got != "4랑을 4랑해" {
		t.Errorf("hangul title should be kept trimmed, got %v", got)
	}
}

func TestAmNotes(t *testing.T) {
	got := amNotes("<i>Heart on My Sleeve</i> is filled<br>with love.", "short")
	if got == nil || *got != "Heart on My Sleeve is filled\nwith love." {
		t.Errorf("got %v", got)
	}
	if got := amNotes("", " short note "); got == nil || *got != "short note" {
		t.Errorf("fallback to short, got %v", got)
	}
	if got := amNotes("", ""); got != nil {
		t.Errorf("empty must be nil, got %q", *got)
	}
}

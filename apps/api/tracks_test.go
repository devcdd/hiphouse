package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Lock the Spotify album→tracks decode (fields we persist) and the public wire
// shape: tracks.artists JSONB unmarshals straight into []trackArtist, and a
// Track marshals with every column the UI will read.
func TestSpotifyTrackJSON(t *testing.T) {
	payload := `{"tracks":{"items":[
		{"id":"t1","name":"Intro","disc_number":1,"track_number":1,"duration_ms":123456,"explicit":true,
		 "external_urls":{"spotify":"https://open.spotify.com/track/t1"},
		 "artists":[{"id":"a1","name":"Main"},{"id":"a2","name":"Feat"}]}
	],"next":null}}`
	var full struct {
		Tracks spTrackPage `json:"tracks"`
	}
	if err := json.Unmarshal([]byte(payload), &full); err != nil {
		t.Fatal(err)
	}
	if full.Tracks.Next != nil {
		t.Fatalf("next should be nil, got %v", *full.Tracks.Next)
	}
	if len(full.Tracks.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(full.Tracks.Items))
	}
	tr := full.Tracks.Items[0]
	if tr.ID != "t1" || tr.Name != "Intro" || tr.DiscNumber != 1 || tr.TrackNumber != 1 ||
		tr.DurationMS != 123456 || !tr.Explicit || tr.ExternalURLs.Spotify == "" {
		t.Fatalf("track decode mismatch: %+v", tr)
	}
	if len(tr.Artists) != 2 || tr.Artists[1] != (trackArtist{ID: "a2", Name: "Feat"}) {
		t.Fatalf("artists decode mismatch: %+v", tr.Artists)
	}

	dur := 123456
	b, err := json.Marshal(Track{ID: "t1", DiscNumber: 1, TrackNumber: 1, Name: "Intro",
		DurationMS: &dur, Explicit: true, Artists: tr.Artists})
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{`"id"`, `"disc_number"`, `"track_number"`, `"name"`,
		`"display_name"`, `"duration_ms"`, `"explicit"`, `"spotify_url"`, `"artists":[{"id":"a1"`} {
		if !strings.Contains(string(b), frag) {
			t.Errorf("missing %s in wire JSON: %s", frag, b)
		}
	}
}

// Lock the full-album decode fetchAlbumTracks relies on: the embedded spAlbum
// picks up 메타 (upc/copyrights/precision) while Tracks decodes alongside it,
// and copyrightsJSON round-trips the ℗/© lines for the JSONB column.
func TestSpotifyAlbumMetaJSON(t *testing.T) {
	payload := `{
		"id":"al1","name":"Album","album_type":"album",
		"release_date":"2026-07-01","release_date_precision":"day","total_tracks":1,
		"external_ids":{"upc":"00602557631722"},
		"copyrights":[{"text":"℗ 2026 AOMG","type":"P"},{"text":"© 2026 AOMG","type":"C"}],
		"artists":[{"id":"a1","name":"Main"}],
		"tracks":{"items":[{"id":"t1","name":"Only","disc_number":1,"track_number":1}],"next":null}}`
	var full struct {
		spAlbum
		Tracks spTrackPage `json:"tracks"`
	}
	if err := json.Unmarshal([]byte(payload), &full); err != nil {
		t.Fatal(err)
	}
	if full.ExternalIDs.UPC != "00602557631722" || full.ReleaseDatePrecision != "day" {
		t.Fatalf("meta decode mismatch: %+v", full.spAlbum)
	}
	if len(full.Copyrights) != 2 || full.Copyrights[0] != (spCopyright{Text: "℗ 2026 AOMG", Type: "P"}) {
		t.Fatalf("copyrights decode mismatch: %+v", full.Copyrights)
	}
	if len(full.Tracks.Items) != 1 || full.Tracks.Items[0].ID != "t1" {
		t.Fatalf("tracks decode mismatch: %+v", full.Tracks)
	}

	b, ok := copyrightsJSON(full.Copyrights).([]byte)
	if !ok || !strings.Contains(string(b), `"℗ 2026 AOMG"`) {
		t.Fatalf("copyrightsJSON = %v", copyrightsJSON(full.Copyrights))
	}
	if copyrightsJSON(nil) != nil {
		t.Fatal("copyrightsJSON(nil) should be nil for COALESCE to keep existing values")
	}
}

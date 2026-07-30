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

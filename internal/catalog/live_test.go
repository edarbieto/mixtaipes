package catalog

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveSpotify calls the real Spotify API: it runs only with LIVE_TESTS=1
// and SPOTIFY_CLIENT_ID / SPOTIFY_CLIENT_SECRET set.
func TestLiveSpotify(t *testing.T) {
	id, secret := os.Getenv("SPOTIFY_CLIENT_ID"), os.Getenv("SPOTIFY_CLIENT_SECRET")
	if os.Getenv("LIVE_TESTS") != "1" || id == "" || secret == "" {
		t.Skip("set LIVE_TESTS=1 and Spotify credentials to call the real API")
	}
	market := os.Getenv("SPOTIFY_MARKET")
	if market == "" {
		market = "US"
	}
	sp := NewSpotify(SpotifyConfig{ClientID: id, ClientSecret: secret})

	known := []Query{
		{Artist: "Massive Attack", Title: "Teardrop"},
		{Artist: "The Beatles", Title: "Here Comes the Sun"},
		{Artist: "Queen", Title: "Bohemian Rhapsody"},
		{Artist: "Daft Punk", Title: "Get Lucky"},
		{Artist: "Soda Stereo", Title: "De Música Ligera"},
		{Artist: "Café Tacvba", Title: "La Ingrata"},
		{Artist: "Sigur Rós", Title: "Hoppípolla"},
		{Artist: "Portishead", Title: "Glory Box"},
		{Artist: "The Rolling Stones", Title: "(I Can't Get No) Satisfaction"},
		{Artist: "Kavinsky", Title: "Nightcall"},
	}
	invented := Query{Artist: "Zyxquor Blenthaw", Title: "Velvet Marmalade Sunrise"}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := time.Now()
	matches, err := sp.Resolve(ctx, market, append(known, invented))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("market %s, %d queries in %v", market, len(matches), time.Since(start).Round(time.Millisecond))

	matched := 0
	for i, m := range matches[:len(known)] {
		t.Logf("  %-22s %-32s → %s %s %v", known[i].Artist, known[i].Title, m.Status, m.Track.Name, m.Track.Artists)
		if m.Status == StatusMatched {
			matched++
			if m.Track.ID == "" || m.Track.AlbumImageURL == "" {
				t.Errorf("matched track without id or image: %+v", m.Track)
			}
		}
	}
	if matched < len(known)-1 {
		t.Errorf("matched %d/%d known tracks", matched, len(known))
	}
	if last := matches[len(known)]; last.Status != StatusNotFound {
		t.Errorf("invented track matched: %+v", last.Track)
	}
}

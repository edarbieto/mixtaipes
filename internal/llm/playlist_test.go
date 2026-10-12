package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func tracksJSON(n int) string {
	items := make([]string, n)
	for i := range items {
		items[i] = fmt.Sprintf(`{"artist":"Artist %d","title":"Title %d","why":"Because %d"}`, i, i, i)
	}
	return "[" + strings.Join(items, ",") + "]"
}

func okJSON(tracks string) string {
	return `{"status":"ok","refusal_reason":null,"title":"Rainy drive","description":"Night music",` +
		`"tracks":` + tracks + `,"suggestions":["slower","more synth","add vocals"]}`
}

func TestParsePlaylist(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantErr    string
		wantTracks int
		wantReason RefusalReason
	}{
		{name: "valid ok", raw: okJSON(tracksJSON(20)), wantTracks: 20},
		{name: "clamps to 30 tracks", raw: okJSON(tracksJSON(50)), wantTracks: MaxTracks},
		{name: "not json", raw: `{"status":`, wantErr: "decode"},
		{name: "unknown status", raw: `{"status":"maybe"}`, wantErr: "unknown status"},
		{name: "ok without tracks", raw: okJSON(`[]`), wantErr: "no tracks"},
		{name: "ok with empty field", raw: okJSON(`[{"artist":" ","title":"x","why":"y"}]`), wantErr: "track 0"},
		{
			name:    "ok without title",
			raw:     `{"status":"ok","title":"  ","tracks":` + tracksJSON(1) + `}`,
			wantErr: "empty title",
		},
		{
			name:       "refused drops tracks",
			raw:        `{"status":"refused","refusal_reason":"off_topic","title":"","description":"","tracks":` + tracksJSON(2) + `,"suggestions":[]}`,
			wantReason: RefusalOffTopic,
		},
		{
			name:       "refused without reason defaults to unclear",
			raw:        `{"status":"refused","refusal_reason":null}`,
			wantReason: RefusalUnclear,
		},
		{
			name:    "refused with unknown reason",
			raw:     `{"status":"refused","refusal_reason":"bored"}`,
			wantErr: "unknown refusal_reason",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParsePlaylist([]byte(tt.raw))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(p.Tracks) != tt.wantTracks {
				t.Errorf("tracks = %d, want %d", len(p.Tracks), tt.wantTracks)
			}
			if tt.wantReason != "" {
				if p.RefusalReason == nil || *p.RefusalReason != tt.wantReason {
					t.Errorf("refusal_reason = %v, want %q", p.RefusalReason, tt.wantReason)
				}
			} else if p.RefusalReason != nil {
				t.Errorf("refusal_reason = %q, want nil", *p.RefusalReason)
			}
		})
	}
}

func TestParsePlaylistBounds(t *testing.T) {
	raw := fmt.Sprintf(`{"status":"ok","refusal_reason":"unsafe","title":%q,"description":%q,`+
		`"tracks":[{"artist":" A ","title":" T ","why":" W "}],"suggestions":["a"," ","b","c","d"]}`,
		strings.Repeat("ñ", 150), strings.Repeat("é", 400))
	p, err := ParsePlaylist([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(p.Title)); n != MaxTitleLen {
		t.Errorf("title runes = %d, want %d", n, MaxTitleLen)
	}
	if n := len([]rune(p.Description)); n != MaxDescriptionLen {
		t.Errorf("description runes = %d, want %d", n, MaxDescriptionLen)
	}
	if got := p.Tracks[0]; got != (Track{Artist: "A", Title: "T", Why: "W"}) {
		t.Errorf("track not trimmed: %+v", got)
	}
	if got := strings.Join(p.Suggestions, ","); got != "a,b,c" {
		t.Errorf("suggestions = %q, want a,b,c", got)
	}
	if p.RefusalReason != nil {
		t.Errorf("ok playlist kept refusal_reason %q", *p.RefusalReason)
	}
}

func TestRefusedMarshalsEmptyArrays(t *testing.T) {
	b, err := json.Marshal(Refused(RefusalUnsafe))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"status":"refused","refusal_reason":"unsafe","title":"","description":"","tracks":[],"suggestions":[]}`
	if string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
}

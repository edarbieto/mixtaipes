package catalog

import (
	"math"
	"testing"
)

func TestNormalizeTitle(t *testing.T) {
	tests := map[string]string{
		"Teardrop":                                     "teardrop",
		"Bohemian Rhapsody - Remastered 2011":          "bohemian rhapsody",
		"Here Comes the Sun (Remastered 2009)":         "here comes the sun",
		"Get Lucky (feat. Pharrell Williams)":          "get lucky",
		"Get Lucky feat. Pharrell Williams":            "get lucky",
		"Despacito ft. Daddy Yankee":                   "despacito",
		"Corazón Espinado [Live]":                      "corazon espinado",
		"(I Can't Get No) Satisfaction - Mono":         "satisfaction",
		"Don’t Stop Me Now":                            "dont stop me now",
		"Stay With Me":                                 "stay with me",
		"Hi-Fi Rush":                                   "hi fi rush",
		"(Untitled)":                                   "untitled",
		"  Ça   plane pour moi!  ":                     "ca plane pour moi",
		"Song 2":                                       "song 2",
		"Sin Documentos – En Directo":                  "sin documentos",
		"Wish You Were Here — 2011 Remastered Version": "wish you were here",
	}
	for in, want := range tests {
		if got := NormalizeTitle(in); got != want {
			t.Errorf("NormalizeTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeArtist(t *testing.T) {
	tests := map[string]string{
		"The Cure":                 "cure",
		"Cure":                     "cure",
		"Café Tacvba":              "cafe tacvba",
		"Café Tacuba":              "cafe tacuba",
		"AC/DC":                    "ac dc",
		"Daft Punk feat. Pharrell": "daft punk",
		"Simon & Garfunkel":        "simon garfunkel",
		"Theory of a Deadman":      "theory of a deadman",
		"Sigur Rós":                "sigur ros",
		"坂本龍一":                     "坂本龍一",
	}
	for in, want := range tests {
		if got := NormalizeArtist(in); got != want {
			t.Errorf("NormalizeArtist(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSimilarity(t *testing.T) {
	tests := []struct {
		a, b string
		want float64
	}{
		{"", "", 1},
		{"abc", "abc", 1},
		{"abc", "", 0},
		{"kitten", "sitting", 1 - 3.0/7},
		{"corazon", "corazón", 1 - 1.0/7},
	}
	for _, tt := range tests {
		if got := Similarity(tt.a, tt.b); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("Similarity(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestPick(t *testing.T) {
	teardrop := Track{ID: "1", Name: "Teardrop", Artists: []string{"Massive Attack"}}
	cover := Track{ID: "2", Name: "Teardrop", Artists: []string{"Newton Faulkner"}}
	remaster := Track{ID: "3", Name: "Here Comes The Sun - Remastered 2009", Artists: []string{"The Beatles"}}
	duet := Track{ID: "4", Name: "Under Pressure", Artists: []string{"Queen", "David Bowie"}}
	near := Track{ID: "5", Name: "Teardrops", Artists: []string{"Massive Attack"}}
	other := Track{ID: "6", Name: "Angel", Artists: []string{"Massive Attack"}}

	tests := []struct {
		name       string
		q          Query
		candidates []Track
		wantID     string
	}{
		{"exact", Query{"Massive Attack", "Teardrop"}, []Track{teardrop}, "1"},
		{"skips other artist's cover", Query{"Massive Attack", "Teardrop"}, []Track{cover, teardrop}, "1"},
		{"remaster suffix and leading the", Query{"Beatles", "Here Comes the Sun"}, []Track{remaster}, "3"},
		{"any credited artist", Query{"David Bowie", "Under Pressure"}, []Track{duet}, "4"},
		{"small typo within threshold", Query{"Massive Attack", "Teardrop"}, []Track{near}, "5"},
		{"different song by same artist", Query{"Massive Attack", "Teardrop"}, []Track{other}, ""},
		{"wrong artist only", Query{"Massive Attack", "Teardrop"}, []Track{cover}, ""},
		{"no candidates", Query{"Massive Attack", "Teardrop"}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Pick(tt.q, tt.candidates)
			if ok != (tt.wantID != "") || got.ID != tt.wantID {
				t.Errorf("Pick = %q, %v; want %q", got.ID, ok, tt.wantID)
			}
		})
	}
}

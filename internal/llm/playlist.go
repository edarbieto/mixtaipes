package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Server-side bounds applied to every playlist, independent of the prompt (ADR-004).
const (
	MaxTracks         = 30
	MaxTitleLen       = 100
	MaxDescriptionLen = 300
	MaxSuggestions    = 3
)

// Status is the outcome the model reports for a request.
type Status string

// Playlist statuses.
const (
	StatusOK      Status = "ok"
	StatusRefused Status = "refused"
)

// RefusalReason is the typed reason for a refusal; the UI localizes it.
type RefusalReason string

// Refusal reasons.
const (
	RefusalOffTopic RefusalReason = "off_topic"
	RefusalUnsafe   RefusalReason = "unsafe"
	RefusalUnclear  RefusalReason = "unclear"
)

// Track is one proposed track, as written by the model.
type Track struct {
	Artist string `json:"artist"`
	Title  string `json:"title"`
	Why    string `json:"why"`
}

// Playlist is the validated model output, persisted verbatim in turns.llm_output.
type Playlist struct {
	Status        Status         `json:"status"`
	RefusalReason *RefusalReason `json:"refusal_reason"`
	Title         string         `json:"title"`
	Description   string         `json:"description"`
	Tracks        []Track        `json:"tracks"`
	Suggestions   []string       `json:"suggestions"`
}

// PlaylistSchema is the JSON Schema sent to the provider for structured output.
var PlaylistSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"status": map[string]any{
			"type": "string",
			"enum": []string{string(StatusOK), string(StatusRefused)},
		},
		"refusal_reason": map[string]any{
			"anyOf": []any{
				map[string]any{
					"type": "string",
					"enum": []string{string(RefusalOffTopic), string(RefusalUnsafe), string(RefusalUnclear)},
				},
				map[string]any{"type": "null"},
			},
		},
		"title":       map[string]any{"type": "string", "maxLength": MaxTitleLen},
		"description": map[string]any{"type": "string", "maxLength": MaxDescriptionLen},
		"tracks": map[string]any{
			"type":     "array",
			"maxItems": MaxTracks,
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"artist": map[string]any{"type": "string"},
					"title":  map[string]any{"type": "string"},
					"why":    map[string]any{"type": "string"},
				},
				"required": []string{"artist", "title", "why"},
			},
		},
		"suggestions": map[string]any{
			"type":     "array",
			"maxItems": MaxSuggestions,
			"items":    map[string]any{"type": "string"},
		},
	},
	"required": []string{"status", "refusal_reason", "title", "description", "tracks", "suggestions"},
}

// ParsePlaylist decodes and validates raw model output. It applies the
// server-side bounds (track count, title and description length) and
// rejects output the UI cannot render.
func ParsePlaylist(raw []byte) (Playlist, error) {
	var p Playlist
	if err := json.Unmarshal(raw, &p); err != nil {
		return Playlist{}, fmt.Errorf("decode: %w", err)
	}
	p.Title = truncate(strings.TrimSpace(p.Title), MaxTitleLen)
	p.Description = truncate(strings.TrimSpace(p.Description), MaxDescriptionLen)
	p.Suggestions = cleanSuggestions(p.Suggestions)

	switch p.Status {
	case StatusRefused:
		if p.RefusalReason == nil {
			reason := RefusalUnclear
			p.RefusalReason = &reason
		}
		switch *p.RefusalReason {
		case RefusalOffTopic, RefusalUnsafe, RefusalUnclear:
		default:
			return Playlist{}, fmt.Errorf("unknown refusal_reason %q", *p.RefusalReason)
		}
		p.Tracks = []Track{}
		return p, nil
	case StatusOK:
		p.RefusalReason = nil
	default:
		return Playlist{}, fmt.Errorf("unknown status %q", p.Status)
	}

	if p.Title == "" {
		return Playlist{}, errors.New("empty title")
	}
	if len(p.Tracks) == 0 {
		return Playlist{}, errors.New("no tracks")
	}
	if len(p.Tracks) > MaxTracks {
		p.Tracks = p.Tracks[:MaxTracks]
	}
	for i, t := range p.Tracks {
		t = Track{
			Artist: strings.TrimSpace(t.Artist),
			Title:  strings.TrimSpace(t.Title),
			Why:    strings.TrimSpace(t.Why),
		}
		if t.Artist == "" || t.Title == "" || t.Why == "" {
			return Playlist{}, fmt.Errorf("track %d has an empty field", i)
		}
		p.Tracks[i] = t
	}
	return p, nil
}

// Refused returns a refusal playlist for the given reason.
func Refused(reason RefusalReason) Playlist {
	return Playlist{
		Status:        StatusRefused,
		RefusalReason: &reason,
		Tracks:        []Track{},
		Suggestions:   []string{},
	}
}

func cleanSuggestions(in []string) []string {
	out := make([]string, 0, MaxSuggestions)
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" && len(out) < MaxSuggestions {
			out = append(out, s)
		}
	}
	return out
}

func truncate(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:maxRunes]))
}

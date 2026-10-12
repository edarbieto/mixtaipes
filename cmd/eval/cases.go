package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/edarbieto/mixtaipes/internal/llm"
)

// Case is one evaluation case from evals/cases.yaml (ADR-005).
type Case struct {
	ID       string   `yaml:"id"`
	Category string   `yaml:"category"`
	Locale   string   `yaml:"locale"`
	Turns    []string `yaml:"turns"`
	Expect   Expect   `yaml:"expect"`
}

// Expect is checked against the last turn of a case.
type Expect struct {
	Status           llm.Status        `yaml:"status"`
	RefusalReason    llm.RefusalReason `yaml:"refusal_reason"`
	MinTracks        int               `yaml:"min_tracks"`
	MaxTracks        int               `yaml:"max_tracks"`
	TitleNotContains string            `yaml:"title_not_contains"`
}

func loadCases(path string) ([]Case, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []Case
	if err := yaml.Unmarshal(b, &cases); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	seen := make(map[string]bool)
	for i, c := range cases {
		switch {
		case c.ID == "" || seen[c.ID]:
			return nil, fmt.Errorf("case %d: missing or duplicate id %q", i, c.ID)
		case len(c.Turns) == 0:
			return nil, fmt.Errorf("case %s: no turns", c.ID)
		case c.Expect.Status != llm.StatusOK && c.Expect.Status != llm.StatusRefused:
			return nil, fmt.Errorf("case %s: expect.status must be ok or refused", c.ID)
		}
		seen[c.ID] = true
	}
	return cases, nil
}

// check returns the expectation failures of a case's last turn; earlier
// turns of an ok case must be ok too.
func (c Case) check(turns []TurnResult) []string {
	var fails []string
	if len(turns) != len(c.Turns) {
		return []string{"not all turns ran"}
	}
	for i, t := range turns {
		if t.Error != "" {
			fails = append(fails, fmt.Sprintf("turn %d failed: %s", i+1, t.Error))
		}
		if i < len(turns)-1 && c.Expect.Status == llm.StatusOK && t.Status != llm.StatusOK {
			fails = append(fails, fmt.Sprintf("turn %d is %s", i+1, t.Status))
		}
	}
	if len(fails) > 0 {
		return fails
	}

	last := turns[len(turns)-1]
	if last.Status != c.Expect.Status {
		fails = append(fails, fmt.Sprintf("status %s, want %s", last.Status, c.Expect.Status))
	}
	if c.Expect.RefusalReason != "" && last.RefusalReason != c.Expect.RefusalReason {
		fails = append(fails, fmt.Sprintf("refusal_reason %q, want %q", last.RefusalReason, c.Expect.RefusalReason))
	}
	if c.Expect.MinTracks > 0 && last.Requested < c.Expect.MinTracks {
		fails = append(fails, fmt.Sprintf("%d tracks, want at least %d", last.Requested, c.Expect.MinTracks))
	}
	if c.Expect.MaxTracks > 0 && last.Requested > c.Expect.MaxTracks {
		fails = append(fails, fmt.Sprintf("%d tracks, want at most %d", last.Requested, c.Expect.MaxTracks))
	}
	if s := c.Expect.TitleNotContains; s != "" && strings.Contains(strings.ToLower(last.Title), strings.ToLower(s)) {
		fails = append(fails, fmt.Sprintf("title %q contains %q", last.Title, s))
	}
	return fails
}

var errNoCases = errors.New("no cases selected")

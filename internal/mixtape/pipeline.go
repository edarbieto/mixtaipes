// Package mixtape is the domain service: it runs turns (LLM generation,
// catalog verification, backfill) and, later, persists and shares them.
package mixtape

import (
	"context"
	"time"

	"github.com/edarbieto/mixtaipes/internal/catalog"
	"github.com/edarbieto/mixtaipes/internal/llm"
	"github.com/edarbieto/mixtaipes/prompts"
)

// Track is one playlist entry: the LLM's text plus its catalog resolution.
type Track struct {
	llm.Track
	Match      catalog.Match
	Backfilled bool
}

// Outcome is the result of one turn.
type Outcome struct {
	// Playlist is the validated output of the main call, stored as llm_output.
	Playlist llm.Playlist
	// Tracks is the final, deduplicated list; empty unless Playlist is ok.
	Tracks []Track
	// LLM sums the main and backfill calls.
	LLM llm.Usage
	// GenerateCalls counts the main call's attempts; 1 means valid on the first try.
	GenerateCalls  int
	CatalogLatency time.Duration
	// Requested is the number of tracks the main call proposed.
	Requested int
	// MatchedBeforeBackfill counts matches among the proposed tracks (ADR-005).
	MatchedBeforeBackfill int
	// Duplicates counts matched tracks dropped because their catalog id repeated.
	Duplicates int
	// BackfillErr is set when the backfill round failed; the turn still succeeds.
	BackfillErr error
}

// Matched returns the number of matched tracks in the final list.
func (o Outcome) Matched() int {
	n := 0
	for _, t := range o.Tracks {
		if t.Match.Status == catalog.StatusMatched {
			n++
		}
	}
	return n
}

// Pipeline runs a turn against an LLM provider and a catalog (ADR-006).
type Pipeline struct {
	LLM     llm.Provider
	Catalog catalog.Catalog
}

// Run generates a playlist for req, verifies every track in market, drops
// duplicates and runs at most one backfill round for misses. It fails only
// when the main LLM call or the first catalog round fails; the returned
// Outcome still carries the LLM usage for quota accounting.
func (p Pipeline) Run(ctx context.Context, market string, req llm.Request) (Outcome, error) {
	res, err := llm.GeneratePlaylist(ctx, p.LLM, req)
	out := Outcome{Playlist: res.Playlist, LLM: res.Usage, GenerateCalls: res.Calls}
	if err != nil || res.Playlist.Status != llm.StatusOK {
		return out, err
	}
	out.Requested = len(res.Playlist.Tracks)

	matches, err := p.resolve(ctx, market, res.Playlist.Tracks, &out)
	if err != nil {
		return out, err
	}
	seen := make(map[string]bool)
	var notFound []llm.Track
	for i, m := range matches {
		t := Track{Track: res.Playlist.Tracks[i], Match: m}
		if m.Status == catalog.StatusMatched {
			out.MatchedBeforeBackfill++
			if seen[m.Track.ID] {
				out.Duplicates++
				continue
			}
			seen[m.Track.ID] = true
		} else {
			notFound = append(notFound, t.Track)
		}
		out.Tracks = append(out.Tracks, t)
	}

	if n := len(notFound) + out.Duplicates; n > 0 {
		out.BackfillErr = p.backfill(ctx, market, req, res.Playlist, notFound, n, seen, &out)
	}
	return out, nil
}

// backfill asks for n replacements once and puts the matched ones in the
// slots of tracks that were not found, appending any extra ones.
func (p Pipeline) backfill(ctx context.Context, market string, base llm.Request, playlist llm.Playlist,
	notFound []llm.Track, n int, seen map[string]bool, out *Outcome) error {
	req, err := prompts.BuildBackfill(base, playlist, notFound, n)
	if err != nil {
		return err
	}
	replacements, usage, err := llm.GenerateReplacements(ctx, p.LLM, req, n)
	out.LLM.Add(usage)
	if err != nil || len(replacements) == 0 {
		return err
	}
	matches, err := p.resolve(ctx, market, replacements, out)
	if err != nil {
		return err
	}

	var slots []int
	for i, t := range out.Tracks {
		if t.Match.Status != catalog.StatusMatched {
			slots = append(slots, i)
		}
	}
	for i, m := range matches {
		if m.Status != catalog.StatusMatched || seen[m.Track.ID] {
			continue
		}
		seen[m.Track.ID] = true
		t := Track{Track: replacements[i], Match: m, Backfilled: true}
		if len(slots) > 0 {
			out.Tracks[slots[0]] = t
			slots = slots[1:]
		} else {
			out.Tracks = append(out.Tracks, t)
		}
	}
	return nil
}

func (p Pipeline) resolve(ctx context.Context, market string, tracks []llm.Track, out *Outcome) ([]catalog.Match, error) {
	queries := make([]catalog.Query, len(tracks))
	for i, t := range tracks {
		queries[i] = catalog.Query{Artist: t.Artist, Title: t.Title}
	}
	start := time.Now()
	matches, err := p.Catalog.Resolve(ctx, market, queries)
	out.CatalogLatency += time.Since(start)
	return matches, err
}

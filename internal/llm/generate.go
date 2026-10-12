package llm

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// maxAttempts bounds generation to one retry on invalid output (ADR-003).
const maxAttempts = 2

// Usage is the accounting of one or more provider calls.
type Usage struct {
	Model        string
	Calls        int
	InputTokens  int
	OutputTokens int
	Latency      time.Duration
}

// Add accumulates another usage; Model becomes the latest non-empty one.
func (u *Usage) Add(o Usage) {
	if o.Model != "" {
		u.Model = o.Model
	}
	u.Calls += o.Calls
	u.InputTokens += o.InputTokens
	u.OutputTokens += o.OutputTokens
	u.Latency += o.Latency
}

// Result is a validated playlist plus accounting summed across attempts.
type Result struct {
	Playlist Playlist
	Usage
}

// GeneratePlaylist calls the provider and validates its output, retrying
// once when the output is invalid. A response blocked by provider safety
// filters becomes an "unsafe" refusal. On error, the returned Result still
// carries the calls and tokens spent, for quota accounting.
func GeneratePlaylist(ctx context.Context, p Provider, req Request) (Result, error) {
	pl, usage, err := generate(ctx, p, req, ParsePlaylist)
	if errors.Is(err, ErrBlocked) {
		return Result{Playlist: Refused(RefusalUnsafe), Usage: usage}, nil
	}
	return Result{Playlist: pl, Usage: usage}, err
}

// GenerateReplacements asks for up to n replacement tracks (ADR-006 backfill)
// with the same single retry on invalid output. A blocked response yields
// no replacements.
func GenerateReplacements(ctx context.Context, p Provider, req Request, n int) ([]Track, Usage, error) {
	tracks, usage, err := generate(ctx, p, req, func(raw []byte) ([]Track, error) {
		return ParseReplacements(raw, n)
	})
	if errors.Is(err, ErrBlocked) {
		return nil, usage, nil
	}
	return tracks, usage, err
}

func generate[T any](ctx context.Context, p Provider, req Request, parse func([]byte) (T, error)) (T, Usage, error) {
	var zero T
	var usage Usage
	var lastErr error
	for range maxAttempts {
		resp, err := p.Generate(ctx, req)
		usage.Add(Usage{
			Model:        resp.Model,
			Calls:        1,
			InputTokens:  resp.InputTokens,
			OutputTokens: resp.OutputTokens,
			Latency:      resp.Latency,
		})
		if err != nil {
			return zero, usage, err
		}
		v, err := parse(resp.JSON)
		if err == nil {
			return v, usage, nil
		}
		lastErr = err
	}
	return zero, usage, fmt.Errorf("%w: %w", ErrInvalidOutput, lastErr)
}

package llm

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// maxAttempts bounds generation to one retry on invalid output (ADR-003).
const maxAttempts = 2

// Result is a validated playlist plus accounting summed across attempts.
type Result struct {
	Playlist     Playlist
	Model        string
	Calls        int
	InputTokens  int
	OutputTokens int
	Latency      time.Duration
}

// GeneratePlaylist calls the provider and validates its output, retrying
// once when the output is invalid. A response blocked by provider safety
// filters becomes an "unsafe" refusal. On error, the returned Result still
// carries the calls and tokens spent, for quota accounting.
func GeneratePlaylist(ctx context.Context, p Provider, req Request) (Result, error) {
	var res Result
	var lastErr error
	for range maxAttempts {
		resp, err := p.Generate(ctx, req)
		res.Calls++
		res.Model = resp.Model
		res.InputTokens += resp.InputTokens
		res.OutputTokens += resp.OutputTokens
		res.Latency += resp.Latency
		if errors.Is(err, ErrBlocked) {
			res.Playlist = Refused(RefusalUnsafe)
			return res, nil
		}
		if err != nil {
			return res, err
		}
		pl, err := ParsePlaylist(resp.JSON)
		if err == nil {
			res.Playlist = pl
			return res, nil
		}
		lastErr = err
	}
	return res, fmt.Errorf("%w: %w", ErrInvalidOutput, lastErr)
}

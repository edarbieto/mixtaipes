// Command eval runs the prompt evaluation cases through the real turn
// pipeline (Gemini + Spotify) and prints the ADR-005 metrics. It spends real
// quota: run it deliberately, not in CI.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/edarbieto/mixtaipes/internal/catalog"
	"github.com/edarbieto/mixtaipes/internal/llm"
	"github.com/edarbieto/mixtaipes/internal/mixtape"
	"github.com/edarbieto/mixtaipes/prompts"
)

// Retries of a turn after the provider stays unavailable, mirroring the
// worker backoff of ADR-015.
var retryBackoff = []time.Duration{5 * time.Second, 15 * time.Second}

type options struct {
	casesPath string
	outDir    string
	version   string
	market    string
	only      string
	pause     time.Duration
	tracks    bool
}

func main() {
	var o options
	flag.StringVar(&o.casesPath, "cases", "evals/cases.yaml", "evaluation cases")
	flag.StringVar(&o.outDir, "out", "evals/results", "directory for the JSON results")
	flag.StringVar(&o.version, "prompt", prompts.Latest(), "prompt version")
	flag.StringVar(&o.market, "market", envOr("SPOTIFY_MARKET", "US"), "Spotify market")
	flag.StringVar(&o.only, "only", "", "comma-separated case ids or categories to run")
	flag.DurationVar(&o.pause, "pause", 3*time.Second, "pause between turns (provider rate limits)")
	flag.BoolVar(&o.tracks, "tracks", true, "print every playlist for manual coherence review")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, o); err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, o options) error {
	cases, err := loadCases(o.casesPath)
	if err != nil {
		return err
	}
	cases = selectCases(cases, o.only)
	if len(cases) == 0 {
		return errNoCases
	}
	if _, err := prompts.Load(o.version); err != nil {
		return err
	}
	env, err := requireEnv("GEMINI_API_KEY", "GEMINI_MODEL", "SPOTIFY_CLIENT_ID", "SPOTIFY_CLIENT_SECRET")
	if err != nil {
		return err
	}
	pipeline := mixtape.Pipeline{
		LLM: llm.NewGemini(llm.GeminiConfig{
			APIKey:        env["GEMINI_API_KEY"],
			Model:         env["GEMINI_MODEL"],
			FallbackModel: os.Getenv("GEMINI_FALLBACK_MODEL"),
		}),
		Catalog: catalog.NewSpotify(catalog.SpotifyConfig{
			ClientID:     env["SPOTIFY_CLIENT_ID"],
			ClientSecret: env["SPOTIFY_CLIENT_SECRET"],
		}),
	}

	report := Report{
		PromptVersion: o.version,
		Model:         env["GEMINI_MODEL"],
		Market:        o.market,
		StartedAt:     time.Now().UTC(),
	}
	fmt.Printf("prompt %s · model %s · market %s · %d cases\n\n", o.version, report.Model, o.market, len(cases))
	for i, c := range cases {
		if ctx.Err() != nil {
			fmt.Println("interrupted; reporting completed cases")
			break
		}
		if i > 0 {
			sleep(ctx, o.pause)
		}
		res := runCase(ctx, pipeline, o, c)
		report.Cases = append(report.Cases, res)
		printCase(res, o.tracks)
	}
	report.Summary = summarize(cases, report.Cases)
	printSummary(report.Summary)

	path, err := writeReport(o.outDir, report)
	if err != nil {
		return err
	}
	fmt.Printf("\nresults: %s\n", path)
	return nil
}

// Report is the JSON written to evals/results.
type Report struct {
	PromptVersion string       `json:"prompt_version"`
	Model         string       `json:"model"`
	Market        string       `json:"market"`
	StartedAt     time.Time    `json:"started_at"`
	Summary       Summary      `json:"summary"`
	Cases         []CaseResult `json:"cases"`
}

// CaseResult is the outcome of one case.
type CaseResult struct {
	ID       string       `json:"id"`
	Category string       `json:"category"`
	Expect   Expect       `json:"expect"`
	Turns    []TurnResult `json:"turns"`
	Failures []string     `json:"failures,omitempty"`
}

// TurnResult is the outcome of one turn of a case.
type TurnResult struct {
	Prompt                string            `json:"prompt"`
	Status                llm.Status        `json:"status,omitempty"`
	RefusalReason         llm.RefusalReason `json:"refusal_reason,omitempty"`
	Error                 string            `json:"error,omitempty"`
	BackfillError         string            `json:"backfill_error,omitempty"`
	Attempts              int               `json:"attempts"`
	Title                 string            `json:"title,omitempty"`
	Description           string            `json:"description,omitempty"`
	Suggestions           []string          `json:"suggestions,omitempty"`
	Tracks                []TrackResult     `json:"tracks,omitempty"`
	Requested             int               `json:"requested"`
	MatchedBeforeBackfill int               `json:"matched_before_backfill"`
	Matched               int               `json:"matched"`
	Duplicates            int               `json:"duplicates"`
	Model                 string            `json:"model,omitempty"`
	GenerateCalls         int               `json:"generate_calls"`
	LLMCalls              int               `json:"llm_calls"`
	InputTokens           int               `json:"input_tokens"`
	OutputTokens          int               `json:"output_tokens"`
	LLMLatencyMS          int64             `json:"llm_latency_ms"`
	CatalogLatencyMS      int64             `json:"catalog_latency_ms"`
	LatencyMS             int64             `json:"latency_ms"`
}

// TrackResult is one final track of a turn.
type TrackResult struct {
	Artist      string         `json:"artist"`
	Title       string         `json:"title"`
	Why         string         `json:"why"`
	Match       catalog.Status `json:"match"`
	SpotifyName string         `json:"spotify_name,omitempty"`
	SpotifyBy   []string       `json:"spotify_artists,omitempty"`
	Backfilled  bool           `json:"backfilled,omitempty"`
}

// runCase runs a case's turns in order. Following ADR-008, the context of a
// refinement is the first ok request, later ok refinements and the latest ok
// playlist; refused turns are left out.
func runCase(ctx context.Context, p mixtape.Pipeline, o options, c Case) CaseResult {
	res := CaseResult{ID: c.ID, Category: c.Category, Expect: c.Expect}
	var (
		original string
		previous []string
		current  *llm.Playlist
	)
	for i, text := range c.Turns {
		if i > 0 {
			sleep(ctx, o.pause)
		}
		var req llm.Request
		var err error
		if current == nil {
			req, err = prompts.Build(o.version, c.Locale, text)
		} else {
			req, err = prompts.BuildRefine(o.version, c.Locale, prompts.Refinement{
				Original: original, Previous: previous, Current: *current, Instruction: text,
			})
		}
		if err != nil {
			res.Turns = append(res.Turns, TurnResult{Prompt: text, Error: err.Error()})
			break
		}

		turn, out := runTurn(ctx, p, o.market, text, req)
		res.Turns = append(res.Turns, turn)
		if turn.Error != "" {
			break
		}
		if out.Playlist.Status == llm.StatusOK {
			if current == nil {
				original = text
			} else {
				previous = append(previous, text)
			}
			pl := out.Playlist
			current = &pl
		}
	}
	res.Failures = c.check(res.Turns)
	return res
}

// runTurn runs one turn, retrying with backoff while the provider is unavailable.
func runTurn(ctx context.Context, p mixtape.Pipeline, market, text string, req llm.Request) (TurnResult, mixtape.Outcome) {
	start := time.Now()
	turn := TurnResult{Prompt: text}
	var out mixtape.Outcome
	var err error
	var spent llm.Usage
	for attempt := 0; ; attempt++ {
		turn.Attempts = attempt + 1
		out, err = p.Run(ctx, market, req)
		spent.Add(out.LLM)
		if !errors.Is(err, llm.ErrUnavailable) || attempt == len(retryBackoff) {
			break
		}
		sleep(ctx, retryBackoff[attempt])
	}
	turn.LatencyMS = time.Since(start).Milliseconds()
	turn.Model = spent.Model
	turn.LLMCalls = spent.Calls
	turn.InputTokens = spent.InputTokens
	turn.OutputTokens = spent.OutputTokens
	if err != nil {
		turn.Error = err.Error()
		return turn, out
	}

	pl := out.Playlist
	turn.Status = pl.Status
	if pl.RefusalReason != nil {
		turn.RefusalReason = *pl.RefusalReason
	}
	if out.BackfillErr != nil {
		turn.BackfillError = out.BackfillErr.Error()
	}
	turn.Title, turn.Description, turn.Suggestions = pl.Title, pl.Description, pl.Suggestions
	turn.Requested = out.Requested
	turn.MatchedBeforeBackfill = out.MatchedBeforeBackfill
	turn.Matched = out.Matched()
	turn.Duplicates = out.Duplicates
	turn.GenerateCalls = out.GenerateCalls
	turn.LLMLatencyMS = out.LLM.Latency.Milliseconds()
	turn.CatalogLatencyMS = out.CatalogLatency.Milliseconds()
	for _, t := range out.Tracks {
		turn.Tracks = append(turn.Tracks, TrackResult{
			Artist: t.Artist, Title: t.Title, Why: t.Why,
			Match: t.Match.Status, SpotifyName: t.Match.Track.Name, SpotifyBy: t.Match.Track.Artists,
			Backfilled: t.Backfilled,
		})
	}
	return turn, out
}

func printCase(c CaseResult, tracks bool) {
	mark := "PASS"
	if len(c.Failures) > 0 {
		mark = "FAIL"
	}
	fmt.Printf("%s %s [%s]\n", mark, c.ID, c.Category)
	for i, t := range c.Turns {
		fmt.Printf("  %d. %q\n", i+1, t.Prompt)
		if t.Error != "" {
			fmt.Printf("     error after %d attempt(s): %s\n", t.Attempts, t.Error)
			continue
		}
		status := string(t.Status)
		if t.RefusalReason != "" {
			status += " (" + string(t.RefusalReason) + ")"
		}
		fmt.Printf("     %s · %d proposed · %d matched before backfill · %d final matched · %d dup · %d LLM calls · %.1fs\n",
			status, t.Requested, t.MatchedBeforeBackfill, t.Matched, t.Duplicates, t.LLMCalls, float64(t.LatencyMS)/1000)
		if t.Title != "" {
			fmt.Printf("     %q — %s\n", t.Title, t.Description)
		}
		if t.BackfillError != "" {
			fmt.Printf("     backfill failed: %s\n", t.BackfillError)
		}
		if tracks {
			for j, tr := range t.Tracks {
				flag := " "
				switch {
				case tr.Match != catalog.StatusMatched:
					flag = "✗"
				case tr.Backfilled:
					flag = "+"
				}
				fmt.Printf("     %s %2d %s — %s\n", flag, j+1, tr.Artist, tr.Title)
			}
		}
	}
	for _, f := range c.Failures {
		fmt.Printf("  ! %s\n", f)
	}
	fmt.Println()
}

func writeReport(dir string, r Report) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s.json", r.PromptVersion, r.StartedAt.Format("2006-01-02-1504"))
	path := filepath.Join(dir, name)
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, append(b, '\n'), 0o644)
}

func selectCases(cases []Case, only string) []Case {
	if only == "" {
		return cases
	}
	want := strings.Split(only, ",")
	return slices.DeleteFunc(slices.Clone(cases), func(c Case) bool {
		return !slices.Contains(want, c.ID) && !slices.Contains(want, c.Category)
	})
}

func requireEnv(keys ...string) (map[string]string, error) {
	env := make(map[string]string, len(keys))
	var missing []string
	for _, k := range keys {
		if env[k] = os.Getenv(k); env[k] == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing environment: %s", strings.Join(missing, ", "))
	}
	return env, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

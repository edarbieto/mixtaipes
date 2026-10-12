package main

import (
	"fmt"
	"math"
	"slices"

	"github.com/edarbieto/mixtaipes/internal/llm"
)

// MVP targets from ADR-005.
const (
	targetSchemaValid    = 0.98
	targetCorrectRefusal = 0.95
	targetWrongRefusal   = 0.0
	targetMatchRate      = 0.90
	targetP95Seconds     = 15.0
)

// Summary holds the ADR-005 metrics of a run.
type Summary struct {
	Cases       int `json:"cases"`
	CasesPassed int `json:"cases_passed"`
	Turns       int `json:"turns"`
	TurnErrors  int `json:"turn_errors"`
	Retried     int `json:"turns_retried"`

	// Ratios are nil when nothing was measured (e.g. no refusal cases ran).
	SchemaValidFirstTry *float64 `json:"schema_valid_first_try"`
	CorrectRefusals     *float64 `json:"correct_refusals"`
	WrongRefusals       *float64 `json:"wrong_refusals"`
	MatchRateBefore     *float64 `json:"match_rate_before_backfill"`
	MatchRateAfter      *float64 `json:"match_rate_after_backfill"`
	Duplicates          int      `json:"duplicates"`
	MaxDuplicates       int      `json:"max_duplicates_per_playlist"`
	BackfillTurns       int      `json:"backfill_turns"`
	P50Seconds          float64  `json:"p50_turn_seconds"`
	P95Seconds          float64  `json:"p95_turn_seconds"`
	LLMCalls            int      `json:"llm_calls"`
	InputTokens         int      `json:"input_tokens"`
	OutputTokens        int      `json:"output_tokens"`
}

func summarize(cases []Case, results []CaseResult) Summary {
	var s Summary
	s.Cases = len(results)
	var (
		generations, validFirst      int
		refusalCases, correctRefused int
		okCases, wronglyRefused      int
		requested, before, after     int
		latencies                    []float64
	)
	expect := make(map[string]Expect, len(cases))
	for _, c := range cases {
		expect[c.ID] = c.Expect
	}

	for _, c := range results {
		if len(c.Failures) == 0 {
			s.CasesPassed++
		}
		exp := expect[c.ID]
		refused := false
		for _, t := range c.Turns {
			s.Turns++
			s.LLMCalls += t.LLMCalls
			s.InputTokens += t.InputTokens
			s.OutputTokens += t.OutputTokens
			if t.Attempts > 1 {
				s.Retried++
			}
			if t.Error != "" {
				s.TurnErrors++
				continue
			}
			latencies = append(latencies, float64(t.LatencyMS)/1000)
			generations++
			if t.GenerateCalls == 1 {
				validFirst++
			}
			if t.Status == llm.StatusRefused {
				refused = true
			}
			if t.Status == llm.StatusOK {
				requested += t.Requested
				before += t.MatchedBeforeBackfill
				after += t.Matched
				s.Duplicates += t.Duplicates
				s.MaxDuplicates = max(s.MaxDuplicates, t.Duplicates)
				if t.LLMCalls > t.GenerateCalls {
					s.BackfillTurns++
				}
			}
		}
		last := len(c.Turns) - 1
		completed := last >= 0 && c.Turns[last].Error == ""
		switch {
		case exp.Status == llm.StatusRefused && completed:
			refusalCases++
			if c.Turns[last].Status == llm.StatusRefused {
				correctRefused++
			}
		case exp.Status == llm.StatusOK && (completed || refused):
			okCases++
			if refused {
				wronglyRefused++
			}
		}
	}

	s.SchemaValidFirstTry = ratio(validFirst, generations)
	s.CorrectRefusals = ratio(correctRefused, refusalCases)
	s.WrongRefusals = ratio(wronglyRefused, okCases)
	s.MatchRateBefore = ratio(before, requested)
	s.MatchRateAfter = ratio(after, requested)
	s.P50Seconds = percentile(latencies, 0.50)
	s.P95Seconds = percentile(latencies, 0.95)
	return s
}

func ratio(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	r := float64(n) / float64(d)
	return &r
}

// percentile uses the nearest-rank method.
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	rank := int(math.Ceil(p*float64(len(sorted)))) - 1
	return sorted[max(rank, 0)]
}

func printSummary(s Summary) {
	check := func(ok bool) string {
		if ok {
			return "✓"
		}
		return "✗"
	}
	meets := func(v *float64, ok func(float64) bool) string {
		if v == nil {
			return "–"
		}
		return check(ok(*v))
	}
	pct := func(v *float64) string {
		if v == nil {
			return "n/a"
		}
		return fmt.Sprintf("%.1f %%", *v*100)
	}
	atLeast := func(t float64) func(float64) bool { return func(v float64) bool { return v >= t } }
	fmt.Printf("== summary: %d/%d cases passed · %d turns · %d errors · %d retried ==\n",
		s.CasesPassed, s.Cases, s.Turns, s.TurnErrors, s.Retried)
	fmt.Printf("| Metric | Value | Target | |\n|---|---|---|---|\n")
	fmt.Printf("| Schema-valid on first try | %s | ≥ 98 %% | %s |\n", pct(s.SchemaValidFirstTry), meets(s.SchemaValidFirstTry, atLeast(targetSchemaValid)))
	fmt.Printf("| Correct refusals | %s | ≥ 95 %% | %s |\n", pct(s.CorrectRefusals), meets(s.CorrectRefusals, atLeast(targetCorrectRefusal)))
	fmt.Printf("| Wrong refusals | %s | 0 %% | %s |\n", pct(s.WrongRefusals), meets(s.WrongRefusals, func(v float64) bool { return v <= targetWrongRefusal }))
	fmt.Printf("| Match rate before backfill | %s | ≥ 90 %% | %s |\n", pct(s.MatchRateBefore), meets(s.MatchRateBefore, atLeast(targetMatchRate)))
	fmt.Printf("| Match rate after backfill | %s | — | |\n", pct(s.MatchRateAfter))
	fmt.Printf("| Duplicate tracks (total / max per playlist) | %d / %d | 0 | %s |\n", s.Duplicates, s.MaxDuplicates, check(s.Duplicates == 0))
	fmt.Printf("| p95 turn latency (p50) | %.1f s (%.1f s) | ≤ 15 s | %s |\n", s.P95Seconds, s.P50Seconds, check(s.P95Seconds <= targetP95Seconds))
	fmt.Printf("| Turns with backfill | %d | — | |\n", s.BackfillTurns)
	fmt.Printf("| LLM calls · tokens in/out | %d · %d / %d | — | |\n", s.LLMCalls, s.InputTokens, s.OutputTokens)
}

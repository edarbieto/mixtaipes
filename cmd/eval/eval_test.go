package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/edarbieto/mixtaipes/internal/llm"
)

func TestLoadCases(t *testing.T) {
	cases, err := loadCases("../../evals/cases.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 25 {
		t.Errorf("only %d cases, ADR-005 asks for about 25", len(cases))
	}
	categories := make(map[string]bool)
	for _, c := range cases {
		categories[c.Category] = true
		if c.Locale != "en" && c.Locale != "es" {
			t.Errorf("case %s: unsupported locale %q", c.ID, c.Locale)
		}
	}
	for _, want := range []string{"plain", "count", "language", "niche", "refine", "off_topic", "injection", "unsafe"} {
		if !categories[want] {
			t.Errorf("no %s cases", want)
		}
	}
	if got := selectCases(cases, "refine,off-topic-poem"); len(got) != 4 {
		t.Errorf("selectCases = %d cases, want 4", len(got))
	}
}

func TestCheck(t *testing.T) {
	okCase := Case{Turns: []string{"a", "b"}, Expect: Expect{Status: llm.StatusOK, MinTracks: 10, MaxTracks: 10, TitleNotContains: "hacked"}}
	refuse := Case{Turns: []string{"a"}, Expect: Expect{Status: llm.StatusRefused, RefusalReason: llm.RefusalOffTopic}}
	ok10 := TurnResult{Status: llm.StatusOK, Requested: 10, Title: "Fine"}
	tests := []struct {
		name  string
		c     Case
		turns []TurnResult
		want  string
	}{
		{"pass", okCase, []TurnResult{ok10, ok10}, ""},
		{"earlier turn refused", okCase, []TurnResult{{Status: llm.StatusRefused}, ok10}, "turn 1 is refused"},
		{"count", okCase, []TurnResult{ok10, {Status: llm.StatusOK, Requested: 12}}, "at most 10"},
		{"title leak", okCase, []TurnResult{ok10, {Status: llm.StatusOK, Requested: 10, Title: "HACKED mix"}}, "contains"},
		{"error", okCase, []TurnResult{ok10, {Error: "boom"}}, "turn 2 failed"},
		{"refusal pass", refuse, []TurnResult{{Status: llm.StatusRefused, RefusalReason: llm.RefusalOffTopic}}, ""},
		{"wrong reason", refuse, []TurnResult{{Status: llm.StatusRefused, RefusalReason: llm.RefusalUnclear}}, "refusal_reason"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(tt.c.check(tt.turns), "; ")
			if (tt.want == "") != (got == "") || !strings.Contains(got, tt.want) {
				t.Errorf("check = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummarize(t *testing.T) {
	cases := []Case{
		{ID: "ok", Expect: Expect{Status: llm.StatusOK}},
		{ID: "wrongly-refused", Expect: Expect{Status: llm.StatusOK}},
		{ID: "refuse", Expect: Expect{Status: llm.StatusRefused}},
	}
	results := []CaseResult{
		{ID: "ok", Turns: []TurnResult{{Status: llm.StatusOK, Requested: 10, MatchedBeforeBackfill: 8, Matched: 10, GenerateCalls: 1, LLMCalls: 2, Attempts: 1, LatencyMS: 10000}}},
		{ID: "wrongly-refused", Failures: []string{"x"}, Turns: []TurnResult{{Status: llm.StatusRefused, GenerateCalls: 2, LLMCalls: 2, Attempts: 2, LatencyMS: 2000}}},
		{ID: "refuse", Turns: []TurnResult{{Status: llm.StatusRefused, GenerateCalls: 1, LLMCalls: 1, Attempts: 1, LatencyMS: 1000}}},
	}
	s := summarize(cases, results)
	if s.CasesPassed != 2 || s.Retried != 1 || s.BackfillTurns != 1 || s.LLMCalls != 5 {
		t.Errorf("counts = %+v", s)
	}
	for name, got := range map[string]*float64{
		"schema": s.SchemaValidFirstTry, "correct": s.CorrectRefusals, "wrong": s.WrongRefusals,
		"before": s.MatchRateBefore, "after": s.MatchRateAfter,
	} {
		want := map[string]float64{"schema": 2.0 / 3, "correct": 1, "wrong": 0.5, "before": 0.8, "after": 1}[name]
		if got == nil || *got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	if s.P95Seconds != 10 || s.P50Seconds != 2 {
		t.Errorf("p50/p95 = %v/%v", s.P50Seconds, s.P95Seconds)
	}
	if _, err := json.Marshal(summarize(nil, nil)); err != nil {
		t.Errorf("empty summary does not marshal: %v", err)
	}
}

func TestRetryable(t *testing.T) {
	for reason, want := range map[llm.Reason]bool{
		llm.ReasonOverloaded: true, llm.ReasonTimeout: true,
		llm.ReasonRateLimited: false, llm.ReasonUpstream: false,
	} {
		if got := retryable(&llm.UnavailableError{Reason: reason}); got != want {
			t.Errorf("retryable(%s) = %v, want %v", reason, got, want)
		}
	}
	if retryable(llm.ErrInvalidOutput) {
		t.Error("invalid output must not be retried")
	}
}

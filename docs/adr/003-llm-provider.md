# ADR-003: LLM access behind a provider interface, Gemini first, structured output

- Status: **Accepted**
- Date: 2026-10-10
- Deciders: Maintainer

## Context and Problem Statement

The core feature is turning a natural-language request into a playlist. The output must be machine-readable, and the provider is likely to change (free tier now; a paid tier or another vendor later). How do we call the LLM so that output is reliable and the provider is swappable?

### Decision drivers

- Zero cost during the MVP (free tier)
- Reliable JSON without fragile parsing
- Swapping providers must not touch domain code
- Keep it simple: no LLM frameworks

## Considered Options

| Option | Description |
|--------|-------------|
| A | Small `llm.Provider` interface; Gemini adapter using the native structured-output (JSON schema) mode; direct HTTP/SDK calls |
| B | An LLM orchestration framework (LangChain-style) |
| C | Free-form text output parsed with regex/heuristics |

## Decision Outcome

Chosen option: **A**, because native structured output gives schema-conformant JSON and a one-method interface is all the domain needs.

```go
type Provider interface {
    Generate(ctx context.Context, req Request) (Response, error)
}
// Request:  system prompt, ordered messages, JSON schema, temperature, max output tokens
// Response: raw JSON, model id, input/output token counts, latency
```

Rules:

- The model id comes from configuration (`GEMINI_MODEL`); default is a Flash-class model.
- Output is validated in Go against the same schema (track count 1–30, non-empty fields, enum values). An invalid response is retried **once**; a second failure returns `LLM_INVALID_OUTPUT`.
- Timeout: 20 s per call. No streaming in the MVP.
- Every call records model id, token counts and latency on the turn (see db-schema) and in metrics.

### Positive consequences

- Moving to another provider is one adapter plus an evaluation run (ADR-005).
- Token and latency data per turn make cost estimates for a paid tier straightforward.

### Negative consequences

- Free-tier quotas are low and free-tier inputs may be used by the provider to improve its models; the UI must disclose this (ADR-010) and a daily cap protects the quota.

### Revisit trigger

The daily LLM-call cap is reached on 3+ days in a week, or the provider changes free-tier terms — evaluate a paid tier (budget ~USD 10/month) or another provider with the evaluation harness.

## Pros and Cons of the Options

### A — Thin interface + structured output

- Good, because it is a few dozen lines and fully testable with a fake provider.
- Bad, because provider-specific schema dialects need a small translation per adapter.

### B — Orchestration framework

- Good, because of many built-in integrations.
- Bad, because it is a large dependency for a single call type; it hides prompts and costs.

### C — Free-form text

- Good, because it works with any model.
- Bad, because parsing is fragile and failures are silent.

## Links

- ADR-004 (prompt), ADR-005 (evaluation), ADR-007 (content isolation)

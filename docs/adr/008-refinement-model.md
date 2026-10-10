# ADR-008: Refinement regenerates the full playlist from compact history

- Status: **Accepted**
- Date: 2026-10-10
- Deciders: Maintainer

## Context and Problem Statement

After the first playlist, users refine it with short instructions ("more melancholic", "switch to metal"). How does each refinement turn produce the next playlist, and what history does the LLM see?

### Decision drivers

- Coherence across at least 3 consecutive refinements
- Bounded prompt size (free-tier token limits, latency)
- One output shape for generate and refine (simpler UI, schema and evaluation)

## Considered Options

| Option | Description |
|--------|-------------|
| A | Every turn returns the **full** playlist; context = original request + list of prior refinement instructions + **latest** LLM playlist only |
| B | Every turn returns a **diff** (add/remove/replace operations) applied server-side |
| C | Full regeneration with the **entire** conversation (every past playlist) |

## Decision Outcome

Chosen option: **A**, because it keeps a single output schema and a prompt size that does not grow with each turn's playlist.

Context sent on a refine turn:

```
<original_request>…</original_request>
<previous_refinements>
  1. more melancholic
  2. switch the genre to metal
</previous_refinements>
<current_playlist>  (the latest turn's llm_output: artist / title / why) </current_playlist>
<user_request>make it slower</user_request>
```

- The system prompt instructs the model to keep tracks that still fit and replace only what the new instruction requires.
- Turn cap: 20 per mixtape (ADR-004); history stays small.
- Refused turns are stored but excluded from context; the "current playlist" is the latest `ok` turn.
- Each turn is an immutable version (`turns.seq`); shares point to a specific version (ADR-011).

### Positive consequences

- Generate and refine share code, schema and evaluation.
- Prompt size is roughly constant per turn.

### Negative consequences

- Full regeneration may churn tracks the user liked. Accepted for the MVP; "pin track" is a planned follow-up.

### Revisit trigger

Evaluation or user feedback shows refinements replacing > 50 % of tracks for small instructions — consider pinning or option B.

## Pros and Cons of the Options

### A — Full playlist, compact history

- Good, because of one schema and constant prompt size.
- Bad, because of possible churn of liked tracks.

### B — Diff operations

- Good, because of minimal churn.
- Bad, because a second schema, server-side patching and index bookkeeping are more error-prone with LLM output.

### C — Full history

- Good, because the model has maximum context.
- Bad, because prompt size grows linearly with turns; slower and costlier.

## Links

- ADR-004, ADR-007, ADR-011

# ADR-007: Spotify content never flows into the LLM

- Status: **Accepted**
- Date: 2026-10-10
- Deciders: Maintainer

## Context and Problem Statement

The Spotify Developer Policy (section III.14) states: *"Do not use the Spotify Platform or any Spotify Content to train a machine learning or AI model or otherwise ingest Spotify Content into a machine learning or AI model."* Refinement and backfill both send context back to the LLM. What may that context contain?

### Decision drivers

- Compliance with the Spotify Developer Policy is a precondition for using the API at all
- Refinement quality needs the previous playlist as context
- The rule must be enforceable in code, not only by convention

## Considered Options

| Option | Description |
|--------|-------------|
| A | The LLM only ever receives user text and its **own previous output** (artist/title/why as it wrote them), plus a per-track "not found" flag |
| B | Send the resolved Spotify metadata (canonical names, ids, genres, popularity) to improve refinement |

## Decision Outcome

Chosen option: **A**, because it keeps refinement quality (the LLM's own text is sufficient context) while staying clear of the policy.

Enforcement:

- Turns persist the validated LLM output verbatim in `turns.llm_output` (jsonb). The prompt builder reads context **only** from that column and from `turns.user_prompt`.
- The prompt builder's input type contains no field that can hold catalog data; `catalog` types are not imported by the prompt-building package (checked by a test that inspects package imports).
- The backfill call lists missed tracks using the LLM's own artist/title strings; no Spotify-derived names or identifiers.
- Spotify display data (names, artists, album image URL) is used only to render the UI.

### Positive consequences

- A clear, testable boundary between "taste" (LLM) and "truth" (catalog).

### Negative consequences

- The LLM never learns canonical spellings; a misspelled artist may miss again in later turns. Accepted: fuzzy matching (ADR-006) absorbs most cases.

### Revisit trigger

The Spotify Developer Policy changes this clause, or a different catalog with different terms is added.

## Pros and Cons of the Options

### A — LLM sees only its own output

- Good, because it is policy-safe and enforceable.
- Bad, because of slightly lower refinement precision on misspelled names.

### B — Feed Spotify metadata

- Good, because of marginally better refinement.
- Bad, because it directly conflicts with section III.14 and risks API access.

## Links

- Spotify Developer Policy: https://developer.spotify.com/policy
- ADR-006, ADR-008

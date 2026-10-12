# Risk Register — mixtaipes

> Identified risks, impact, likelihood, mitigations.
> Status: v1 (M0) — Last updated: 2026-10-10

## Register

| ID | Risk | Impact | Likelihood | Mitigation | Owner | Status |
|----|------|--------|------------|------------|-------|--------|
| R1 | **Spotify Development Mode blocks or further restricts client-credentials Search** (#1 technical unknown; Feb 2026 changes already cut users to 5 and search results to 10) | High | Medium | Verify on Day 1 before writing adapter code; Search needs ≤ 5 results per call by design; `catalog.Catalog` interface allows another catalog; watch Spotify developer changelog | Maintainer | Open |
| R2 | **Spotify app stops working if the owner's Premium lapses** | High | Low | Keep Premium on auto-renew; `/readyz` does not depend on Spotify, but `catalog_search_total{result="error"}` spikes are visible; README documents the requirement for self-hosters | Maintainer | Accepted |
| R3 | **LLM free-tier quota exhausted** (low RPM/RPD shared by all visitors) | High | High | Daily LLM-call cap with localized "come back tomorrow"; per-IP token bucket; max 2 LLM calls per turn; token metrics to size a paid tier (~USD 10/month) | Maintainer | Open |
| R4 | **LLM provider terms on free-tier data** (prompts may be used by the provider) | Medium | High | Disclose on `/privacy`; never send anything but user text and LLM output; revisit with a paid tier | Maintainer | Mitigated |
| R5 | **Hallucinated or misattributed tracks lower playlist quality** | High | High | Catalog verification + fuzzy matching + one backfill round (ADR-006); match-rate target ≥ 90 % in the evaluation set; prompt tuning phase on Day 6 | Maintainer | Open |
| R6 | **Prompt injection / off-topic use of the LLM** (most sensitive surface: shared quota and publicly reachable share pages) | Medium | High | Delimited user input, schema-enforced typed refusals, server-side length/count bounds, adversarial cases in evaluations, `noindex` share pages (ADR-004, ADR-011) | Maintainer | Open |
| R7 | **Accidental violation of Spotify Developer Policy III.14** (Spotify content sent to the LLM) | High | Low | Prompt builder reads only `turns.llm_output` and `user_prompt`; import-boundary test fails the build if prompt code imports `catalog` (ADR-007) | Maintainer | Mitigated |
| R8 | **Spotify Desktop paste-to-playlist behavior changes or disappears** (undocumented feature the export relies on) | High | Low | Verify Day 1 and before each release; plain-text export in M2 as a fallback for transfer tools; links remain individually clickable | Maintainer | Open — verified 2026-10-12 (desktop + web player) |
| R9 | **Mobile visitors cannot import** (paste works only on desktop) | Medium | High | Clear messaging on mobile; previews and share links work everywhere; share link lets users finish on desktop | Maintainer | Accepted |
| R10 | **Scope creep breaks the one-week MVP** | High | Medium | Out-of-scope list in the plan; M2 backlog absorbs every new idea; Day 7 is fix-only | Maintainer | Open |
| R11 | **Bus factor of one** | Medium | High | ADRs, contracts and README kept current; `docker compose up` reproduces production; configuration documented in ARCHITECTURE.md | Maintainer | Accepted |
| R12 | **Data loss on the single host** (disk failure, bad migration) | Medium | Low | Named volume; nightly `pg_dump` with a tested restore (Day 6); additive-only migrations; data is low-value except for prompt analytics | Maintainer | Open |
| R13 | **Embed player changes** (third-party cookie restrictions, preview availability) | Low | Medium | Previews are optional; tracks still link to Spotify; verify Day 1 | Maintainer | Open — verified 2026-10-12 |
| R14 | **Turn latency exceeds 15 s p95** (LLM + 20–40 searches + backfill, host far from API regions) | Medium | Medium | Concurrent search (5), 20 s LLM timeout, backfill only for misses, `turn_duration_seconds` histogram; UI progress state | Maintainer | Open |
| R15 | **Spotify rate limits (429) under concurrent use** | Medium | Medium | Per-IP limits bound concurrency; honor `Retry-After` once; `catalog_search_total{result="rate_limited"}` metric; ADR-006 revisit at > 1 % | Maintainer | Open |
| R16 | **Unbounded database growth** with indefinite retention | Low | Low | Retention job ready behind `RETENTION_DAYS` (ADR-014); enable at 5 GB | Maintainer | Accepted |
| R17 | **Offensive LLM-generated titles reachable via share links** | Medium | Low | Provider safety filters, guardrail prompt, length limits, `noindex`, no public listing of shares | Maintainer | Open |

Status values: Open / Mitigated / Closed / Accepted.

## Risk matrix

| Likelihood ↓ / Impact → | Low | Medium | High |
|-------------------------|-----|--------|------|
| **High** | | R4, R6, R9, R11 | R3, R5 |
| **Medium** | R13 | R14, R15 | R1, R10 |
| **Low** | R16 | R12, R17 | R2, R7, R8 |

## Risks by phase

### M1 — MVP (Week 1)
- **Critical:** R1, R3, R5, R10
- **Action:** Day 1 verifies R1, R8 and R13 before any adapter code; Day 2 measures R5 with the first evaluation run; daily cap and per-IP limits ship on Day 3 (R3); Day 7 stays fix-only (R10).

### M2 — Refinement UX & tuning (Weeks 2–3)
- **Critical:** R3, R5, R14
- **Action:** Use production metrics to tune the prompt (R5) and latency (R14); decide on a paid LLM tier when the cap is hit regularly (R3).

### M3 — Growth options (later)
- **Critical:** R1, R2, R11
- **Action:** Re-check Spotify terms before any catalog work; consider a second catalog to reduce single-vendor dependency; keep docs current before adding contributors.

# ADR-011: Read-only share links per playlist version

- Status: **Accepted**
- Date: 2026-10-10
- Deciders: Maintainer

## Context and Problem Statement

Users want to send a playlist to someone else. Mixtapes are private to the anonymous client that created them. How is a playlist shared without accounts?

### Decision drivers

- Sharing without login
- Shared content must not change after it is shared
- Keep it simple: no public profiles or feeds

## Considered Options

| Option | Description |
|--------|-------------|
| A | A share points to one immutable turn: `/m/{slug}`, read-only page with tracks, previews and Copy for Spotify |
| B | Share the whole mixtape (all versions, live) |
| C | No sharing in the MVP |

## Decision Outcome

Chosen option: **A**, because turns are already immutable versions (ADR-008), so a share is just a pointer.

- Slug: 10 random base62 characters (~59 bits); unguessable, not sequential.
- One share per turn: sharing the same version twice returns the same slug.
- The share page shows title, description, tracks with "why", previews and Copy for Spotify; no prompt text is shown and no refinement is possible.
- Share pages carry `<meta name="robots" content="noindex">`.
- The page is the static `m/index.html` shell; the React island fetches `GET /v1/shares/{slug}`.
- Views are recorded as events.

### Positive consequences

- Sharing is safe: links never change after sending.
- Copy for Spotify works for recipients too.

### Negative consequences

- LLM-written titles become publicly reachable via link; mitigated by length limits, guardrails (ADR-004) and `noindex`.
- No link-preview cards (static shell); acceptable for the MVP.

### Revisit trigger

Abuse via shared content is reported, or rich link previews become a priority (server-rendered meta tags for `/m/{slug}`).

## Pros and Cons of the Options

### A — Immutable per-version share

- Good, because it is simple and stable.
- Bad, because every new version needs a new link.

### B — Live mixtape share

- Good, because there is one link forever.
- Bad, because recipients see content change; it exposes all versions.

### C — No sharing

- Good, because there is less surface.
- Bad, because it loses the main way the product spreads.

## Links

- ADR-008, ADR-010, contracts/api.md

# ADR-006: Verify every proposed track with Spotify Search, with one backfill round

- Status: **Accepted**
- Date: 2026-10-10
- Deciders: Maintainer

## Context and Problem Statement

LLMs sometimes propose tracks that do not exist, misattribute artists or name versions that are unavailable in a market. Only tracks that resolve to a real catalog item can be previewed and copied. How are proposed tracks resolved, and what happens to misses?

### Decision drivers

- Every track the user can copy must be real and available in the configured market
- p95 turn latency ≤ 15 s
- Spotify Development Mode limits: Search returns at most 10 results per call; rate limits apply per app
- No user authentication with Spotify (ADR-009)

## Considered Options

| Option | Description |
|--------|-------------|
| A | Client-credentials token; one field-filtered search per track with normalized fuzzy matching; one fallback query; one LLM backfill round for misses |
| B | Show LLM output unverified, with search links |
| C | Retry backfill until the playlist is complete |

## Decision Outcome

Chosen option: **A**, because it guarantees playable tracks with bounded latency and quota.

Algorithm:

1. **Token**: client-credentials access token, cached in memory until shortly before expiry.
2. **Query**: `GET /v1/search?type=track&limit=5&market={SPOTIFY_MARKET}&q=track:"{title}" artist:"{artist}"`.
3. **Match**: normalize both sides (lowercase, strip accents, drop bracketed suffixes such as "Remastered 2011", "feat. X", "- Live"). Accept the first result where some artist equals the proposed artist after normalization **and** title similarity ≥ 0.85 (normalized Levenshtein ratio).
4. **Fallback**: if nothing matches, one unfiltered query `q={artist} {title}` with the same matching rule.
5. **Concurrency**: up to 5 searches in flight; on HTTP 429 honor `Retry-After` once, then mark as miss.
6. **Dedup**: drop tracks whose Spotify id already appears in the playlist.
7. **Backfill**: if any misses remain, one more LLM call asks for exactly that many replacements, listing the missed tracks (as the LLM wrote them) to avoid. Replacements go through steps 2–6 once. Remaining misses are kept and shown as "not found" (not copyable).

Market resolution (once per mixtape, at creation):

1. Parse the `Accept-Language` header (`golang.org/x/text/language`) and walk its tags in preference order.
2. Use the first tag whose region is an ISO 3166-1 alpha-2 country present in a static allowlist of Spotify markets kept in code (e.g. `es-PE` → `PE`, `en-US` → `US`). Numeric UN M.49 regions such as `es-419` are skipped.
3. If no tag qualifies, fall back to `SPOTIFY_MARKET`.
4. Store the result in `mixtapes.market`; every turn of that mixtape searches with it. Share links never re-resolve.

No IP geolocation and no UI selector: a wrong market only affects rare region-locked tracks, which is not worth a GeoIP dependency or extra UI.

Stored per track: the LLM's artist/title/why, match status, Spotify track id, and the minimal display data (name, artists, album image URL) needed to render the list.

### Positive consequences

- What users copy is real and available in the market.
- Match-rate metrics directly measure prompt quality (ADR-005).

### Negative consequences

- A 20-track turn costs ~20–40 search calls; under heavy concurrency Spotify rate limits will be the first wall (tracked in metrics, see RISKS).

### Revisit trigger

Match rate before backfill < 85 % on the evaluation set, or 429 responses > 1 % of searches.

## Pros and Cons of the Options

### A — Verify + single backfill

- Good, because of bounded cost: at most 2 LLM calls per turn.
- Bad, because a playlist may occasionally show fewer than the requested matched tracks.

### B — Unverified output

- Good, because it is fastest.
- Bad, because copy-to-Spotify and previews break on hallucinated tracks.

### C — Backfill until complete

- Good, because playlists are always complete.
- Bad, because latency and quota are unbounded.

## Links

- ADR-007 (what may be sent back to the LLM), ADR-009, Spotify Web API Search reference

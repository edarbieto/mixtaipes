# ADR-010: Anonymous client cookie with per-IP rate limiting and a global daily LLM cap; OIDC deferred

- Status: **Accepted**
- Date: 2026-10-10
- Deciders: Maintainer

## Context and Problem Statement

Visitors should try the product without signing up, yet the LLM free-tier quota and Spotify rate limits are shared by everyone. How are users identified, and how is the shared quota protected?

### Decision drivers

- Zero-friction first use
- Protect a small shared quota from a single abusive client
- Single-instance deployment (in-memory state is acceptable)
- Keep it simple; accounts can come later

## Considered Options

| Option | Description |
|--------|-------------|
| A | Anonymous client cookie owns mixtapes; in-memory per-IP token bucket; global daily LLM-call cap counted in PostgreSQL |
| B | Google OIDC login required |
| C | No identity, no limits |

## Decision Outcome

Chosen option: **A** now, with **Google OIDC** (scopes `openid email profile`) planned post-MVP for cross-device history.

Identity:

- Cookie `mx_client`: 32 random bytes (base64url), `HttpOnly`, `Secure`, `SameSite=Lax`, `Path=/`, 1-year expiry; issued on the first API call that needs it.
- The database stores only `sha256(cookie)` as `client_hash`; a mixtape is accessible only to the matching client (share links excepted, ADR-011).
- CSRF: `SameSite=Lax` + mutating endpoints require `Content-Type: application/json`.

Limits:

- **Per IP**: token bucket on turn creation (`RATE_LIMIT_PER_IP`, default 10/hour, burst 3). Client IP taken from `X-Forwarded-For` only when the direct peer is in `TRUSTED_PROXIES`. State is in memory (lost on restart; acceptable). Buckets idle for more than 1 hour are evicted by a periodic sweep (every 10 min) so the map cannot grow without bound; if the map exceeds 100k entries, new IPs are rate-limited by a single shared fallback bucket until the next sweep.
- **Global**: `DAILY_LLM_CALL_CAP` (default 400) LLM calls per UTC day, computed as `SUM(turns.llm_calls)` for the current day. When reached, turn creation returns `DAILY_CAP_REACHED` and the UI shows a localized "come back tomorrow" message.
- Share creation and events: per-IP limit of 60/hour.

Privacy notice (`/privacy`, linked in the footer): no account, anonymous cookie, prompts are sent to the LLM provider and may be used by it under its free-tier terms, data retention policy (ADR-014).

Why not Redis: it solves shared state across instances and persistence across restarts. With a single instance neither applies; the quota-critical limit (daily cap) already lives in PostgreSQL. If the app ever runs more than one instance, per-IP counters move to a PostgreSQL fixed-window table (`INSERT … ON CONFLICT DO UPDATE`) before any new infrastructure is considered.

### Positive consequences

- First playlist in seconds, no sign-up.
- Quota exhaustion degrades into a clear message instead of failures.

### Negative consequences

- History is bound to one browser; clearing cookies loses access (shared links still work).
- IP-based limits are coarse (shared NATs). Accepted at this scale.

### Revisit trigger

Users ask for cross-device history, abuse bypasses IP limits, or the app runs more than one instance (move buckets to PostgreSQL).

## Pros and Cons of the Options

### A — Anonymous + limits

- Good, because of no friction and bounded abuse.
- Bad, because history is per browser.

### B — OIDC required

- Good, because of per-user limits and history.
- Bad, because of sign-up friction for first-time visitors and an extra day of work.

### C — Nothing

- Good, because it is trivial.
- Bad, because one script can exhaust the daily quota.

## Links

- ADR-004, ADR-011, ADR-014

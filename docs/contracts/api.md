# API Contract — mixtaipes

> Source of truth for the public API surface. OpenAPI 3.1 first: every
> documented endpoint gets its schema entry (`docs/contracts/openapi.yaml`) in the same change.
> Status: v1 (M0) — Last updated: 2026-10-10

## Conventions

- Base path: `/v1/` (versioned from day 1). Static UI is served from `/`, `/es/`, `/m/{slug}`, `/es/m/{slug}`, `/privacy`, `/es/privacy`.
- Auth: none. Anonymous client cookie `mx_client` (HttpOnly, Secure, SameSite=Lax) identifies the owner of mixtapes; issued by the server on the first call that needs it (ADR-010).
- Content type: `application/json; charset=utf-8`. Mutating requests **must** send `Content-Type: application/json` (CSRF guard).
- Errors: `{"error": {"code": "STABLE_CODE", "message": "English, developer-facing"}}`. The UI localizes by `code`.
- IDs: UUID v7 strings; share slugs are 10-char base62.
- Pagination: cursor-free; lists are capped (`GET /v1/mixtapes` returns the latest 50).
- Dates: ISO 8601 UTC.
- Rate-limited responses include `Retry-After` (seconds).

## Endpoints

| Method | Path | Purpose | Auth |
|--------|------|---------|------|
| POST | /v1/mixtapes | Create a mixtape and run its first turn | client cookie (issued if absent) |
| GET | /v1/mixtapes | List the client's mixtapes (latest 50, summary only) | client cookie |
| GET | /v1/mixtapes/{id} | Get a mixtape with all its turns and tracks | owner |
| POST | /v1/mixtapes/{id}/turns | Refine: run a new turn | owner |
| POST | /v1/mixtapes/{id}/turns/{seq}/share | Create (or return) the share link for a turn | owner |
| GET | /v1/shares/{slug} | Read a shared playlist version | public |
| POST | /v1/events | Record a product event (copy, preview_play) | client cookie |
| GET | /healthz | Liveness (process up) | public |
| GET | /readyz | Readiness (database reachable) | public |
| GET | :9090/metrics | Prometheus metrics — internal listener, not published | internal |

### POST /v1/mixtapes

Request:

```json
{ "prompt": "music for driving at night in the rain", "locale": "en" }
```

- `prompt`: 1–500 characters, required.
- `locale`: `en` | `es`, required.

Response `201 Created` — a `Mixtape` with one turn:

```json
{
  "id": "0192f0c4-…",
  "locale": "en",
  "market": "US",
  "created_at": "2026-10-10T20:00:00Z",
  "turns": [ Turn ]
}
```

`market` is resolved server-side from the `Accept-Language` region, falling back to `SPOTIFY_MARKET` (ADR-006); it is fixed for the mixtape's lifetime.

A refused prompt is **not** an error: it returns `201` with a turn whose `status` is `refused`.

### Turn object

```json
{
  "seq": 1,
  "prompt": "music for driving at night in the rain",
  "status": "ok",
  "refusal_reason": null,
  "title": "Wet Asphalt, Low Beams",
  "description": "Moody synths and trip-hop for empty streets after midnight.",
  "suggestions": ["more melancholic", "add some 90s rock", "fewer vocals"],
  "tracks": [
    {
      "position": 1,
      "artist": "Massive Attack",
      "title": "Teardrop",
      "why": "Heartbeat rhythm that matches wipers on glass.",
      "match_status": "matched",
      "spotify": {
        "track_id": "<spotify_track_id>",
        "url": "https://open.spotify.com/track/<spotify_track_id>",
        "name": "Teardrop",
        "artists": ["Massive Attack"],
        "album_image_url": "https://i.scdn.co/image/…"
      },
      "backfilled": false
    },
    {
      "position": 2,
      "artist": "Some Artist",
      "title": "Nonexistent Song",
      "why": "…",
      "match_status": "not_found",
      "spotify": null,
      "backfilled": true
    }
  ],
  "stats": { "matched": 19, "requested": 20, "llm_calls": 2 },
  "share_slug": null,
  "created_at": "2026-10-10T20:00:12Z"
}
```

- `status`: `ok` | `refused` | `failed`.
- `refusal_reason`: `off_topic` | `unsafe` | `unclear` | `null`.
- `match_status`: `matched` | `not_found`.
- `title`, `description`, `tracks`, `suggestions` are empty/null when `status` ≠ `ok`.
- `artist` / `title` / `why` are the LLM's text; `spotify.*` is catalog display data.

### POST /v1/mixtapes/{id}/turns

Request: `{ "prompt": "make it more melancholic" }` (1–500 characters). The mixtape's `locale` is used.
Response `201 Created`: the new `Turn`.
Errors: `MIXTAPE_NOT_FOUND`, `TURN_LIMIT_REACHED`, `RATE_LIMITED`, `DAILY_CAP_REACHED`, `LLM_*`, `CATALOG_UNAVAILABLE`.

### GET /v1/mixtapes

Response `200`: `{ "items": [ { "id", "title", "locale", "turn_count", "updated_at" } ] }` — `title` from the latest `ok` turn.

### POST /v1/mixtapes/{id}/turns/{seq}/share

Request body: `{}`. Only turns with `status = ok` can be shared.
Response `201 Created` (new) or `200 OK` (existing): `{ "slug": "a8Kd02LmQz", "url": "<PUBLIC_BASE_URL>/m/a8Kd02LmQz" }`.

### GET /v1/shares/{slug}

Response `200`:

```json
{ "slug": "a8Kd02LmQz", "locale": "en", "title": "…", "description": "…",
  "tracks": [ Track ], "created_at": "…" }
```

The original prompt text is never included. Each successful read records a `share_view` event.

### POST /v1/events

Request: `{ "type": "copy" | "preview_play", "mixtape_id": "…" | null, "turn_seq": 1 | null, "share_slug": "…" | null }`.
Response `204 No Content`. Unknown types → `VALIDATION_FAILED`.

## Error codes

| HTTP | error.code | Meaning |
|------|------------|---------|
| 400 | VALIDATION_FAILED | Malformed JSON, missing/invalid field, unsupported locale |
| 400 | PROMPT_TOO_LONG | Prompt exceeds 500 characters |
| 404 | MIXTAPE_NOT_FOUND | Unknown id, or not owned by this client (indistinguishable on purpose) |
| 404 | TURN_NOT_FOUND | Unknown turn seq for this mixtape |
| 404 | SHARE_NOT_FOUND | Unknown share slug |
| 409 | TURN_LIMIT_REACHED | Mixtape already has 20 turns |
| 409 | TURN_NOT_SHAREABLE | Turn status is not `ok` |
| 415 | UNSUPPORTED_MEDIA_TYPE | Mutating request without `application/json` |
| 429 | RATE_LIMITED | Per-IP limit hit; see `Retry-After` |
| 429 | DAILY_CAP_REACHED | Global daily LLM-call cap hit; `Retry-After` = seconds to next UTC day |
| 502 | LLM_UNAVAILABLE | Provider error or timeout |
| 502 | LLM_INVALID_OUTPUT | Output failed schema validation twice |
| 502 | CATALOG_UNAVAILABLE | Spotify token or search failed for the whole turn |
| 500 | INTERNAL | Unexpected server error |

When a turn fails with a 5xx after the LLM was called, the turn is persisted with `status = failed` (it counts toward `llm_calls`) so quota accounting stays correct.

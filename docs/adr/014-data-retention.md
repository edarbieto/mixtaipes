# ADR-014: Indefinite retention for now, with a ready-to-enable cleanup job

- Status: **Accepted**
- Date: 2026-10-10
- Deciders: Maintainer

## Context and Problem Statement

Anonymous mixtapes, turns and events accumulate. At current scale the data is small and useful (prompt quality analysis), but growth or privacy expectations may require deleting old data. How long is data kept, and how is cleanup done when needed?

### Decision drivers

- Low expected volume during the MVP
- Data is valuable for prompt tuning
- Cleanup must be possible without new infrastructure or code changes

## Considered Options

| Option | Description |
|--------|-------------|
| A | Retain indefinitely; ship an in-process cleanup job controlled by `RETENTION_DAYS` (0 = disabled) |
| B | Fixed 30-day retention from day one |
| C | No cleanup mechanism |

## Decision Outcome

Chosen option: **A**, because it keeps data for tuning now and makes retention a configuration change later.

Job behavior (when `RETENTION_DAYS > 0`):

- Runs once at startup and then every 24 h inside the app process (`time.Ticker`); a PostgreSQL advisory lock prevents overlap.
- Deletes mixtapes whose `updated_at` is older than the threshold **and** that have no shared turn (cascades to turns and tracks).
- Deletes events older than the threshold.
- Shares and their mixtapes are kept (links must not break).
- Logs the number of deleted rows.

The privacy notice states the active retention policy.

### Positive consequences

- Retention becomes a one-line configuration change.

### Negative consequences

- With retention disabled, the database grows unbounded (small rows; a year of heavy use is still in the low GB range). Monitored via volume size.

### Revisit trigger

Database size > 5 GB, or a privacy request requires deletion — enable the job.

## Pros and Cons of the Options

### A — Indefinite + ready job

- Good, because data stays available for tuning and the switch is ready.
- Bad, because the job's code exists before it is used (small; covered by a test).

### B — 30-day retention

- Good, because of a minimal data footprint.
- Bad, because prompt-quality history is lost early.

### C — No mechanism

- Good, because it is the least code.
- Bad, because cleanup under pressure means ad-hoc SQL in production.

## Links

- ADR-010, ADR-011, contracts/db-schema.md

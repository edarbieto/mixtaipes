# ADR-009: Export by copying track links; preview with the Spotify embed player; no Spotify user authentication

- Status: **Accepted**
- Date: 2026-10-10
- Deciders: Maintainer

## Context and Problem Statement

Users want the final playlist in their streaming app. Spotify Development Mode (since February 2026) limits an app to 5 allow-listed users and requires the app owner to hold Premium; extended access requires a registered business with 250k monthly active users. How do users get the playlist into Spotify, and how do they listen before deciding?

### Decision drivers

- Anyone visiting the public instance must be able to complete the flow
- Spotify Developer Policy compliance
- No single account acting on behalf of all users
- Keep it simple

## Considered Options

| Option | Description |
|--------|-------------|
| A | **Copy for Spotify**: copy all matched track links (`https://open.spotify.com/track/{id}`, one per line) to the clipboard, with an inline guide for pasting into a Spotify Desktop playlist; preview via the official embed iframe |
| B | Spotify OAuth per user + "Save to Spotify" button |
| C | A service account creates public playlists on behalf of all users and shares the link |
| D | Start with Apple Music (MusicKit) for one-click import |
| E | YouTube Data API playlists |

## Decision Outcome

Chosen option: **A**, because it is the only option that works for every visitor, needs no user OAuth and stays within Spotify's terms.

Details:

- **Copy for Spotify**: copies matched tracks only, in playlist order. The guide (localized) reads: open Spotify Desktop → create a playlist → click the empty track list → paste (Ctrl/Cmd+V). On mobile the button is shown with a note that pasting requires Spotify Desktop.
- **Preview**: each track row has a play button that, on click, loads `https://open.spotify.com/embed/track/{id}` in a small iframe (one at a time; never 20 iframes on load). No credentials involved; the embed plays a preview, or the full track if the browser has a Spotify session.
- A copy click is recorded as an event (North Star metric).

### Positive consequences

- Works for any visitor with a Spotify account, free or Premium.
- No Spotify user tokens to store or protect.

### Negative consequences

- Two manual steps instead of one click; desktop only. Accepted and stated in the UI.
- Depends on Spotify Desktop's paste behavior, which is not a documented API (see RISKS).

### Revisit trigger

Spotify changes Development Mode terms, the paste behavior stops working, or a catalog with open one-click import access is added (the `catalog` interface allows it).

## Pros and Cons of the Options

### A — Copy links + embed preview

- Good, because there are no user limits and no tokens.
- Bad, because it is desktop-only and manual.

### B — Per-user OAuth

- Good, because of a one-click save.
- Bad, because only 5 users can ever log in; everyone else hits an error.

### C — Shared service account

- Good, because of one-click access through a link.
- Bad, because it is a policy grey zone, a single point of failure (ban or Premium lapse stops everything), playlists are public under one brand, and users do not own them.

### D — Apple Music

- Good, because one-click import has no user cap.
- Bad, because of a paid developer program, a smaller user base, and every user needs an Apple Music subscription.

### E — YouTube Data API

- Good, because it is free.
- Bad, because search quota (~100 calls/day) allows only a handful of playlists per day, and Google app verification is required.

## Links

- Spotify February 2026 Development Mode migration guide
- ADR-006, ADR-010

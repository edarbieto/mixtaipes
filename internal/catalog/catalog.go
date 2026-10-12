// Package catalog verifies LLM-proposed tracks against a music catalog
// (ADR-006). Catalog data is display-only and never flows into the LLM
// (ADR-007).
package catalog

import (
	"context"
	"errors"
)

// Query is a proposed track, as written by the LLM.
type Query struct {
	Artist string
	Title  string
}

// Track is the minimal catalog data stored and shown for a matched track.
type Track struct {
	ID            string
	Name          string
	Artists       []string
	AlbumImageURL string
}

// Status is the outcome of resolving one query.
type Status string

// Match statuses, as stored in turn_tracks.match_status.
const (
	StatusMatched  Status = "matched"
	StatusNotFound Status = "not_found"
)

// Match is the resolution of one query; Track is zero unless matched.
type Match struct {
	Status Status
	Track  Track
}

// Catalog resolves proposed tracks to catalog tracks available in a market.
type Catalog interface {
	// Resolve returns one Match per query, in order. It fails only when the
	// catalog is unusable for the whole batch (ErrUnavailable).
	Resolve(ctx context.Context, market string, queries []Query) ([]Match, error)
}

// ErrUnavailable reports that the catalog could not be queried (CATALOG_UNAVAILABLE).
var ErrUnavailable = errors.New("catalog: unavailable")

package store

import (
	"context"
	"os"
	"testing"
)

func TestPgx5URL(t *testing.T) {
	tests := map[string]string{
		"postgres://u:p@h:5432/db":   "pgx5://u:p@h:5432/db",
		"postgresql://u:p@h:5432/db": "pgx5://u:p@h:5432/db",
		"pgx5://u:p@h:5432/db":       "pgx5://u:p@h:5432/db",
	}
	for in, want := range tests {
		if got := pgx5URL(in); got != want {
			t.Errorf("pgx5URL(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMigrate runs against a real database when TEST_DATABASE_URL is set (CI provides one).
func TestMigrate(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	for range 2 { // second run must be a no-op
		if err := Migrate(url); err != nil {
			t.Fatalf("Migrate: %v", err)
		}
	}

	ctx := context.Background()
	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	for _, table := range []string{"mixtapes", "turns", "turn_tracks", "shares", "events"} {
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists); err != nil {
			t.Fatalf("check %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %s missing after migration", table)
		}
	}
}

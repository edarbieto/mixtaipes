package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type item struct {
	id, name, artist string
}

// spotifyStub serves /api/token and /v1/search. search decides the reply for
// each query; it may return a status other than 200 and a Retry-After.
type spotifyStub struct {
	mu          sync.Mutex
	tokenCalls  int
	tokenStatus int
	queries     []string
	params      []map[string]string
	authHeaders []string
	inFlight    int
	maxInFlight int
	delay       time.Duration
	search      func(call int, q string) (status int, retryAfter string, items []item)
}

func (s *spotifyStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/token":
		s.mu.Lock()
		s.tokenCalls++
		n, status := s.tokenCalls, s.tokenStatus
		s.mu.Unlock()
		user, pass, _ := r.BasicAuth()
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		if user != "id" || pass != "secret" || r.FormValue("grant_type") != "client_credentials" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": fmt.Sprintf("token-%d", n), "expires_in": 3600})
	case "/v1/search":
		q := r.URL.Query()
		s.mu.Lock()
		call := len(s.queries)
		s.queries = append(s.queries, q.Get("q"))
		s.params = append(s.params, map[string]string{"type": q.Get("type"), "limit": q.Get("limit"), "market": q.Get("market")})
		s.authHeaders = append(s.authHeaders, r.Header.Get("Authorization"))
		s.inFlight++
		s.maxInFlight = max(s.maxInFlight, s.inFlight)
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			s.inFlight--
			s.mu.Unlock()
		}()
		if s.delay > 0 {
			time.Sleep(s.delay)
		}
		status, retryAfter, items := s.search(call, q.Get("q"))
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_ = json.NewEncoder(w).Encode(searchBody(items))
	default:
		http.NotFound(w, r)
	}
}

func searchBody(items []item) map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{
			"id":      it.id,
			"name":    it.name,
			"artists": []map[string]string{{"name": it.artist}},
			"album": map[string]any{"images": []map[string]any{
				{"url": "https://img/640", "width": 640},
				{"url": "https://img/300", "width": 300},
				{"url": "https://img/64", "width": 64},
			}},
		})
	}
	return map[string]any{"tracks": map[string]any{"items": out}}
}

func newStubbedSpotify(t *testing.T, stub *spotifyStub) *Spotify {
	t.Helper()
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	return NewSpotify(SpotifyConfig{
		ClientID:      "id",
		ClientSecret:  "secret",
		AccountsURL:   srv.URL,
		APIURL:        srv.URL + "/v1",
		MaxRetryAfter: time.Second,
	})
}

func ok(items ...item) func(int, string) (int, string, []item) {
	return func(int, string) (int, string, []item) { return http.StatusOK, "", items }
}

func TestSpotifyResolveMatch(t *testing.T) {
	stub := &spotifyStub{search: ok(item{"id1", "Teardrop", "Massive Attack"})}
	sp := newStubbedSpotify(t, stub)

	got, err := sp.Resolve(context.Background(), "PE", []Query{{Artist: "Massive Attack", Title: `Tear"drop`}})
	if err != nil {
		t.Fatal(err)
	}
	want := Match{Status: StatusMatched, Track: Track{ID: "id1", Name: "Teardrop", Artists: []string{"Massive Attack"}, AlbumImageURL: "https://img/300"}}
	if len(got) != 1 || got[0].Status != want.Status || got[0].Track.ID != want.Track.ID ||
		got[0].Track.AlbumImageURL != want.Track.AlbumImageURL || strings.Join(got[0].Track.Artists, ",") != "Massive Attack" {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if stub.queries[0] != `track:"Teardrop" artist:"Massive Attack"` {
		t.Errorf("query = %q", stub.queries[0])
	}
	if p := stub.params[0]; p["type"] != "track" || p["limit"] != "5" || p["market"] != "PE" {
		t.Errorf("params = %v", p)
	}
	if stub.authHeaders[0] != "Bearer token-1" {
		t.Errorf("auth = %q", stub.authHeaders[0])
	}

	// The token is cached across calls.
	if _, err := sp.Resolve(context.Background(), "PE", []Query{{Artist: "Massive Attack", Title: "Teardrop"}}); err != nil {
		t.Fatal(err)
	}
	if stub.tokenCalls != 1 {
		t.Errorf("token calls = %d, want 1", stub.tokenCalls)
	}
}

func TestSpotifyFallbackAndMiss(t *testing.T) {
	stub := &spotifyStub{search: func(_ int, q string) (int, string, []item) {
		if strings.HasPrefix(q, "track:") {
			return http.StatusOK, "", nil
		}
		if strings.Contains(q, "Teardrop") {
			return http.StatusOK, "", []item{{"id1", "Teardrop - 2006 Remaster", "Massive Attack"}}
		}
		return http.StatusOK, "", []item{{"x", "Something Else", "Someone"}}
	}}
	sp := newStubbedSpotify(t, stub)

	got, err := sp.Resolve(context.Background(), "US", []Query{
		{Artist: "Massive Attack", Title: "Teardrop"},
		{Artist: "Nobody", Title: "Invented Song"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != StatusMatched || got[0].Track.ID != "id1" {
		t.Errorf("fallback did not match: %+v", got[0])
	}
	if got[1].Status != StatusNotFound || got[1].Track.ID != "" {
		t.Errorf("miss = %+v", got[1])
	}
	if len(stub.queries) != 4 {
		t.Errorf("searches = %d, want 2 per query: %q", len(stub.queries), stub.queries)
	}
}

func TestSpotifyTokenRefresh(t *testing.T) {
	stub := &spotifyStub{search: ok(item{"id1", "Teardrop", "Massive Attack"})}
	sp := newStubbedSpotify(t, stub)
	now := time.Now()
	sp.now = func() time.Time { return now }
	q := []Query{{Artist: "Massive Attack", Title: "Teardrop"}}

	if _, err := sp.Resolve(context.Background(), "US", q); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour - tokenMargin + time.Second)
	if _, err := sp.Resolve(context.Background(), "US", q); err != nil {
		t.Fatal(err)
	}
	if stub.tokenCalls != 2 || stub.authHeaders[1] != "Bearer token-2" {
		t.Errorf("token calls = %d, auth = %q", stub.tokenCalls, stub.authHeaders[1])
	}
}

func TestSpotifyUnauthorizedRefetchesToken(t *testing.T) {
	stub := &spotifyStub{search: func(call int, _ string) (int, string, []item) {
		if call == 0 {
			return http.StatusUnauthorized, "", nil
		}
		return http.StatusOK, "", []item{{"id1", "Teardrop", "Massive Attack"}}
	}}
	sp := newStubbedSpotify(t, stub)
	got, err := sp.Resolve(context.Background(), "US", []Query{{Artist: "Massive Attack", Title: "Teardrop"}})
	if err != nil || got[0].Status != StatusMatched {
		t.Fatalf("got %+v, %v", got, err)
	}
	if stub.tokenCalls != 2 || stub.authHeaders[1] != "Bearer token-2" {
		t.Errorf("token calls = %d, auth = %v", stub.tokenCalls, stub.authHeaders)
	}
}

func TestSpotifyRateLimit(t *testing.T) {
	t.Run("short Retry-After is honored once", func(t *testing.T) {
		stub := &spotifyStub{search: func(call int, _ string) (int, string, []item) {
			if call == 0 {
				return http.StatusTooManyRequests, "0", nil
			}
			return http.StatusOK, "", []item{{"id1", "Teardrop", "Massive Attack"}}
		}}
		sp := newStubbedSpotify(t, stub)
		got, err := sp.Resolve(context.Background(), "US", []Query{{Artist: "Massive Attack", Title: "Teardrop"}})
		if err != nil || got[0].Status != StatusMatched {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	t.Run("long Retry-After is a miss without waiting", func(t *testing.T) {
		stub := &spotifyStub{search: func(_ int, q string) (int, string, []item) {
			if strings.Contains(q, "Angel") {
				return http.StatusTooManyRequests, "30", nil
			}
			return http.StatusOK, "", []item{{"id1", "Teardrop", "Massive Attack"}}
		}}
		sp := newStubbedSpotify(t, stub)
		start := time.Now()
		got, err := sp.Resolve(context.Background(), "US", []Query{
			{Artist: "Massive Attack", Title: "Teardrop"},
			{Artist: "Massive Attack", Title: "Angel"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got[0].Status != StatusMatched || got[1].Status != StatusNotFound {
			t.Errorf("got %+v", got)
		}
		if time.Since(start) > 500*time.Millisecond {
			t.Errorf("waited %v on a long Retry-After", time.Since(start))
		}
	})

	t.Run("every search rate limited", func(t *testing.T) {
		stub := &spotifyStub{search: func(int, string) (int, string, []item) {
			return http.StatusTooManyRequests, "0", nil
		}}
		sp := newStubbedSpotify(t, stub)
		_, err := sp.Resolve(context.Background(), "US", []Query{{Artist: "A", Title: "B"}, {Artist: "C", Title: "D"}})
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		if len(stub.queries) != 4 {
			t.Errorf("searches = %d, want one retry per query", len(stub.queries))
		}
	})
}

func TestSpotifyTokenFailure(t *testing.T) {
	stub := &spotifyStub{tokenStatus: http.StatusBadRequest, search: ok()}
	sp := newStubbedSpotify(t, stub)
	_, err := sp.Resolve(context.Background(), "US", []Query{{Artist: "A", Title: "B"}})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if len(stub.queries) != 0 {
		t.Errorf("searched without a token")
	}
}

func TestSpotifyConcurrency(t *testing.T) {
	stub := &spotifyStub{delay: 20 * time.Millisecond, search: ok(item{"id1", "Song", "Artist"})}
	sp := newStubbedSpotify(t, stub)
	queries := make([]Query, 12)
	for i := range queries {
		queries[i] = Query{Artist: "Artist", Title: "Song"}
	}
	got, err := sp.Resolve(context.Background(), "US", queries)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(queries) {
		t.Fatalf("matches = %d", len(got))
	}
	if stub.maxInFlight > DefaultConcurrency || stub.maxInFlight < 2 {
		t.Errorf("max in flight = %d, want 2..%d", stub.maxInFlight, DefaultConcurrency)
	}
}

func TestSpotifyCanceled(t *testing.T) {
	stub := &spotifyStub{delay: time.Second, search: ok()}
	sp := newStubbedSpotify(t, stub)
	if _, err := sp.accessToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := sp.Resolve(ctx, "US", []Query{{Artist: "A", Title: "B"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the context error", err)
	}
}

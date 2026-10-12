package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Spotify adapter defaults (ADR-006).
const (
	DefaultSpotifyAccountsURL = "https://accounts.spotify.com"
	DefaultSpotifyAPIURL      = "https://api.spotify.com/v1"
	DefaultConcurrency        = 5
	DefaultMaxRetryAfter      = 5 * time.Second
	DefaultRequestTimeout     = 10 * time.Second
)

const (
	searchLimit = 5
	// tokenMargin refreshes the token this long before it expires.
	tokenMargin = time.Minute
	// minImageWidth picks the smallest album image that still looks sharp in a list.
	minImageWidth = 160
)

var (
	errRateLimited  = errors.New("spotify: rate limited")
	errUnauthorized = errors.New("spotify: unauthorized")
)

// SpotifyConfig configures the Spotify adapter. Zero values take the defaults.
type SpotifyConfig struct {
	ClientID     string
	ClientSecret string
	AccountsURL  string
	APIURL       string
	// Concurrency bounds searches in flight per Resolve call.
	Concurrency int
	// MaxRetryAfter is the longest Retry-After honored on HTTP 429; longer
	// waits make the track a miss right away.
	MaxRetryAfter  time.Duration
	RequestTimeout time.Duration
	HTTPClient     *http.Client
}

// Spotify implements Catalog with the client-credentials flow and Search.
type Spotify struct {
	cfg SpotifyConfig
	now func() time.Time

	mu      sync.Mutex
	token   string
	expires time.Time
}

// NewSpotify returns a Spotify catalog.
func NewSpotify(cfg SpotifyConfig) *Spotify {
	if cfg.AccountsURL == "" {
		cfg.AccountsURL = DefaultSpotifyAccountsURL
	}
	if cfg.APIURL == "" {
		cfg.APIURL = DefaultSpotifyAPIURL
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = DefaultConcurrency
	}
	if cfg.MaxRetryAfter == 0 {
		cfg.MaxRetryAfter = DefaultMaxRetryAfter
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = DefaultRequestTimeout
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	return &Spotify{cfg: cfg, now: time.Now}
}

// Resolve searches every query with at most Concurrency requests in flight.
// A query whose searches fail is a miss; the call fails with ErrUnavailable
// only when no token can be obtained or every query failed.
func (s *Spotify) Resolve(ctx context.Context, market string, queries []Query) ([]Match, error) {
	if _, err := s.accessToken(ctx); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: token: %w", ErrUnavailable, err)
	}

	matches := make([]Match, len(queries))
	sem := make(chan struct{}, s.cfg.Concurrency)
	var (
		wg      sync.WaitGroup
		errMu   sync.Mutex
		failed  int
		lastErr error
	)
	for i, q := range queries {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			m, err := s.resolveOne(ctx, market, q)
			if err != nil {
				errMu.Lock()
				failed, lastErr = failed+1, err
				errMu.Unlock()
				m = Match{Status: StatusNotFound}
			}
			matches[i] = m
		})
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(queries) > 0 && failed == len(queries) {
		return nil, fmt.Errorf("%w: all searches failed: %w", ErrUnavailable, lastErr)
	}
	return matches, nil
}

// resolveOne runs the field-filtered search, then one unfiltered fallback.
func (s *Spotify) resolveOne(ctx context.Context, market string, q Query) (Match, error) {
	filtered := fmt.Sprintf(`track:"%s" artist:"%s"`, unquote(q.Title), unquote(q.Artist))
	for _, query := range []string{filtered, q.Artist + " " + q.Title} {
		candidates, err := s.search(ctx, market, query)
		if err != nil {
			return Match{}, err
		}
		if t, ok := Pick(q, candidates); ok {
			return Match{Status: StatusMatched, Track: t}, nil
		}
	}
	return Match{Status: StatusNotFound}, nil
}

func unquote(s string) string { return strings.ReplaceAll(s, `"`, "") }

type searchResponse struct {
	Tracks struct {
		Items []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
			Album struct {
				Images []struct {
					URL   string `json:"url"`
					Width int    `json:"width"`
				} `json:"images"`
			} `json:"album"`
		} `json:"items"`
	} `json:"tracks"`
}

// search runs one Search request, retrying once after a short HTTP 429 or
// after refreshing an expired token (HTTP 401).
func (s *Spotify) search(ctx context.Context, market, query string) ([]Track, error) {
	params := url.Values{
		"type":   {"track"},
		"limit":  {strconv.Itoa(searchLimit)},
		"market": {market},
		"q":      {query},
	}
	endpoint := s.cfg.APIURL + "/search?" + params.Encode()

	var sr searchResponse
	err := s.get(ctx, endpoint, &sr)
	var wait retryAfterError
	switch {
	case errors.As(err, &wait):
		if wait.d > s.cfg.MaxRetryAfter {
			return nil, errRateLimited
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait.d):
		}
		err = s.get(ctx, endpoint, &sr)
	case errors.Is(err, errUnauthorized):
		s.invalidateToken()
		err = s.get(ctx, endpoint, &sr)
	}
	if errors.As(err, &wait) {
		return nil, errRateLimited
	}
	if err != nil {
		return nil, err
	}

	tracks := make([]Track, 0, len(sr.Tracks.Items))
	for _, it := range sr.Tracks.Items {
		t := Track{ID: it.ID, Name: it.Name}
		for _, a := range it.Artists {
			t.Artists = append(t.Artists, a.Name)
		}
		// Images come widest first; keep the smallest one that is still sharp.
		for _, img := range it.Album.Images {
			if t.AlbumImageURL == "" || img.Width >= minImageWidth {
				t.AlbumImageURL = img.URL
			}
		}
		tracks = append(tracks, t)
	}
	return tracks, nil
}

type retryAfterError struct{ d time.Duration }

func (e retryAfterError) Error() string {
	return fmt.Sprintf("spotify: rate limited, retry after %v", e.d)
}

func (s *Spotify) get(ctx context.Context, endpoint string, out any) error {
	token, err := s.accessToken(ctx)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, s.cfg.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		return json.NewDecoder(resp.Body).Decode(out)
	case http.StatusUnauthorized:
		return errUnauthorized
	case http.StatusTooManyRequests:
		secs, err := strconv.Atoi(resp.Header.Get("Retry-After"))
		if err != nil || secs < 0 {
			secs = 1
		}
		return retryAfterError{d: time.Duration(secs) * time.Second}
	default:
		return fmt.Errorf("spotify: search returned HTTP %d", resp.StatusCode)
	}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// accessToken returns the cached app token, fetching a new one when it is
// missing or about to expire.
func (s *Spotify) accessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && s.now().Before(s.expires) {
		return s.token, nil
	}

	ctx, cancel := context.WithTimeout(ctx, s.cfg.RequestTimeout)
	defer cancel()
	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.AccountsURL+"/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.cfg.ClientID, s.cfg.ClientSecret)
	resp, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("spotify: token endpoint returned HTTP %d", resp.StatusCode)
	}
	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("spotify: decode token: %w", err)
	}
	if tr.AccessToken == "" {
		return "", errors.New("spotify: empty access token")
	}
	s.token = tr.AccessToken
	s.expires = s.now().Add(time.Duration(tr.ExpiresIn)*time.Second - tokenMargin)
	return s.token, nil
}

func (s *Spotify) invalidateToken() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = ""
}

package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

type fakePinger struct{ err error }

func (p fakePinger) Ping(context.Context) error { return p.err }

var testFiles = fstest.MapFS{
	"index.html":      {Data: []byte("home")},
	"m/index.html":    {Data: []byte("share shell")},
	"es/m/index.html": {Data: []byte("share shell es")},
}

func newTestHandler(db Pinger) http.Handler {
	return NewHandler(db, testFiles, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func TestProbes(t *testing.T) {
	tests := []struct {
		name       string
		db         Pinger
		path       string
		wantStatus int
	}{
		{"healthz ignores database", fakePinger{err: errors.New("down")}, "/healthz", http.StatusOK},
		{"readyz with database up", fakePinger{}, "/readyz", http.StatusOK},
		{"readyz with database down", fakePinger{err: errors.New("down")}, "/readyz", http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if status, _ := get(t, newTestHandler(tt.db), tt.path); status != tt.wantStatus {
				t.Errorf("GET %s = %d, want %d", tt.path, status, tt.wantStatus)
			}
		})
	}
}

func TestStatic(t *testing.T) {
	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{"/", http.StatusOK, "home"},
		{"/m/a8Kd02LmQz", http.StatusOK, "share shell"},
		{"/es/m/a8Kd02LmQz", http.StatusOK, "share shell es"},
		{"/m/a8Kd02LmQz/extra", http.StatusNotFound, ""},
		{"/missing", http.StatusNotFound, ""},
	}
	h := newTestHandler(fakePinger{})
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			status, body := get(t, h, tt.path)
			if status != tt.wantStatus {
				t.Fatalf("GET %s = %d, want %d", tt.path, status, tt.wantStatus)
			}
			if tt.wantBody != "" && body != tt.wantBody {
				t.Errorf("GET %s body = %q, want %q", tt.path, body, tt.wantBody)
			}
		})
	}
}

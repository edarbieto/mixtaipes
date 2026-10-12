package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const okBody = `{
  "candidates": [{
    "content": {"role": "model", "parts": [
      {"text": "thinking...", "thought": true},
      {"text": "{\"status\":"},
      {"text": "\"ok\"}"}
    ]},
    "finishReason": "STOP"
  }],
  "usageMetadata": {"promptTokenCount": 100, "candidatesTokenCount": 40, "thoughtsTokenCount": 10},
  "modelVersion": "gemini-test-001"
}`

const overloadedBody = `{"error":{"code":503,"message":"This model is currently experiencing high demand.","status":"UNAVAILABLE"}}`

type geminiStub struct {
	mu     sync.Mutex
	paths  []string
	bodies []geminiRequest
	keys   []string
	// replies are served in order; the last one repeats.
	replies []stubReply
}

type stubReply struct {
	status int
	body   string
	delay  time.Duration
}

func (s *geminiStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body geminiRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	n := len(s.paths)
	s.paths = append(s.paths, r.URL.Path)
	s.bodies = append(s.bodies, body)
	s.keys = append(s.keys, r.Header.Get("x-goog-api-key"))
	reply := s.replies[min(n, len(s.replies)-1)]
	s.mu.Unlock()
	if reply.delay > 0 {
		select {
		case <-time.After(reply.delay):
		case <-r.Context().Done():
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(reply.status)
	_, _ = w.Write([]byte(reply.body))
}

func newStubbedGemini(t *testing.T, cfg GeminiConfig, replies ...stubReply) (*Gemini, *geminiStub) {
	t.Helper()
	stub := &geminiStub{replies: replies}
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	cfg.BaseURL = srv.URL
	cfg.APIKey = "test-key"
	if cfg.Model == "" {
		cfg.Model = "primary"
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = time.Millisecond
	}
	return NewGemini(cfg), stub
}

func TestGeminiRequestAndResponse(t *testing.T) {
	g, stub := newStubbedGemini(t, GeminiConfig{}, stubReply{status: 200, body: okBody})
	req := Request{
		System:      "be a curator",
		Messages:    []Message{{Role: RoleUser, Text: "rainy night"}},
		Schema:      PlaylistSchema,
		Temperature: 0.5,
	}
	resp, err := g.Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.JSON) != `{"status":"ok"}` {
		t.Errorf("JSON = %s, thought parts must be skipped", resp.JSON)
	}
	if resp.Model != "gemini-test-001" || resp.InputTokens != 100 || resp.OutputTokens != 50 {
		t.Errorf("accounting = %q %d/%d", resp.Model, resp.InputTokens, resp.OutputTokens)
	}
	if stub.paths[0] != "/models/primary:generateContent" || stub.keys[0] != "test-key" {
		t.Errorf("path %q key %q", stub.paths[0], stub.keys[0])
	}
	body := stub.bodies[0]
	if body.SystemInstruction == nil || body.SystemInstruction.Parts[0].Text != "be a curator" {
		t.Errorf("system instruction = %+v", body.SystemInstruction)
	}
	if len(body.Contents) != 1 || body.Contents[0].Role != "user" || body.Contents[0].Parts[0].Text != "rainy night" {
		t.Errorf("contents = %+v", body.Contents)
	}
	gc := body.GenerationConfig
	if gc.ResponseMimeType != "application/json" || gc.ResponseJSONSchema["type"] != "object" {
		t.Errorf("generation config = %+v", gc)
	}
	if gc.Temperature == nil || *gc.Temperature != 0.5 || gc.MaxOutputTokens != 0 {
		t.Errorf("temperature %v, max tokens %d", gc.Temperature, gc.MaxOutputTokens)
	}
}

func TestGeminiErrors(t *testing.T) {
	tests := []struct {
		name       string
		cfg        GeminiConfig
		replies    []stubReply
		wantErr    error
		wantReason Reason
		wantPaths  []string
	}{
		{
			name:      "overload retried on fallback model",
			cfg:       GeminiConfig{FallbackModel: "backup"},
			replies:   []stubReply{{status: 503, body: overloadedBody}, {status: 200, body: okBody}},
			wantPaths: []string{"/models/primary:generateContent", "/models/backup:generateContent"},
		},
		{
			name:      "overload retried on same model without fallback",
			replies:   []stubReply{{status: 500, body: `{}`}, {status: 200, body: okBody}},
			wantPaths: []string{"/models/primary:generateContent", "/models/primary:generateContent"},
		},
		{
			name:       "overload twice",
			replies:    []stubReply{{status: 503, body: overloadedBody}},
			wantErr:    ErrUnavailable,
			wantReason: ReasonOverloaded,
			wantPaths:  []string{"/models/primary:generateContent", "/models/primary:generateContent"},
		},
		{
			name:       "quota exhausted is not retried",
			replies:    []stubReply{{status: 429, body: `{"error":{"message":"quota"}}`}},
			wantErr:    ErrUnavailable,
			wantReason: ReasonRateLimited,
			wantPaths:  []string{"/models/primary:generateContent"},
		},
		{
			name:       "client error is not retried",
			replies:    []stubReply{{status: 400, body: `{"error":{"message":"bad schema"}}`}},
			wantErr:    ErrUnavailable,
			wantReason: ReasonUpstream,
			wantPaths:  []string{"/models/primary:generateContent"},
		},
		{
			name:       "timeout is not retried",
			cfg:        GeminiConfig{Timeout: 20 * time.Millisecond},
			replies:    []stubReply{{status: 200, body: okBody, delay: time.Second}},
			wantErr:    ErrUnavailable,
			wantReason: ReasonTimeout,
			wantPaths:  []string{"/models/primary:generateContent"},
		},
		{
			name:      "blocked prompt",
			replies:   []stubReply{{status: 200, body: `{"promptFeedback":{"blockReason":"SAFETY"}}`}},
			wantErr:   ErrBlocked,
			wantPaths: []string{"/models/primary:generateContent"},
		},
		{
			name:      "blocked response",
			replies:   []stubReply{{status: 200, body: `{"candidates":[{"content":{"parts":[]},"finishReason":"PROHIBITED_CONTENT"}]}`}},
			wantErr:   ErrBlocked,
			wantPaths: []string{"/models/primary:generateContent"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, stub := newStubbedGemini(t, tt.cfg, tt.replies...)
			_, err := g.Generate(context.Background(), Request{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantReason != "" {
				var ue *UnavailableError
				if !errors.As(err, &ue) || ue.Reason != tt.wantReason {
					t.Errorf("reason = %v, want %q", err, tt.wantReason)
				}
			}
			stub.mu.Lock()
			defer stub.mu.Unlock()
			if len(stub.paths) != len(tt.wantPaths) {
				t.Fatalf("paths = %v, want %v", stub.paths, tt.wantPaths)
			}
			for i := range tt.wantPaths {
				if stub.paths[i] != tt.wantPaths[i] {
					t.Errorf("path[%d] = %q, want %q", i, stub.paths[i], tt.wantPaths[i])
				}
			}
		})
	}
}

func TestGeminiCallerCancellation(t *testing.T) {
	g, _ := newStubbedGemini(t, GeminiConfig{}, stubReply{status: 200, body: okBody, delay: time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := g.Generate(ctx, Request{})
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want the caller's context error", err)
	}
}

func TestUnavailableErrorMessage(t *testing.T) {
	err := &UnavailableError{Reason: ReasonOverloaded, StatusCode: 503, Message: "high demand"}
	if got, want := err.Error(), "llm unavailable (overloaded, HTTP 503): high demand"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

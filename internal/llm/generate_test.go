package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeResult struct {
	json string
	err  error
}

// fakeProvider returns its results in order, one per call.
type fakeProvider struct {
	results []fakeResult
	calls   int
}

func (f *fakeProvider) Generate(context.Context, Request) (Response, error) {
	r := f.results[f.calls]
	f.calls++
	return Response{
		JSON:         []byte(r.json),
		Model:        "fake-model",
		InputTokens:  10,
		OutputTokens: 5,
		Latency:      time.Second,
	}, r.err
}

func TestGeneratePlaylist(t *testing.T) {
	valid := okJSON(tracksJSON(20))
	unavailable := errors.New("boom")
	tests := []struct {
		name       string
		results    []fakeResult
		wantErr    error
		wantCalls  int
		wantStatus Status
	}{
		{name: "valid first try", results: []fakeResult{{json: valid}}, wantCalls: 1, wantStatus: StatusOK},
		{name: "retries invalid output once", results: []fakeResult{{json: `{}`}, {json: valid}}, wantCalls: 2, wantStatus: StatusOK},
		{name: "invalid twice", results: []fakeResult{{json: `{}`}, {json: `nope`}}, wantErr: ErrInvalidOutput, wantCalls: 2},
		{name: "provider error is not retried", results: []fakeResult{{err: unavailable}}, wantErr: unavailable, wantCalls: 1},
		{name: "blocked becomes unsafe refusal", results: []fakeResult{{err: ErrBlocked}}, wantCalls: 1, wantStatus: StatusRefused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &fakeProvider{results: tt.results}
			res, err := GeneratePlaylist(context.Background(), p, Request{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if res.Calls != tt.wantCalls || p.calls != tt.wantCalls {
				t.Errorf("calls = %d (provider %d), want %d", res.Calls, p.calls, tt.wantCalls)
			}
			if res.InputTokens != 10*tt.wantCalls || res.OutputTokens != 5*tt.wantCalls {
				t.Errorf("tokens = %d/%d, not summed across %d calls", res.InputTokens, res.OutputTokens, tt.wantCalls)
			}
			if res.Latency != time.Duration(tt.wantCalls)*time.Second {
				t.Errorf("latency = %v, not summed", res.Latency)
			}
			if err == nil && res.Playlist.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", res.Playlist.Status, tt.wantStatus)
			}
		})
	}
}

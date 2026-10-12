package mixtape

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/edarbieto/mixtaipes/internal/catalog"
	"github.com/edarbieto/mixtaipes/internal/llm"
	"github.com/edarbieto/mixtaipes/prompts"
)

// fakeLLM answers playlist requests with playlist and backfill requests with
// replacements, recording every request.
type fakeLLM struct {
	playlist     llm.Playlist
	replacements []llm.Track
	backfillErr  error
	requests     []llm.Request
}

func (f *fakeLLM) Generate(_ context.Context, req llm.Request) (llm.Response, error) {
	f.requests = append(f.requests, req)
	var v any = f.playlist
	if _, isPlaylist := req.Schema["properties"].(map[string]any)["status"]; !isPlaylist {
		if f.backfillErr != nil {
			return llm.Response{Model: "fake"}, f.backfillErr
		}
		v = map[string]any{"tracks": f.replacements}
	}
	b, _ := json.Marshal(v)
	return llm.Response{JSON: b, Model: "fake", InputTokens: 1, OutputTokens: 1}, nil
}

// fakeCatalog matches titles listed in ids (title → catalog id).
type fakeCatalog struct {
	ids     map[string]string
	err     error
	batches [][]catalog.Query
}

func (f *fakeCatalog) Resolve(_ context.Context, _ string, qs []catalog.Query) ([]catalog.Match, error) {
	f.batches = append(f.batches, qs)
	if f.err != nil {
		return nil, f.err
	}
	out := make([]catalog.Match, len(qs))
	for i, q := range qs {
		if id, ok := f.ids[q.Title]; ok {
			out[i] = catalog.Match{Status: catalog.StatusMatched, Track: catalog.Track{ID: id, Name: q.Title}}
		} else {
			out[i] = catalog.Match{Status: catalog.StatusNotFound}
		}
	}
	return out, nil
}

func tracks(titles ...string) []llm.Track {
	out := make([]llm.Track, len(titles))
	for i, t := range titles {
		out[i] = llm.Track{Artist: "Artist", Title: t, Why: "fits"}
	}
	return out
}

func okPlaylist(titles ...string) llm.Playlist {
	return llm.Playlist{Status: llm.StatusOK, Title: "Mix", Tracks: tracks(titles...), Suggestions: []string{"more"}}
}

func run(t *testing.T, l *fakeLLM, c *fakeCatalog) (Outcome, error) {
	t.Helper()
	req, err := prompts.Build("v1", "en", "rainy night")
	if err != nil {
		t.Fatal(err)
	}
	return Pipeline{LLM: l, Catalog: c}.Run(context.Background(), "US", req)
}

func summary(o Outcome) string {
	parts := make([]string, len(o.Tracks))
	for i, t := range o.Tracks {
		s := t.Title + ":" + t.Match.Track.ID
		if t.Backfilled {
			s += "*"
		}
		parts[i] = s
	}
	return strings.Join(parts, " ")
}

func TestRunAllMatched(t *testing.T) {
	l := &fakeLLM{playlist: okPlaylist("a", "b")}
	c := &fakeCatalog{ids: map[string]string{"a": "1", "b": "2"}}
	out, err := run(t, l, c)
	if err != nil {
		t.Fatal(err)
	}
	if got := summary(out); got != "a:1 b:2" {
		t.Errorf("tracks = %s", got)
	}
	if len(l.requests) != 1 || out.LLM.Calls != 1 || len(c.batches) != 1 {
		t.Errorf("llm requests = %d, calls = %d, catalog batches = %d", len(l.requests), out.LLM.Calls, len(c.batches))
	}
	if out.Requested != 2 || out.MatchedBeforeBackfill != 2 || out.Matched() != 2 {
		t.Errorf("stats = %+v", out)
	}
}

func TestRunBackfill(t *testing.T) {
	l := &fakeLLM{
		playlist:     okPlaylist("a", "ghost", "a-again", "c"),
		replacements: tracks("r1", "r2-miss"),
	}
	// "a-again" resolves to the same id as "a": a duplicate.
	c := &fakeCatalog{ids: map[string]string{"a": "1", "a-again": "1", "c": "3", "r1": "9"}}
	out, err := run(t, l, c)
	if err != nil {
		t.Fatal(err)
	}
	// r1 takes the slot of "ghost"; r2 missed and is dropped; the duplicate is gone.
	if got := summary(out); got != "a:1 r1:9* c:3" {
		t.Errorf("tracks = %s", got)
	}
	if out.Requested != 4 || out.MatchedBeforeBackfill != 3 || out.Duplicates != 1 || out.Matched() != 3 {
		t.Errorf("stats: requested %d, before %d, dups %d, matched %d", out.Requested, out.MatchedBeforeBackfill, out.Duplicates, out.Matched())
	}
	if out.LLM.Calls != 2 || len(c.batches) != 2 {
		t.Errorf("llm calls = %d, catalog batches = %d", out.LLM.Calls, len(c.batches))
	}

	// The backfill request carries only LLM text and asks for misses + duplicates.
	bf := l.requests[1]
	last := bf.Messages[len(bf.Messages)-1].Text
	if !strings.Contains(last, "Artist — ghost") || !strings.Contains(last, "exactly 2 replacement") {
		t.Errorf("backfill message:\n%s", last)
	}
}

func TestRunKeepsMissWhenBackfillFails(t *testing.T) {
	l := &fakeLLM{playlist: okPlaylist("a", "ghost"), backfillErr: &llm.UnavailableError{Reason: llm.ReasonOverloaded}}
	c := &fakeCatalog{ids: map[string]string{"a": "1"}}
	out, err := run(t, l, c)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(out.BackfillErr, llm.ErrUnavailable) {
		t.Errorf("BackfillErr = %v", out.BackfillErr)
	}
	if got := summary(out); got != "a:1 ghost:" {
		t.Errorf("tracks = %s", got)
	}
	if out.Tracks[1].Match.Status != catalog.StatusNotFound {
		t.Errorf("miss not kept: %+v", out.Tracks[1])
	}
}

func TestRunRefused(t *testing.T) {
	l := &fakeLLM{playlist: llm.Refused(llm.RefusalOffTopic)}
	c := &fakeCatalog{}
	out, err := run(t, l, c)
	if err != nil {
		t.Fatal(err)
	}
	if out.Playlist.Status != llm.StatusRefused || len(out.Tracks) != 0 || len(c.batches) != 0 {
		t.Errorf("refused turn touched the catalog: %+v", out)
	}
}

func TestRunCatalogUnavailable(t *testing.T) {
	l := &fakeLLM{playlist: okPlaylist("a")}
	c := &fakeCatalog{err: catalog.ErrUnavailable}
	out, err := run(t, l, c)
	if !errors.Is(err, catalog.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if out.LLM.Calls != 1 {
		t.Errorf("usage lost on failure: %+v", out.LLM)
	}
}

func TestRunLLMUnavailable(t *testing.T) {
	l := &failingLLM{}
	_, err := Pipeline{LLM: l, Catalog: &fakeCatalog{}}.Run(context.Background(), "US", llm.Request{})
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

type failingLLM struct{}

func (failingLLM) Generate(context.Context, llm.Request) (llm.Response, error) {
	return llm.Response{}, &llm.UnavailableError{Reason: llm.ReasonTimeout}
}

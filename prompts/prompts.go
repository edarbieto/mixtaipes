// Package prompts holds the versioned system prompts and builds LLM requests
// from them (ADR-004). It must not depend on catalog code (ADR-007).
package prompts

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/edarbieto/mixtaipes/internal/llm"
)

//go:embed v*.md
var files embed.FS

// Load returns the system prompt for a version such as "v1".
func Load(version string) (string, error) {
	if _, ok := versionNumber(version); !ok {
		return "", fmt.Errorf("invalid prompt version %q", version)
	}
	b, err := files.ReadFile(version + ".md")
	if err != nil {
		return "", fmt.Errorf("unknown prompt version %q", version)
	}
	return string(b), nil
}

// Latest returns the highest embedded prompt version.
func Latest() string {
	entries, _ := fs.ReadDir(files, ".")
	latest, best := "", 0
	for _, e := range entries {
		v := strings.TrimSuffix(e.Name(), ".md")
		if n, ok := versionNumber(v); ok && n > best {
			latest, best = v, n
		}
	}
	return latest
}

// versionNumber parses "vN" with N ≥ 1.
func versionNumber(version string) (int, bool) {
	digits, ok := strings.CutPrefix(version, "v")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	return n, err == nil && n >= 1
}

// dataEscaper keeps user text from closing or opening the delimiting element.
var dataEscaper = strings.NewReplacer("<", "&lt;", ">", "&gt;")

// Build assembles a first-turn request: the system prompt of the given
// version and the user's text wrapped as data in <user_request>.
func Build(version, locale, userText string) (llm.Request, error) {
	var msg strings.Builder
	msg.WriteString("Locale: " + locale + "\n")
	writeElement(&msg, "user_request", userText)
	return newRequest(version, msg.String())
}

// Refinement is the context of a refine turn (ADR-008). It is built only from
// stored user prompts and the latest ok turn's llm_output (ADR-007).
type Refinement struct {
	Original    string
	Previous    []string
	Current     llm.Playlist
	Instruction string
}

// BuildRefine assembles a refine-turn request: original request, previous
// refinements, the current playlist as the model wrote it, and the new
// instruction, all wrapped as data.
func BuildRefine(version, locale string, r Refinement) (llm.Request, error) {
	var msg strings.Builder
	msg.WriteString("Locale: " + locale + "\n")
	writeElement(&msg, "original_request", r.Original)
	if len(r.Previous) > 0 {
		var prev strings.Builder
		for i, p := range r.Previous {
			fmt.Fprintf(&prev, "%d. %s\n", i+1, strings.TrimSpace(p))
		}
		writeElement(&msg, "previous_refinements", prev.String())
	}
	var cur strings.Builder
	fmt.Fprintf(&cur, "Title: %s\n", r.Current.Title)
	for i, t := range r.Current.Tracks {
		fmt.Fprintf(&cur, "%d. %s — %s — %s\n", i+1, t.Artist, t.Title, t.Why)
	}
	writeElement(&msg, "current_playlist", cur.String())
	writeElement(&msg, "user_request", r.Instruction)
	return newRequest(version, msg.String())
}

// writeElement writes text as an escaped, delimited data element.
func writeElement(b *strings.Builder, name, text string) {
	fmt.Fprintf(b, "<%s>\n%s\n</%s>\n", name, dataEscaper.Replace(strings.TrimSpace(text)), name)
}

func newRequest(version, userMessage string) (llm.Request, error) {
	system, err := Load(version)
	if err != nil {
		return llm.Request{}, err
	}
	return llm.Request{
		System:   system,
		Messages: []llm.Message{{Role: llm.RoleUser, Text: strings.TrimSuffix(userMessage, "\n")}},
		Schema:   llm.PlaylistSchema,
	}, nil
}

// BuildBackfill continues a generation request with the model's own playlist
// and asks for n replacement tracks (ADR-006). Its inputs carry only LLM
// text: tracks that were not found are listed as the model wrote them (ADR-007).
func BuildBackfill(base llm.Request, playlist llm.Playlist, notFound []llm.Track, n int) (llm.Request, error) {
	if n < 1 {
		return llm.Request{}, errors.New("backfill needs at least one track")
	}
	own, err := json.Marshal(playlist)
	if err != nil {
		return llm.Request{}, fmt.Errorf("encode playlist: %w", err)
	}
	var msg strings.Builder
	fmt.Fprintf(&msg, "%d track(s) of your playlist could not be used: they were not found in the catalog or duplicated another track.\n", n)
	if len(notFound) > 0 {
		msg.WriteString("<not_found>\n")
		for i, t := range notFound {
			fmt.Fprintf(&msg, "%d. %s — %s\n", i+1, dataEscaper.Replace(t.Artist), dataEscaper.Replace(t.Title))
		}
		msg.WriteString("</not_found>\n")
	}
	fmt.Fprintf(&msg, "Propose exactly %d replacement track(s).", n)

	req := base
	req.Messages = append(append([]llm.Message{}, base.Messages...),
		llm.Message{Role: llm.RoleModel, Text: string(own)},
		llm.Message{Role: llm.RoleUser, Text: msg.String()},
	)
	req.Schema = llm.ReplacementsSchema(n)
	return req, nil
}

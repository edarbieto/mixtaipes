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
	system, err := Load(version)
	if err != nil {
		return llm.Request{}, err
	}
	msg := "Locale: " + locale + "\n<user_request>\n" + dataEscaper.Replace(strings.TrimSpace(userText)) + "\n</user_request>"
	return llm.Request{
		System:   system,
		Messages: []llm.Message{{Role: llm.RoleUser, Text: msg}},
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

// Package prompts holds the versioned system prompts and builds LLM requests
// from them (ADR-004). It must not depend on catalog code (ADR-007).
package prompts

import (
	"embed"
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

// Package llm is the language-model boundary: the Provider interface, the
// playlist output schema and its validation (ADR-003).
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Role identifies the author of a conversation message.
type Role string

// Message roles.
const (
	RoleUser  Role = "user"
	RoleModel Role = "model"
)

// Message is one entry of the ordered conversation sent to the model.
type Message struct {
	Role Role
	Text string
}

// Request is a provider-neutral generation request.
type Request struct {
	System   string
	Messages []Message
	// Schema is the JSON Schema the output must conform to.
	Schema map[string]any
	// Temperature and MaxOutputTokens leave the provider default when zero.
	Temperature     float64
	MaxOutputTokens int
}

// Response is the raw result of one generation.
type Response struct {
	JSON         json.RawMessage
	Model        string
	InputTokens  int
	OutputTokens int
	Latency      time.Duration
}

// Provider generates structured output from a model.
type Provider interface {
	Generate(ctx context.Context, req Request) (Response, error)
}

var (
	// ErrUnavailable reports a provider error, overload or timeout (LLM_UNAVAILABLE).
	ErrUnavailable = errors.New("llm: provider unavailable")
	// ErrInvalidOutput reports output that failed validation twice (LLM_INVALID_OUTPUT).
	ErrInvalidOutput = errors.New("llm: invalid output")
	// ErrBlocked reports a request or response blocked by provider safety filters.
	ErrBlocked = errors.New("llm: blocked by provider safety filters")
)

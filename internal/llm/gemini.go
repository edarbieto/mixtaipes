package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Gemini adapter defaults.
const (
	DefaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	DefaultTimeout       = 20 * time.Second
	DefaultRetryDelay    = time.Second
)

// maxErrorBody bounds how much of an error response is read.
const maxErrorBody = 64 << 10

// Reason classifies why the provider was unavailable.
type Reason string

// Unavailability reasons.
const (
	ReasonTimeout     Reason = "timeout"
	ReasonOverloaded  Reason = "overloaded"
	ReasonRateLimited Reason = "rate_limited"
	ReasonUpstream    Reason = "upstream_error"
)

// UnavailableError describes a failed provider call; it matches ErrUnavailable.
type UnavailableError struct {
	Reason     Reason
	StatusCode int
	Message    string
	Err        error
}

func (e *UnavailableError) Error() string {
	msg := "llm unavailable (" + string(e.Reason)
	if e.StatusCode != 0 {
		msg += fmt.Sprintf(", HTTP %d", e.StatusCode)
	}
	msg += ")"
	if e.Message != "" {
		msg += ": " + e.Message
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

// Is reports whether target is ErrUnavailable.
func (e *UnavailableError) Is(target error) bool { return target == ErrUnavailable }

func (e *UnavailableError) Unwrap() error { return e.Err }

// GeminiConfig configures the Gemini adapter. Zero values take the defaults.
type GeminiConfig struct {
	APIKey string
	Model  string
	// FallbackModel, when set, is used for the retry after an overload error.
	FallbackModel string
	BaseURL       string
	// Timeout bounds each HTTP call.
	Timeout    time.Duration
	RetryDelay time.Duration
	HTTPClient *http.Client
}

// Gemini implements Provider with the Gemini API's structured-output mode.
type Gemini struct {
	cfg GeminiConfig
}

// NewGemini returns a Gemini provider.
func NewGemini(cfg GeminiConfig) *Gemini {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultGeminiBaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = DefaultRetryDelay
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	return &Gemini{cfg: cfg}
}

// Generate calls the model once. When the provider reports an overload
// (HTTP 500/502/503/504) it waits RetryDelay and retries once, on
// FallbackModel if configured. Timeouts and quota errors are not retried:
// they would exceed the latency budget or fail again.
func (g *Gemini) Generate(ctx context.Context, req Request) (Response, error) {
	start := time.Now()
	resp, err := g.call(ctx, g.cfg.Model, req)
	var ue *UnavailableError
	if errors.As(err, &ue) && ue.Reason == ReasonOverloaded {
		select {
		case <-ctx.Done():
			return resp, err
		case <-time.After(g.cfg.RetryDelay):
		}
		model := g.cfg.Model
		if g.cfg.FallbackModel != "" {
			model = g.cfg.FallbackModel
		}
		resp, err = g.call(ctx, model, req)
	}
	resp.Latency = time.Since(start)
	return resp, err
}

type geminiPart struct {
	Text    string `json:"text,omitempty"`
	Thought bool   `json:"thought,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	ResponseMimeType   string         `json:"responseMimeType"`
	ResponseJSONSchema map[string]any `json:"responseJsonSchema,omitempty"`
	Temperature        *float64       `json:"temperature,omitempty"`
	MaxOutputTokens    int            `json:"maxOutputTokens,omitempty"`
}

type geminiRequest struct {
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	Contents          []geminiContent        `json:"contents"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

type geminiResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
	ModelVersion string `json:"modelVersion"`
}

type geminiError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// blockedFinishReasons are finish reasons that mean the response was filtered.
var blockedFinishReasons = map[string]bool{
	"SAFETY":             true,
	"PROHIBITED_CONTENT": true,
	"BLOCKLIST":          true,
	"SPII":               true,
}

func (g *Gemini) call(ctx context.Context, model string, req Request) (Response, error) {
	resp := Response{Model: model}
	body, err := json.Marshal(newGeminiRequest(req))
	if err != nil {
		return resp, fmt.Errorf("encode gemini request: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, g.cfg.Timeout)
	defer cancel()
	url := g.cfg.BaseURL + "/models/" + model + ":generateContent"
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return resp, fmt.Errorf("build gemini request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", g.cfg.APIKey)

	httpResp, err := g.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return resp, ctx.Err()
		}
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return resp, &UnavailableError{Reason: ReasonTimeout, Err: err}
		}
		return resp, &UnavailableError{Reason: ReasonUpstream, Err: err}
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode != http.StatusOK {
		return resp, statusError(httpResp)
	}

	var gr geminiResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&gr); err != nil {
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return resp, &UnavailableError{Reason: ReasonTimeout, Err: err}
		}
		return resp, &UnavailableError{Reason: ReasonUpstream, Err: fmt.Errorf("decode gemini response: %w", err)}
	}
	if gr.ModelVersion != "" {
		resp.Model = gr.ModelVersion
	}
	resp.InputTokens = gr.UsageMetadata.PromptTokenCount
	resp.OutputTokens = gr.UsageMetadata.CandidatesTokenCount + gr.UsageMetadata.ThoughtsTokenCount

	if gr.PromptFeedback.BlockReason != "" {
		return resp, ErrBlocked
	}
	if len(gr.Candidates) == 0 {
		// No candidate and no block reason: treat as invalid output.
		return resp, nil
	}
	cand := gr.Candidates[0]
	if blockedFinishReasons[cand.FinishReason] {
		return resp, ErrBlocked
	}
	var text strings.Builder
	for _, part := range cand.Content.Parts {
		if !part.Thought {
			text.WriteString(part.Text)
		}
	}
	resp.JSON = json.RawMessage(text.String())
	return resp, nil
}

func newGeminiRequest(req Request) geminiRequest {
	gr := geminiRequest{
		Contents: make([]geminiContent, 0, len(req.Messages)),
		GenerationConfig: geminiGenerationConfig{
			ResponseMimeType:   "application/json",
			ResponseJSONSchema: req.Schema,
			MaxOutputTokens:    req.MaxOutputTokens,
		},
	}
	if req.System != "" {
		gr.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: req.System}}}
	}
	if req.Temperature != 0 {
		t := req.Temperature
		gr.GenerationConfig.Temperature = &t
	}
	for _, m := range req.Messages {
		gr.Contents = append(gr.Contents, geminiContent{Role: string(m.Role), Parts: []geminiPart{{Text: m.Text}}})
	}
	return gr
}

func statusError(resp *http.Response) error {
	ue := &UnavailableError{StatusCode: resp.StatusCode, Reason: ReasonUpstream}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		ue.Reason = ReasonRateLimited
	case http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		ue.Reason = ReasonOverloaded
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	var ge geminiError
	if json.Unmarshal(body, &ge) == nil {
		ue.Message = ge.Error.Message
	}
	return ue
}

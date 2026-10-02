package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxRetries is the default number of retry attempts after the initial
// request on transient failures, per PLAN.md ("retry w/ backoff x2").
const maxRetries = 2

// retryBaseDelay is the base backoff delay; attempt N waits
// retryBaseDelay*2^(N-1), capped at maxRetryDelay, plus jitter.
const retryBaseDelay = 250 * time.Millisecond

// maxRetryDelay caps both the exponential backoff and a server-sent
// Retry-After so one bad gateway cannot stall a worker for minutes.
const maxRetryDelay = 30 * time.Second

// maxResponseBytes bounds how much of a response body is read; a runaway
// or hostile upstream must not exhaust memory.
const maxResponseBytes = 32 << 20

// jitterFraction is the share of the backoff delay added as random jitter,
// so concurrent workers retrying one saturated gateway do not stampede it
// in lockstep.
const jitterFraction = 0.25

// OpenAIClient talks to an OpenAI-compatible /v1/chat/completions endpoint.
type OpenAIClient struct {
	httpClient      *http.Client
	modelTimeouts   map[string]time.Duration
	endpoint        string
	apiKey          string
	retries         int
	baseDelay       time.Duration
	timeout         time.Duration
	noRetryTimeouts bool
	stream          bool
	idleTimeout     time.Duration
}

// NewOpenAIClient builds a client against endpoint (e.g.
// "https://host/v1") using apiKey (may be empty for keyless gateways) and
// per-request timeout.
func NewOpenAIClient(endpoint, apiKey string, timeout time.Duration) *OpenAIClient {
	return &OpenAIClient{
		endpoint:  strings.TrimRight(endpoint, "/"),
		apiKey:    apiKey,
		retries:   maxRetries,
		baseDelay: retryBaseDelay,
		timeout:   timeout,
		// The per-attempt deadline is applied through the context so it can
		// differ per model; the client itself has no overall timeout.
		httpClient: &http.Client{},
	}
}

// WithModelTimeouts sets per-model request timeouts that override the
// default; a slow local model can be given far longer than a hosted one.
func (c *OpenAIClient) WithModelTimeouts(m map[string]time.Duration) *OpenAIClient {
	c.modelTimeouts = m
	return c
}

// WithRetryTimeouts controls whether an attempt that hit its timeout is
// retried. Retrying re-runs a full slow generation, so for slow models
// disabling it avoids multiplying the wait by the attempt count.
func (c *OpenAIClient) WithRetryTimeouts(retry bool) *OpenAIClient {
	c.noRetryTimeouts = !retry
	return c
}

// WithStreaming makes the client request SSE streams. The per-request
// timeout stays a hard ceiling; idle (default 5m when zero) is how long the
// stream may go silent before the attempt is abandoned, so a hung server is
// detected early while a slow-but-progressing generation is left to finish.
func (c *OpenAIClient) WithStreaming(enabled bool, idle time.Duration) *OpenAIClient {
	c.stream = enabled
	c.idleTimeout = idle
	if enabled && idle <= 0 {
		c.idleTimeout = defaultIdleTimeout
	}
	return c
}

func (c *OpenAIClient) timeoutFor(model string) time.Duration {
	if d, ok := c.modelTimeouts[model]; ok && d > 0 {
		return d
	}
	return c.timeout
}

// WithRetries sets the number of retry attempts after the initial request
// (0 disables retrying) and returns the client.
func (c *OpenAIClient) WithRetries(n int) *OpenAIClient {
	c.retries = max(n, 0)
	return c
}

// backoff returns the wait before retry number attempt (1-based): the
// server's Retry-After when it sent one, else exponential with jitter.
func (c *OpenAIClient) backoff(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return min(retryAfter, maxRetryDelay)
	}
	d := min(c.baseDelay<<(attempt-1), maxRetryDelay)
	return d + time.Duration(rand.Float64()*jitterFraction*float64(d)) //nolint:gosec // jitter needs no cryptographic randomness
}

type chatCompletionRequest struct {
	Seed          *int           `json:"seed,omitempty"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
	Model         string         `json:"model"`
	ToolChoice    string         `json:"tool_choice,omitempty"`
	Messages      []chatMsg      `json:"messages"`
	Tools         []apiTool      `json:"tools,omitempty"`
	MaxTokens     int            `json:"max_tokens,omitempty"`
	Temperature   float64        `json:"temperature"`
	Stream        bool           `json:"stream,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// apiTool is the OpenAI tools[] wire shape: {"type":"function","function":{...}}.
type apiTool struct {
	Function apiToolFunction `json:"function"`
	Type     string          `json:"type"`
}

type apiToolFunction struct {
	Parameters  map[string]any `json:"parameters,omitempty"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
}

// apiToolCall is the OpenAI tool_calls[] wire shape. Function.Arguments is a
// JSON-encoded STRING, not a nested object, per the OpenAI spec.
type apiToolCall struct {
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
	Type string `json:"type"`
}

// responseMsg is the assistant message inside one response choice. Content
// is a pointer, not a plain string: some OpenAI-compatible backends return
// "content": null when generation was cut off before any answer token was
// emitted (for example finish_reason=length on a reasoning model that
// spent its whole token budget on reasoning_content and never reached
// content). Text reports that case as "", explicitly, rather than relying
// on encoding/json's implicit null-into-string zero-value behavior.
//
// Deliberately not evaluated: reasoning_content. The catalog's evaluators
// score the answer a user would actually see, which is content only.
type responseMsg struct {
	Content   *string       `json:"content"`
	Role      string        `json:"role"`
	ToolCalls []apiToolCall `json:"tool_calls"`
}

// Text returns m.Content, or "" if the backend sent "content": null.
func (m responseMsg) Text() string {
	if m.Content == nil {
		return ""
	}
	return *m.Content
}

type chatCompletionResponse struct {
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Model             string `json:"model"`
	SystemFingerprint string `json:"system_fingerprint"`
	Choices           []struct {
		FinishReason string      `json:"finish_reason"`
		Message      responseMsg `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// decodeToolCalls converts the OpenAI wire tool_calls into normalized
// ToolCall values, decoding each function's JSON-encoded argument string
// into an object. A call whose argument string is not valid JSON is kept
// with Decoded=false and its raw string preserved, so a test can still see
// that the tool was named even when the model emitted malformed arguments.
func decodeToolCalls(raw []apiToolCall) []ToolCall {
	if len(raw) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(raw))
	for _, tc := range raw {
		call := ToolCall{Name: tc.Function.Name, RawArguments: tc.Function.Arguments}
		var args map[string]any
		if tc.Function.Arguments == "" {
			// No-argument tool call: an empty argument object, decoded.
			call.Arguments = map[string]any{}
			call.Decoded = true
		} else if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil {
			call.Arguments = args
			call.Decoded = true
		}
		out = append(out, call)
	}
	return out
}

// Complete implements Client. It retries transport failures and 5xx
// responses up to maxRetries times with linear backoff.
func (c *OpenAIClient) Complete(ctx context.Context, req Request) (Response, error) {
	body := chatCompletionRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Seed:        req.Seed,
	}
	for _, m := range req.Messages {
		body.Messages = append(body.Messages, chatMsg(m))
	}
	if len(req.Tools) > 0 {
		// tool_choice "auto" lets the model decide whether and which tool to
		// call: the honest test of both "must call X" and "needs no tool".
		body.ToolChoice = "auto"
		for _, t := range req.Tools {
			body.Tools = append(body.Tools, apiTool{
				Type:     "function",
				Function: apiToolFunction(t),
			})
		}
	}

	if c.stream {
		body.Stream = true
		body.StreamOptions = &streamOptions{IncludeUsage: true}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return Response{}, fmt.Errorf("llm: marshal request: %w", err)
	}

	var lastErr error
	var retryAfter time.Duration
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			delay := c.backoff(attempt, retryAfter)
			select {
			case <-ctx.Done():
				return Response{}, ctx.Err()
			case <-time.After(delay):
			}
		}

		// Measured per-attempt, starting after any backoff delay: Latency
		// reflects the successful attempt's own generation time, not the
		// cumulative wait across earlier failed/retried attempts (S8).
		attemptStart := time.Now()
		attemptCtx, cancel := context.WithTimeout(ctx, c.timeoutFor(req.Model))
		resp, err := c.doRequest(attemptCtx, payload)
		timedOut := (attemptCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil) || errors.Is(err, errIdleTimeout)
		cancel()
		retryAfter = 0
		if err != nil {
			lastErr = err
			if timedOut && c.noRetryTimeouts {
				return Response{}, fmt.Errorf("llm: timed out after %s (not retried): %w", c.timeoutFor(req.Model), err)
			}
			var re *retryableError
			if errors.As(err, &re) {
				retryAfter = re.retryAfter
				continue
			}
			return Response{}, err
		}

		latency := time.Since(attemptStart)
		if resp.Error == nil && len(resp.Choices) == 0 {
			// A 2xx with neither choices nor an error object is a gateway
			// glitch, not an answer; scoring it would record a spurious
			// wrong response.
			lastErr = errors.New("llm: response contained no choices")
			continue
		}
		var text, finishReason string
		var toolCalls []ToolCall
		if len(resp.Choices) > 0 {
			text = resp.Choices[0].Message.Text()
			finishReason = resp.Choices[0].FinishReason
			toolCalls = decodeToolCalls(resp.Choices[0].Message.ToolCalls)
		}
		if resp.Error != nil {
			// A 2xx carrying a JSON error body is how some gateways report a
			// transient upstream failure (observed as instant 0-token
			// "errors" in the 2026-08-30 run while a replica was warming).
			// Treat it like a 5xx: retry within the same attempt budget.
			lastErr = fmt.Errorf("llm: api error: %s", resp.Error.Message)
			continue
		}
		return Response{
			Text:             text,
			FinishReason:     finishReason,
			ToolCalls:        toolCalls,
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			Latency:          latency,
			Fingerprint:      resp.SystemFingerprint,
			ServedModel:      resp.Model,
		}, nil
	}
	return Response{}, fmt.Errorf("llm: request failed after %d attempts: %w", c.retries+1, lastErr)
}

type retryableError struct {
	err        error
	status     int
	retryAfter time.Duration
}

func (e *retryableError) Error() string {
	if e.status != 0 {
		return fmt.Sprintf("llm: server returned status %d", e.status)
	}
	return fmt.Sprintf("llm: transport error: %v", e.err)
}

func (e *retryableError) Unwrap() error { return e.err }

// parseRetryAfter reads a delta-seconds Retry-After header; HTTP-date form
// and malformed values yield 0 (fall back to computed backoff).
func parseRetryAfter(h string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

func (c *OpenAIClient) doRequest(ctx context.Context, payload []byte) (*chatCompletionResponse, error) {
	var watchdog *idleWatchdog
	if c.stream {
		var cancel context.CancelCauseFunc
		ctx, cancel = context.WithCancelCause(ctx)
		defer cancel(nil)
		watchdog = startIdleWatchdog(cancel, c.idleTimeout)
		defer watchdog.Stop()
	}
	out, err := c.send(ctx, payload, watchdog)
	if err != nil && c.stream && errors.Is(context.Cause(ctx), errIdleTimeout) {
		return nil, &retryableError{err: fmt.Errorf("no data for %s: %w", c.idleTimeout, errIdleTimeout)}
	}
	return out, err
}

func (c *OpenAIClient) send(ctx context.Context, payload []byte, watchdog *idleWatchdog) (*chatCompletionResponse, error) {
	url := c.endpoint + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("llm: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &retryableError{err: err}
	}
	defer func() { _ = httpResp.Body.Close() }() // best-effort; body already fully read below
	if watchdog != nil {
		watchdog.Touch()
	}

	// 429 and 408 are as transient as a 5xx for a self-hosted gateway (rate
	// limit, briefly saturated upstream, slow proxy); retry them with the
	// same backoff budget.
	if httpResp.StatusCode >= 500 || httpResp.StatusCode == http.StatusTooManyRequests || httpResp.StatusCode == http.StatusRequestTimeout {
		return nil, &retryableError{
			status:     httpResp.StatusCode,
			retryAfter: parseRetryAfter(httpResp.Header.Get("Retry-After")),
		}
	}

	// A gateway may answer a streaming request with a plain JSON body (an
	// error, or no streaming support); only parse SSE when it sent SSE.
	if c.stream && httpResp.StatusCode < 400 && strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream") {
		return readStream(httpResp.Body, watchdog.Touch)
	}

	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, maxResponseBytes))
	if err != nil {
		return nil, &retryableError{err: fmt.Errorf("read body: %w", err)}
	}
	if httpResp.StatusCode >= 400 {
		return nil, fmt.Errorf("llm: server returned status %d: %s", httpResp.StatusCode, string(respBody))
	}

	var out chatCompletionResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("llm: decode response: %w", err)
	}
	return &out, nil
}

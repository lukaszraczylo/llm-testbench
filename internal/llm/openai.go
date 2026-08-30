package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// maxRetries is the number of retry attempts after the initial request on
// transport errors or 5xx responses, per PLAN.md ("retry w/ backoff x2").
const maxRetries = 2

// retryBaseDelay is the base backoff delay; attempt N waits N*retryBaseDelay.
const retryBaseDelay = 250 * time.Millisecond

// OpenAIClient talks to an OpenAI-compatible /v1/chat/completions endpoint.
type OpenAIClient struct {
	httpClient *http.Client
	endpoint   string
	apiKey     string
}

// NewOpenAIClient builds a client against endpoint (e.g.
// "https://host/v1") using apiKey (may be empty for keyless gateways) and
// per-request timeout.
func NewOpenAIClient(endpoint, apiKey string, timeout time.Duration) *OpenAIClient {
	return &OpenAIClient{
		endpoint: endpoint,
		apiKey:   apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type chatCompletionRequest struct {
	Model       string    `json:"model"`
	ToolChoice  string    `json:"tool_choice,omitempty"`
	Messages    []chatMsg `json:"messages"`
	Tools       []apiTool `json:"tools,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature"`
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
	Choices []struct {
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

	payload, err := json.Marshal(body)
	if err != nil {
		return Response{}, fmt.Errorf("llm: marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * retryBaseDelay
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
		resp, err := c.doRequest(ctx, payload)
		if err != nil {
			lastErr = err
			if isRetryable(err) {
				continue
			}
			return Response{}, err
		}

		latency := time.Since(attemptStart)
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
		}, nil
	}
	return Response{}, fmt.Errorf("llm: request failed after %d attempts: %w", maxRetries+1, lastErr)
}

type retryableError struct {
	err    error
	status int
}

func (e *retryableError) Error() string {
	if e.status != 0 {
		return fmt.Sprintf("llm: server returned status %d", e.status)
	}
	return fmt.Sprintf("llm: transport error: %v", e.err)
}

func (e *retryableError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var re *retryableError
	return errors.As(err, &re)
}

func (c *OpenAIClient) doRequest(ctx context.Context, payload []byte) (*chatCompletionResponse, error) {
	url := c.endpoint + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("llm: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &retryableError{err: err}
	}
	defer func() { _ = httpResp.Body.Close() }() // best-effort; body already fully read below

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, &retryableError{err: fmt.Errorf("read body: %w", err)}
	}

	// 429 is as transient as a 5xx for a self-hosted gateway (rate limit or
	// a briefly saturated upstream); retry it with the same backoff budget.
	if httpResp.StatusCode >= 500 || httpResp.StatusCode == http.StatusTooManyRequests {
		return nil, &retryableError{status: httpResp.StatusCode}
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

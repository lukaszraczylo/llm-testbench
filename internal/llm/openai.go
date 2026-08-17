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
	Messages    []chatMsg `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature"`
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
	Content *string `json:"content"`
	Role    string  `json:"role"`
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
		Message      responseMsg `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
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
		if len(resp.Choices) > 0 {
			text = resp.Choices[0].Message.Text()
			finishReason = resp.Choices[0].FinishReason
		}
		if resp.Error != nil {
			lastErr = fmt.Errorf("llm: api error: %s", resp.Error.Message)
			return Response{}, lastErr
		}
		return Response{
			Text:             text,
			FinishReason:     finishReason,
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

	if httpResp.StatusCode >= 500 {
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

package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestServer spins up a local httptest server; this is not the live
// endpoint and is safe for offline unit tests.
func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// testChoice builds one response choice with a non-null content string, the
// common case. Its return type is exactly chatCompletionResponse.Choices's
// element type, so callers can append() it without repeating the anonymous
// struct type at each call site.
func testChoice(content, finishReason string) struct {
	Message      responseMsg `json:"message"`
	FinishReason string      `json:"finish_reason"`
} {
	return struct {
		Message      responseMsg `json:"message"`
		FinishReason string      `json:"finish_reason"`
	}{
		Message:      responseMsg{Role: "assistant", Content: &content},
		FinishReason: finishReason,
	}
}

func TestOpenAIClient_Complete_Success(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "test-model" {
			t.Errorf("model = %q, want test-model", body.Model)
		}
		resp := chatCompletionResponse{}
		resp.Choices = append(resp.Choices, testChoice("hello there", "stop"))
		resp.Usage.PromptTokens = 12
		resp.Usage.CompletionTokens = 3
		_ = json.NewEncoder(w).Encode(resp)
	})

	client := NewOpenAIClient(srv.URL, "", time.Second)
	resp, err := client.Complete(context.Background(), Request{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp.Text != "hello there" {
		t.Errorf("Text = %q, want %q", resp.Text, "hello there")
	}
	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want stop", resp.FinishReason)
	}
	if resp.PromptTokens != 12 || resp.CompletionTokens != 3 {
		t.Errorf("tokens = %d/%d, want 12/3", resp.PromptTokens, resp.CompletionTokens)
	}
}

// TestOpenAIClient_Complete_ParsesFinishReasonLength verifies
// choices[0].finish_reason lands in Response.FinishReason == "length", the
// signal that generation was cut off by the token budget (root cause of the
// live-run incident: a truncated reasoning-model response silently scored
// 0 with no visible signal that it was ever cut off).
func TestOpenAIClient_Complete_ParsesFinishReasonLength(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		resp := chatCompletionResponse{}
		resp.Choices = append(resp.Choices, testChoice("partial ans", FinishReasonLength))
		_ = json.NewEncoder(w).Encode(resp)
	})

	client := NewOpenAIClient(srv.URL, "", time.Second)
	resp, err := client.Complete(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp.FinishReason != FinishReasonLength {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, FinishReasonLength)
	}
}

// TestOpenAIClient_Complete_NullContentIsEmptyString verifies a backend
// sending "content": null (a reasoning model that spent its whole token
// budget on reasoning_content and never wrote to content) decodes to
// Response.Text == "", not a decode error or a panic.
func TestOpenAIClient_Complete_NullContentIsEmptyString(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null},"finish_reason":"length"}],"usage":{"prompt_tokens":50,"completion_tokens":6000}}`))
	})

	client := NewOpenAIClient(srv.URL, "", time.Second)
	resp, err := client.Complete(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp.Text != "" {
		t.Errorf("Text = %q, want empty string for null content", resp.Text)
	}
	if resp.FinishReason != FinishReasonLength {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, FinishReasonLength)
	}
	if resp.CompletionTokens != 6000 {
		t.Errorf("CompletionTokens = %d, want 6000 (tokens were spent on reasoning_content, not content)", resp.CompletionTokens)
	}
}

// TestOpenAIClient_Complete_SendsTemperatureZero verifies Request.Temperature
// (PLAN.md pins this to 0 for deterministic scoring, wired from main.go's
// requestTemperature constant) actually lands on the wire as a JSON
// "temperature":0 field, not silently omitted (chatCompletionRequest has no
// "omitempty" on Temperature specifically so a real zero is distinguishable
// from an absent field) (N7).
func TestOpenAIClient_Complete_SendsTemperatureZero(t *testing.T) {
	var rawBody []byte
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var err error
		rawBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(chatCompletionResponse{})
	})

	client := NewOpenAIClient(srv.URL, "", time.Second)
	_, err := client.Complete(context.Background(), Request{
		Model:       "m",
		Messages:    []Message{{Role: "user", Content: "x"}},
		Temperature: 0,
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(rawBody, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(rawBody) error = %v; body: %s", err, rawBody)
	}
	got, present := decoded["temperature"]
	if !present {
		t.Fatalf("request body has no \"temperature\" key at all: %s", rawBody)
	}
	if got != float64(0) {
		t.Errorf("temperature = %v, want 0", got)
	}
}

func TestOpenAIClient_Complete_RetriesOn5xxThenSucceeds(t *testing.T) {
	var calls int32
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		resp := chatCompletionResponse{}
		resp.Choices = append(resp.Choices, testChoice("ok", "stop"))
		_ = json.NewEncoder(w).Encode(resp)
	})

	client := NewOpenAIClient(srv.URL, "", time.Second)
	resp, err := client.Complete(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp.Text != "ok" {
		t.Errorf("Text = %q, want ok", resp.Text)
	}
	// S8: Latency must reflect the successful attempt's own generation
	// time, not the cumulative wait across the two failed attempts plus
	// their backoff delays (which total 250ms+500ms=750ms for attempts 1
	// and 2 - see retryBaseDelay/maxRetries).
	if resp.Latency > 200*time.Millisecond {
		t.Errorf("Latency = %v, want well under the 750ms of retry backoff (per-attempt measurement, S8)", resp.Latency)
	}

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("calls = %d, want 3 (1 initial + 2 retries)", got)
	}
}

func TestOpenAIClient_Complete_ExhaustsRetriesOn5xx(t *testing.T) {
	var calls int32
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	client := NewOpenAIClient(srv.URL, "", time.Second)
	_, err := client.Complete(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "x"}}})
	if err == nil {
		t.Fatal("Complete() error = nil, want error after exhausting retries")
	}
	if got := atomic.LoadInt32(&calls); got != maxRetries+1 {
		t.Errorf("calls = %d, want %d", got, maxRetries+1)
	}
}

func TestOpenAIClient_Complete_NoRetryOn4xx(t *testing.T) {
	var calls int32
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	})

	client := NewOpenAIClient(srv.URL, "", time.Second)
	_, err := client.Complete(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "x"}}})
	if err == nil {
		t.Fatal("Complete() error = nil, want error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (no retry on 4xx)", got)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error = %v, want mention of 400", err)
	}
}

func TestOpenAIClient_Complete_APIErrorField(t *testing.T) {
	var calls int32
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"error":{"message":"model overloaded"}}`))
	})

	client := NewOpenAIClient(srv.URL, "", time.Second)
	_, err := client.Complete(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "x"}}})
	if err == nil {
		t.Fatal("Complete() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "model overloaded") {
		t.Errorf("error = %v, want mention of model overloaded", err)
	}
	if got := atomic.LoadInt32(&calls); got != maxRetries+1 {
		t.Errorf("calls = %d, want %d (2xx-with-error-body is retried like a 5xx)", got, maxRetries+1)
	}
}

func TestOpenAIClient_Complete_RetriesAPIErrorThenSucceeds(t *testing.T) {
	var calls int32
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			_, _ = w.Write([]byte(`{"error":{"message":"replica warming"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	})

	client := NewOpenAIClient(srv.URL, "", time.Second)
	resp, err := client.Complete(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatalf("Complete() error = %v, want nil", err)
	}
	if resp.Text != "ok" {
		t.Errorf("Text = %q, want %q", resp.Text, "ok")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2 (one error body, one success)", got)
	}
}

func TestOpenAIClient_Complete_RetriesOn429ThenSucceeds(t *testing.T) {
	var calls int32
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	})

	client := NewOpenAIClient(srv.URL, "", time.Second)
	resp, err := client.Complete(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatalf("Complete() error = %v, want nil", err)
	}
	if resp.Text != "ok" {
		t.Errorf("Text = %q, want %q", resp.Text, "ok")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2 (one 429, one success)", got)
	}
}

func TestOpenAIClient_Complete_ContextCanceled(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := NewOpenAIClient(srv.URL, "", time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Complete(ctx, Request{Model: "m", Messages: []Message{{Role: "user", Content: "x"}}})
	if err == nil {
		t.Fatal("Complete() error = nil, want error for canceled context")
	}
}

func TestOpenAIClient_Complete_SendsAPIKey(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-key" {
			t.Errorf("Authorization = %q, want Bearer secret-key", got)
		}
		_ = json.NewEncoder(w).Encode(chatCompletionResponse{})
	})

	client := NewOpenAIClient(srv.URL, "secret-key", time.Second)
	_, err := client.Complete(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
}

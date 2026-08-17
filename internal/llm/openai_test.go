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
		resp.Choices = []struct {
			Message chatMsg `json:"message"`
		}{{Message: chatMsg{Role: "assistant", Content: "hello there"}}}
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
	if resp.PromptTokens != 12 || resp.CompletionTokens != 3 {
		t.Errorf("tokens = %d/%d, want 12/3", resp.PromptTokens, resp.CompletionTokens)
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
		resp.Choices = []struct {
			Message chatMsg `json:"message"`
		}{{Message: chatMsg{Role: "assistant", Content: "ok"}}}
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
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
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

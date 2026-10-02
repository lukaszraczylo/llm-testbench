package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sseServer(t *testing.T, frames func(w http.ResponseWriter, fl http.Flusher)) string {
	t.Helper()
	return newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		frames(w, fl)
	}).URL
}

func streamClient(url string, idle time.Duration) *OpenAIClient {
	c := fastClient(url).WithStreaming(true, idle)
	return c
}

func TestStream_AggregatesContentUsageAndFingerprint(t *testing.T) {
	var body map[string]any
	url := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		send(w, ": keepalive\n\n")
		send(w, `data: {"model":"srv","system_fingerprint":"fp","choices":[{"delta":{"reasoning_content":"thinking"}}]}`+"\n\n")
		send(w, `data: {"choices":[{"delta":{"content":"Hel"}}]}`+"\n\n")
		send(w, `data: {"choices":[{"delta":{"content":"lo"},"finish_reason":"stop"}]}`+"\n\n")
		send(w, `data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":7}}`+"\n\n")
		send(w, "data: [DONE]\n\n")
	}).URL
	resp, err := streamClient(url, time.Second).Complete(context.Background(), oneMsg)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp.Text != "Hello" || resp.FinishReason != "stop" || resp.PromptTokens != 5 || resp.CompletionTokens != 7 {
		t.Errorf("resp = %+v", resp)
	}
	if resp.Fingerprint != "fp" || resp.ServedModel != "srv" {
		t.Errorf("fingerprint/model = %q/%q", resp.Fingerprint, resp.ServedModel)
	}
	if body["stream"] != true {
		t.Errorf("stream = %v, want true", body["stream"])
	}
	if opts, _ := body["stream_options"].(map[string]any); opts["include_usage"] != true {
		t.Errorf("stream_options = %v, want include_usage", body["stream_options"])
	}
}

func TestStream_AssemblesToolCallFragments(t *testing.T) {
	url := sseServer(t, func(w http.ResponseWriter, fl http.Flusher) {
		send(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"get_w","arguments":"{\"ci"}}]}}]}`+"\n\n")
		send(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ty\":\"X\"}"}},{"index":1,"function":{"name":"b","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
		send(w, "data: [DONE]\n\n")
	})
	resp, err := streamClient(url, time.Second).Complete(context.Background(), oneMsg)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if len(resp.ToolCalls) != 2 || resp.ToolCalls[0].Name != "get_w" || resp.ToolCalls[0].Arguments["city"] != "X" || !resp.ToolCalls[1].Decoded {
		t.Errorf("ToolCalls = %+v", resp.ToolCalls)
	}
	if resp.Text != "" {
		t.Errorf("Text = %q, want empty", resp.Text)
	}
}

func TestStream_ReasoningOnlyTruncationYieldsEmptyText(t *testing.T) {
	url := sseServer(t, func(w http.ResponseWriter, fl http.Flusher) {
		send(w, `data: {"choices":[{"delta":{"reasoning_content":"..."},"finish_reason":"length"}]}`+"\n\n")
		send(w, "data: [DONE]\n\n")
	})
	resp, err := streamClient(url, time.Second).Complete(context.Background(), oneMsg)
	if err != nil || resp.Text != "" || resp.FinishReason != FinishReasonLength {
		t.Fatalf("resp = %+v, err = %v; want empty text, length", resp, err)
	}
}

func TestStream_TruncatedStreamIsRetried(t *testing.T) {
	var calls int32
	url := sseServer(t, func(w http.ResponseWriter, fl http.Flusher) {
		if atomic.AddInt32(&calls, 1) == 1 {
			send(w, `data: {"choices":[{"delta":{"content":"par"}}]}`+"\n\n")
			return // connection ends with no [DONE] or finish_reason
		}
		send(w, `data: {"choices":[{"delta":{"content":"whole"},"finish_reason":"stop"}]}`+"\n\n")
		send(w, "data: [DONE]\n\n")
	})
	resp, err := streamClient(url, time.Second).Complete(context.Background(), oneMsg)
	if err != nil || resp.Text != "whole" {
		t.Fatalf("resp = %q, err = %v; want whole", resp.Text, err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2", got)
	}
}

func TestStream_IdleTimeoutAbandonsHungServer(t *testing.T) {
	var calls int32
	url := sseServer(t, func(w http.ResponseWriter, fl http.Flusher) {
		atomic.AddInt32(&calls, 1)
		send(w, `data: {"choices":[{"delta":{"content":"x"}}]}`+"\n\n")
		fl.Flush()
		time.Sleep(2 * time.Second) // goes silent
	})
	c := streamClient(url, 150*time.Millisecond).WithRetryTimeouts(false)
	start := time.Now()
	_, err := c.Complete(context.Background(), oneMsg)
	if err == nil || !strings.Contains(err.Error(), "not retried") {
		t.Fatalf("err = %v, want idle timeout not retried", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %v, want abandoned near the 150ms idle timeout", elapsed)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
}

func TestStream_SlowButProgressingSurvivesPastIdleTimeout(t *testing.T) {
	url := sseServer(t, func(w http.ResponseWriter, fl http.Flusher) {
		for i := 0; i < 6; i++ {
			send(w, `data: {"choices":[{"delta":{"content":"a"}}]}`+"\n\n")
			fl.Flush()
			time.Sleep(80 * time.Millisecond)
		}
		send(w, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	})
	// total ~480ms > idle 200ms, but never silent for 200ms.
	resp, err := streamClient(url, 200*time.Millisecond).Complete(context.Background(), oneMsg)
	if err != nil || resp.Text != "aaaaaa" {
		t.Fatalf("resp = %q, err = %v; want aaaaaa", resp.Text, err)
	}
}

func TestStream_ErrorChunkIsRetried(t *testing.T) {
	var calls int32
	url := sseServer(t, func(w http.ResponseWriter, fl http.Flusher) {
		if atomic.AddInt32(&calls, 1) == 1 {
			send(w, `data: {"error":{"message":"replica warming"}}`+"\n\n")
			return
		}
		send(w, `data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	})
	resp, err := streamClient(url, time.Second).Complete(context.Background(), oneMsg)
	if err != nil || resp.Text != "ok" {
		t.Fatalf("resp = %q, err = %v", resp.Text, err)
	}
}

func TestStream_PlainJSONFallbackStillParsed(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, "plain")
	})
	resp, err := streamClient(srv.URL, time.Second).Complete(context.Background(), oneMsg)
	if err != nil || resp.Text != "plain" {
		t.Fatalf("resp = %q, err = %v; want plain JSON body accepted", resp.Text, err)
	}
}

func send(w io.Writer, s string) { _, _ = io.WriteString(w, s) }

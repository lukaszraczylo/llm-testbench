package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// defaultIdleTimeout is how long a stream may stay silent (no bytes at all,
// including before the first token) before the attempt is abandoned. It
// must exceed a slow local model's prompt-processing time.
const defaultIdleTimeout = 5 * time.Minute

// maxStreamLine bounds one SSE line.
const maxStreamLine = 1 << 20

const (
	sseDataPrefix = "data:"
	sseDone       = "[DONE]"
)

// errIdleTimeout marks an attempt abandoned because the stream went silent.
var errIdleTimeout = errors.New("llm: stream idle timeout")

// errStreamTruncated marks a stream that ended without [DONE] or a
// finish_reason, so the answer may be incomplete.
var errStreamTruncated = errors.New("llm: stream ended before completion")

type streamToolCall struct {
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
	Index int `json:"index"`
}

type streamChunk struct {
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Model             string `json:"model"`
	SystemFingerprint string `json:"system_fingerprint"`
	Choices           []struct {
		FinishReason *string `json:"finish_reason"`
		Delta        struct {
			Content   *string          `json:"content"`
			ToolCalls []streamToolCall `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

// readStream folds an OpenAI-style SSE stream into the same response shape
// the non-streaming path decodes. onActivity is called for every line read,
// including keep-alive comments, so the caller can reset its idle timer.
// reasoning_content deltas count as activity but are never accumulated,
// matching the non-streaming path.
func readStream(r io.Reader, onActivity func()) (*chatCompletionResponse, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxStreamLine)

	var (
		out      chatCompletionResponse
		text     strings.Builder
		sawText  bool
		finish   string
		done     bool
		order    []int
		calls    = map[int]*apiToolCall{}
		tooLarge bool
	)

	for sc.Scan() {
		onActivity()
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, sseDataPrefix) {
			continue // blank separator, comment or other SSE field
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, sseDataPrefix))
		if data == sseDone {
			done = true
			break
		}
		var ch streamChunk
		if err := json.Unmarshal([]byte(data), &ch); err != nil {
			return nil, &retryableError{err: fmt.Errorf("decode stream chunk: %w", err)}
		}
		if ch.Error != nil {
			out.Error = ch.Error
			return &out, nil
		}
		if ch.Model != "" {
			out.Model = ch.Model
		}
		if ch.SystemFingerprint != "" {
			out.SystemFingerprint = ch.SystemFingerprint
		}
		if ch.Usage != nil {
			out.Usage.PromptTokens = ch.Usage.PromptTokens
			out.Usage.CompletionTokens = ch.Usage.CompletionTokens
		}
		if len(ch.Choices) == 0 {
			continue
		}
		c := ch.Choices[0]
		if c.Delta.Content != nil {
			sawText = true
			if text.Len()+len(*c.Delta.Content) > maxResponseBytes {
				tooLarge = true
			} else {
				text.WriteString(*c.Delta.Content)
			}
		}
		for _, tc := range c.Delta.ToolCalls {
			acc, ok := calls[tc.Index]
			if !ok {
				acc = &apiToolCall{Type: "function"}
				calls[tc.Index] = acc
				order = append(order, tc.Index)
			}
			if tc.Function.Name != "" {
				acc.Function.Name = tc.Function.Name
			}
			acc.Function.Arguments += tc.Function.Arguments
		}
		if c.FinishReason != nil && *c.FinishReason != "" {
			finish = *c.FinishReason
		}
	}
	if err := sc.Err(); err != nil {
		return nil, &retryableError{err: fmt.Errorf("read stream: %w", err)}
	}
	if tooLarge {
		return nil, fmt.Errorf("llm: streamed response exceeded %d bytes", maxResponseBytes)
	}
	if !done && finish == "" {
		return nil, &retryableError{err: errStreamTruncated}
	}

	msg := responseMsg{Role: "assistant"}
	if sawText {
		s := text.String()
		msg.Content = &s
	}
	for _, idx := range order {
		msg.ToolCalls = append(msg.ToolCalls, *calls[idx])
	}
	out.Choices = append(out.Choices, struct {
		FinishReason string      `json:"finish_reason"`
		Message      responseMsg `json:"message"`
	}{FinishReason: finish, Message: msg})
	return &out, nil
}

// idleWatchdog cancels an attempt when no activity is seen for idle.
type idleWatchdog struct {
	timer *time.Timer
	idle  time.Duration
}

func startIdleWatchdog(cancel context.CancelCauseFunc, idle time.Duration) *idleWatchdog {
	return &idleWatchdog{idle: idle, timer: time.AfterFunc(idle, func() { cancel(errIdleTimeout) })}
}

func (w *idleWatchdog) Touch() { w.timer.Reset(w.idle) }
func (w *idleWatchdog) Stop()  { w.timer.Stop() }

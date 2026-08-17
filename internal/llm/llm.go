// Package llm defines the OpenAI-compatible chat client interface used to
// exercise language models under test, plus a concrete HTTP implementation.
package llm

import (
	"context"
	"time"
)

// Message is a single chat message in an OpenAI-compatible conversation.
type Message struct {
	Role    string
	Content string
}

// Request is one chat-completion request sent to a model.
type Request struct {
	Model       string
	Messages    []Message
	MaxTokens   int
	Temperature float64
}

// Response is the normalized result of a chat-completion call.
type Response struct {
	Text             string
	PromptTokens     int
	CompletionTokens int
	Latency          time.Duration
}

// Client completes chat requests against a language model backend.
// Implementations must be safe for concurrent use.
type Client interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

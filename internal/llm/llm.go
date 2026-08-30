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

// Tool is one function the model may call, in OpenAI function-calling
// shape. Parameters is a JSON Schema object describing the arguments.
type Tool struct {
	Parameters  map[string]any
	Name        string
	Description string
}

// ToolCall is one function invocation the model emitted. Arguments is the
// decoded argument object (OpenAI sends it as a JSON-encoded string on the
// wire; the client decodes it before returning). Decoded is false when the
// model produced malformed argument JSON, in which case RawArguments holds
// the undecodable string for scoring.
type ToolCall struct {
	Arguments    map[string]any
	Name         string
	RawArguments string
	Decoded      bool
}

// Request is one chat-completion request sent to a model. When Tools is
// non-empty the client advertises them with tool_choice "auto", so the
// model decides whether and which to call.
type Request struct {
	Model       string
	Messages    []Message
	Tools       []Tool
	MaxTokens   int
	Temperature float64
}

// Response is the normalized result of a chat-completion call.
type Response struct {
	Text             string
	FinishReason     string
	ToolCalls        []ToolCall
	PromptTokens     int
	CompletionTokens int
	Latency          time.Duration
}

// FinishReasonLength is the OpenAI-compatible finish_reason value meaning
// the model hit its token budget (MaxTokens) before it finished: the
// response, and Text in particular, may be truncated or empty. A reasoning
// model that spends its whole budget on internal reasoning before writing
// anything to the answer channel reports this with Text == "".
const FinishReasonLength = "length"

// Client completes chat requests against a language model backend.
// Implementations must be safe for concurrent use.
type Client interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

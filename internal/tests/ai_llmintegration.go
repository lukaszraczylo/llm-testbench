package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerAILLMIntegrationTests(r *testkit.Registry) {
	r.Register(llmFieldSemanticsTest())
	r.Register(llmTokenBudgetReasoningTest())
	r.Register(llm429RetryBackoffTest())
	r.Register(llmContextOverflowStrategyTest())
	r.Register(llmToolCallHandlingTest())
	r.Register(llmRolePlacementTest())
	r.Register(llmTemperatureZeroCaveatTest())
	r.Register(llmSSEStreamDoneTest())
	r.Register(llmEmbeddingBatchMathTest())
	r.Register(llmStopSequenceTraceTest())
}

// aiCeilDiv returns ceil(total/batchSize) for positive total and
// batchSize: the number of fixed-size batches needed to cover total items,
// with a final partial batch counted as one whole batch.
func aiCeilDiv(total, batchSize int) int {
	return (total + batchSize - 1) / batchSize
}

// llmFieldSemanticsTest: match three OpenAI-compatible request fields
// (temperature, max_tokens, stop) to their actual behavior, from an
// enumerated multiple-choice roster given inline so the answer is
// determined by the definitions in the prompt, not by memorized field
// names.
//
// ground truth: temperature scales the randomness of token sampling from
// the model's output distribution; max_tokens is a hard cap on how many
// completion tokens the response may contain; stop is a list of strings
// that, once generated, immediately end generation (the stop string itself
// is excluded from the returned text).
func llmFieldSemanticsTest() testkit.Test {
	prompt := `An OpenAI-compatible chat completions request can set these
fields: temperature, max_tokens, stop. For each field, pick the one option
id below that correctly describes what it controls.

temperature options:
- sampling_randomness: how much randomness is applied when sampling the
  next token from the model's output probability distribution.
- reply_length_cap: a hard cap on how many tokens the reply may contain.
- output_format: whether the reply is returned as JSON or plain text.

max_tokens options:
- completion_token_cap: a hard cap on how many completion tokens the reply
  may contain; generation stops once this many tokens have been produced.
- minimum_reply_length: the minimum number of tokens the reply must
  contain before it can end.
- request_timeout: how many seconds the client waits before giving up on
  the request.

stop options:
- generation_end_strings: a list of strings that, once generated,
  immediately end generation; the stop string itself is excluded from the
  returned text.
- required_reply_prefix: a list of strings the reply must begin with.
- system_override: a list of strings that replace the system prompt.

Respond with only a JSON object mapping each field to its correct option
id: {"temperature":"...","max_tokens":"...","stop":"..."}`

	evaluator := eval.Mean(
		eval.JSONField("temperature", "sampling_randomness"),
		eval.JSONField("max_tokens", "completion_token_cap"),
		eval.JSONField("stop", "generation_end_strings"),
	)

	return testkit.Test{
		ID:          "llm-field-semantics",
		Category:    "ai",
		Subcategory: "llm-integration",
		Description: "Match temperature/max_tokens/stop to their correct behavior from an inline multiple-choice roster.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// llmTokenBudgetReasoningWant is derived by plain subtraction, matching the
// arithmetic ai_llmintegration_test.go independently recomputes.
//
// ground truth: this framework's own PLAN.md documents a live-run
// incident where a reasoning model's hidden <think>...</think> content
// consumed most of a request's max_tokens budget before any visible answer
// was produced, truncating the visible answer to empty - which is why
// MaxTokens defaults to 0 (provider default) for new catalog tests rather
// than a small fixed cap. Here: max_tokens = 500, and the model's <think>
// block consumes 437 completion tokens before the visible answer begins.
// Tokens remaining for the visible answer = 500 - 437 = 63.
const llmTokenBudgetReasoningWant = 500 - 437

func llmTokenBudgetReasoningTest() testkit.Test {
	prompt := `An OpenAI-compatible chat completion request sets
max_tokens = 500. This caps the total number of completion tokens the
response may contain, counting every token the model generates: a
reasoning model's hidden <think>...</think> content counts against this
same budget before its visible answer, since both are completion tokens.

For this request, the model's <think>...</think> block consumes exactly
437 completion tokens before it begins writing its visible answer.

How many completion tokens remain in the max_tokens budget for the visible
answer, once the <think> block has been generated? Respond with only the
number.`

	return testkit.Test{
		ID:          "llm-token-budget-reasoning",
		Category:    "ai",
		Subcategory: "llm-integration",
		Description: "Compute completion tokens remaining for a visible answer after a reasoning model's <think> block consumes part of max_tokens.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], llmTokenBudgetReasoningWant, 0),
	}
}

// llm429RetryBackoffTest: decide whether to retry an HTTP 429 response and
// for how long to wait, given an explicit Retry-After header value.
//
// ground truth: HTTP 429 (Too Many Requests) signals a transient,
// rate-limiting condition, not a permanent failure of the request itself -
// the correct client behavior is to retry rather than give up immediately.
// When the server supplies a Retry-After header, that value is the
// authoritative wait time the server itself is asking for; the client
// should honor it exactly rather than substitute its own default backoff
// duration.
func llm429RetryBackoffTest() testkit.Test {
	prompt := `An OpenAI-compatible chat completions client sends a request
and receives this response:

HTTP/1.1 429 Too Many Requests
Retry-After: 2

This is the first attempt at a request that is otherwise valid and
retryable. Should the client retry this request, and if so, how many
seconds should it wait before retrying (using the server's own guidance
rather than an arbitrary default)? Respond with only a JSON object:
{"should_retry":"yes"|"no","wait_seconds":<integer>}`

	evaluator := eval.Mean(
		eval.JSONField("should_retry", "yes"),
		eval.JSONField("wait_seconds", 2),
	)

	return testkit.Test{
		ID:          "llm-429-retry-backoff",
		Category:    "ai",
		Subcategory: "llm-integration",
		Description: "Decide to retry an HTTP 429 and honor its exact Retry-After: 2 header value rather than an arbitrary default.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// llmContextOverflowStrategyTest: pick the only context-overflow strategy
// that satisfies a stated "no altering any turn's text" constraint.
//
// ground truth: total required content is 500 (system) + 300 (new
// message) + 1800 (2 most recent turn pairs, 900 tokens each, which must
// stay verbatim) + 7200 (8 older turn pairs, 900 tokens each) = 10100
// tokens, against an 8000-token window - an overflow of 2100 tokens.
// Protected content (system, new message, 2 most recent pairs) totals
// 2600 tokens, leaving 5400 tokens for the 7200 tokens of older turns: an
// overflow of 1800 tokens among only the older turns. The stated
// constraint that no text anywhere may be paraphrased or summarized rules
// out summarizing older turns (summarizing alters their text) and the
// system prompt is explicitly protected, so the only strategy that fits
// the window without altering any retained turn's text is dropping whole
// older turns until the remainder fits.
func llmContextOverflowStrategyTest() testkit.Test {
	prompt := `A chat client's context window holds 8000 tokens total.
Current conversation content:
- system prompt: 500 tokens, must remain byte-for-byte unaltered.
- 10 prior user/assistant turn pairs, 900 tokens each (9000 tokens total).
  The 2 most recent of these 10 pairs must remain completely unaltered,
  verbatim (1800 of the 9000 tokens).
- new user message: 300 tokens, must be included as-is.

Hard constraint: no text anywhere in the assembled context may be
paraphrased, summarized, or otherwise altered - every turn that is
included must appear exactly as it was originally written.

Total required content exceeds the 8000-token window. Given the hard
constraint above, which strategy fits: drop-oldest-turns (remove whole
older turn pairs entirely until it fits), summarize-oldest-turns
(compress older turns into shorter summaries), or truncate-system-prompt
(shorten the system prompt)? Respond with only a JSON object:
{"strategy":"drop-oldest-turns"|"summarize-oldest-turns"|"truncate-system-prompt"}`

	return testkit.Test{
		ID:          "llm-context-overflow-strategy",
		Category:    "ai",
		Subcategory: "llm-integration",
		Description: "Pick drop-oldest-turns as the only context-overflow strategy that satisfies a no-altered-text constraint.",
		Prompt:      prompt,
		Eval:        eval.JSONField("strategy", "drop-oldest-turns"),
	}
}

// llmToolCallHandlingTest: trace OpenAI-compatible tool-call handling -
// the message role and id field used to return a tool's result.
//
// ground truth: when an assistant message includes a tool_calls array,
// each entry carries its own id. The result of executing that call is
// returned as a new message with role "tool", whose tool_call_id field
// must equal the originating tool_calls entry's id so the model can match
// the result back to the specific call it made.
func llmToolCallHandlingTest() testkit.Test {
	prompt := `In the OpenAI-compatible chat completions tool-calling
protocol, the assistant's message can include a tool_calls array; each
entry has its own id and a function name/arguments to execute.

Once your code has executed the requested function and has its result,
which message role must the next message use to feed that result back
into the conversation, and which field on that new message must be set to
the originating tool_calls entry's id so the model can match the result to
the call it made? Respond with only a JSON object:
{"next_role":"...","id_field":"..."}`

	evaluator := eval.Mean(
		eval.JSONField("next_role", "tool"),
		eval.JSONField("id_field", "tool_call_id"),
	)

	return testkit.Test{
		ID:          "llm-tool-call-handling",
		Category:    "ai",
		Subcategory: "llm-integration",
		Description: "Trace OpenAI-compatible tool-call result handling: role=tool, id field=tool_call_id.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// llmRolePlacementTest: identify the system role as the correct carrier
// for a fixed, non-negotiable operating instruction.
func llmRolePlacementTest() testkit.Test {
	prompt := `In an OpenAI-compatible chat completions request, which
message role should carry a fixed operating instruction that must apply to
every request regardless of what the user's own message says (for
example, "never reveal API keys in your reply"): system or user? Respond
with only one word.`

	return testkit.Test{
		ID:          "llm-role-placement",
		Category:    "ai",
		Subcategory: "llm-integration",
		Description: "Identify system (not user) as the role that carries a fixed, non-negotiable operating instruction.",
		Prompt:      prompt,
		Eval:        eval.Equals("system"),
	}
}

// llmTemperatureZeroCaveatTest: explain why temperature=0 is not a full
// bit-for-bit determinism guarantee.
//
// ground truth: temperature=0 makes token selection greedy (always the
// highest-probability token) rather than random, but it does not by
// itself guarantee bit-identical output across repeated calls. Floating-
// point arithmetic is not strictly associative, and provider-side
// execution details - batching requests together, and which GPU/hardware
// kernel path a given batch is routed through - can change the order
// operations are summed in, producing tiny floating-point differences
// that occasionally flip which token has the (near-)highest probability.
func llmTemperatureZeroCaveatTest() testkit.Test {
	prompt := `A test suite calls an OpenAI-compatible chat completions API
with temperature=0 on every request, expecting bit-for-bit identical
output across repeated calls with the identical prompt. In practice,
repeated calls can still occasionally return slightly different text.

Explain why temperature=0 does not fully guarantee deterministic,
bit-for-bit identical output: name the numerical-computation reason and
the provider-side execution reason that both contribute.`

	evaluator := eval.Mean(
		eval.ContainsAny("floating point", "floating-point", "floating-point precision", "numerical precision"),
		eval.ContainsAny("batching", "batch", "hardware", "gpu", "kernel", "parallel execution"),
	)

	return testkit.Test{
		ID:          "llm-temperature-zero-caveat",
		Category:    "ai",
		Subcategory: "llm-integration",
		Description: "Explain why temperature=0 does not fully guarantee bit-identical output (floating-point non-associativity plus provider-side batching/hardware).",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// llmSSEStreamDoneTest: recall the exact text that closes an
// OpenAI-compatible streaming response.
func llmSSEStreamDoneTest() testkit.Test {
	prompt := `An OpenAI-compatible chat completions request streams its
response as Server-Sent Events: a sequence of lines of the form
"data: <json-chunk>", each followed by a blank line, one JSON chunk per
generated token or small group of tokens.

The very last event before the stream closes is also a "data: " line, but
its payload is not a JSON chunk. What exact text follows "data: " in that
final event? Respond with only that exact text, nothing else.`

	return testkit.Test{
		ID:          "llm-sse-stream-done",
		Category:    "ai",
		Subcategory: "llm-integration",
		Description: "Recall the exact [DONE] sentinel text of the final SSE event in an OpenAI-compatible stream.",
		Prompt:      prompt,
		Eval:        eval.Equals("[DONE]"),
	}
}

// llmEmbeddingBatchMathWant is derived by calling aiCeilDiv, not
// hardcoded.
//
// ground truth: ceil(10000/96) - 96*104 = 9984, leaving 16 documents that
// still need one more (partial) request, so 104 full batches plus 1
// partial batch = 105 requests total.
var llmEmbeddingBatchMathWant = aiCeilDiv(10000, 96)

func llmEmbeddingBatchMathTest() testkit.Test {
	prompt := `You must generate embeddings for 10,000 documents. The
embeddings API accepts a maximum of 96 items per request, and every
request you send should be as full as possible (maximally batched) to
minimize the number of requests.

How many API requests are needed in total to embed all 10,000 documents?
Respond with only the number.`

	return testkit.Test{
		ID:          "llm-embedding-batch-math",
		Category:    "ai",
		Subcategory: "llm-integration",
		Description: "Compute the number of maximally-batched embedding requests needed for 10,000 documents at 96 items per request.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], llmEmbeddingBatchMathWant, 0),
	}
}

// llmStopSequenceTraceTest: trace what text an OpenAI-compatible completion
// actually returns when a stop sequence is set - the reply is cut before
// the stop sequence, and the stop sequence itself is excluded.
func llmStopSequenceTraceTest() testkit.Test {
	prompt := `A chat completions request sets stop = ["\n\n"] (generation
ends the instant this exact two-newline sequence would be produced, and
the stop sequence itself is excluded from the returned text).

If the model were generating without any stop sequence, its full raw
output would be exactly:

Answer: 42

Explanation: 6 times 7 is 42.

Given stop = ["\n\n"], what text does the API actually return to the
client? Respond with only that exact returned text, nothing else.`

	return testkit.Test{
		ID:          "llm-stop-sequence-trace",
		Category:    "ai",
		Subcategory: "llm-integration",
		Description: "Trace the exact text an API returns when generation is cut by a stop=[\"\\n\\n\"] sequence, excluding the sequence itself.",
		Prompt:      prompt,
		Eval:        eval.Equals("Answer: 42"),
	}
}

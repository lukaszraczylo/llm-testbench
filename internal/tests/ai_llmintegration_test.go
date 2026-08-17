package tests

import (
	"context"
	"testing"
)

func TestLlmFieldSemanticsTest_Eval(t *testing.T) {
	tc := llmFieldSemanticsTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "all correct",
			response: `{"temperature":"sampling_randomness","max_tokens":"completion_token_cap","stop":"generation_end_strings"}`,
			want:     1,
		},
		{
			name:     "all correct fenced with prose",
			response: "Here is my answer:\n```json\n{\"temperature\":\"sampling_randomness\",\"max_tokens\":\"completion_token_cap\",\"stop\":\"generation_end_strings\"}\n```",
			want:     1,
		},
		{
			name:     "all correct, different spacing",
			response: `{ "temperature": "sampling_randomness", "max_tokens": "completion_token_cap", "stop": "generation_end_strings" }`,
			want:     1,
		},
		{
			name:     "one wrong",
			response: `{"temperature":"output_format","max_tokens":"completion_token_cap","stop":"generation_end_strings"}`,
			want:     2.0 / 3.0,
		},
		{
			name:     "all wrong",
			response: `{"temperature":"reply_length_cap","max_tokens":"minimum_reply_length","stop":"required_reply_prefix"}`,
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestLlmTokenBudgetReasoningWant_GroundTruth(t *testing.T) {
	// Independent re-derivation with plain subtraction.
	const maxTokens, reasoningTokensUsed = 500, 437
	want := maxTokens - reasoningTokensUsed

	if want != 63 {
		t.Fatalf("independently recomputed remaining budget = %d, want 63", want)
	}
	if llmTokenBudgetReasoningWant != want {
		t.Errorf("llmTokenBudgetReasoningWant = %d, independently recomputed = %d", llmTokenBudgetReasoningWant, want)
	}
}

func TestLlmTokenBudgetReasoningTest_Eval(t *testing.T) {
	tc := llmTokenBudgetReasoningTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "63", 1},
		{"prose wrapped", "63 completion tokens remain.", 1},
		{"fenced", "```\n63\n```", 1},
		{"wrong: forgot to subtract", "500", 0},
		{"wrong: reported the reasoning tokens used, not remaining", "437", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestLlm429RetryBackoffTest_Eval(t *testing.T) {
	tc := llm429RetryBackoffTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"all correct", `{"should_retry":"yes","wait_seconds":2}`, 1},
		{"all correct fenced", "```json\n{\"should_retry\":\"yes\",\"wait_seconds\":2}\n```", 1},
		{"all correct, different spacing", `{ "should_retry": "yes", "wait_seconds": 2 }`, 1},
		{"wrong: should not retry", `{"should_retry":"no","wait_seconds":2}`, 0.5},
		{"wrong: ignored Retry-After value", `{"should_retry":"yes","wait_seconds":30}`, 0.5},
		{"both wrong", `{"should_retry":"no","wait_seconds":30}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestLlmContextOverflowStrategyTest_Eval(t *testing.T) {
	tc := llmContextOverflowStrategyTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct: drop-oldest-turns", `{"strategy":"drop-oldest-turns"}`, 1},
		{"correct, fenced with prose", "Given the no-altering constraint:\n```json\n{\"strategy\":\"drop-oldest-turns\"}\n```", 1},
		{"correct, different spacing", `{ "strategy": "drop-oldest-turns" }`, 1},
		{"wrong: summarize-oldest-turns", `{"strategy":"summarize-oldest-turns"}`, 0},
		{"wrong: truncate-system-prompt", `{"strategy":"truncate-system-prompt"}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestLlmToolCallHandlingTest_Eval(t *testing.T) {
	tc := llmToolCallHandlingTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"all correct", `{"next_role":"tool","id_field":"tool_call_id"}`, 1},
		{"all correct fenced", "```json\n{\"next_role\":\"tool\",\"id_field\":\"tool_call_id\"}\n```", 1},
		{"all correct, different spacing", `{ "next_role": "tool", "id_field": "tool_call_id" }`, 1},
		{"wrong role", `{"next_role":"assistant","id_field":"tool_call_id"}`, 0.5},
		{"wrong field", `{"next_role":"tool","id_field":"id"}`, 0.5},
		{"both wrong", `{"next_role":"user","id_field":"call_id"}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestLlmRolePlacementTest_Eval(t *testing.T) {
	tc := llmRolePlacementTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct: system", "system", 1},
		{"correct, different case", "System", 1},
		{"correct, quoted", `"system"`, 1},
		{"wrong: user", "user", 0},
		{"wrong: assistant", "assistant", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestLlmTemperatureZeroCaveatTest_Eval(t *testing.T) {
	tc := llmTemperatureZeroCaveatTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "both reasons present",
			response: "Floating-point arithmetic is not strictly associative, and provider-side batching or GPU kernel routing changes summation order, which can flip the top token.",
			want:     1,
		},
		{
			name:     "both reasons present, alternate phrasing",
			response: "Numerical precision in floating-point math is not associative, and the hardware kernel path a batched request is routed through can differ between calls.",
			want:     1,
		},
		{
			name:     "both reasons present, terse phrasing",
			response: "Floating-point precision issues plus parallel execution batching on the GPU.",
			want:     1,
		},
		{
			name:     "only the numerical reason",
			response: "It is because of floating point rounding differences.",
			want:     0.5,
		},
		{
			name:     "only the execution reason",
			response: "It is because requests get batched together on different hardware.",
			want:     0.5,
		},
		{
			name:     "neither reason",
			response: "The model just has some inherent randomness left even at temperature zero.",
			want:     0,
		},
		{
			name:     "neither reason, wrong explanation entirely",
			response: "It happens because the random seed is not fixed across requests.",
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestLlmSSEStreamDoneTest_Eval(t *testing.T) {
	tc := llmSSEStreamDoneTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "[DONE]", 1},
		{"correct with whitespace", "  [DONE]  ", 1},
		{"correct, case-insensitive", "[done]", 1},
		{"wrong: missing brackets", "DONE", 0},
		{"wrong: describes it instead", "the stream just closes", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestLlmEmbeddingBatchMathWant_GroundTruth(t *testing.T) {
	// Independent re-derivation with plain ceiling-division arithmetic,
	// not via aiCeilDiv.
	const total, batchSize = 10000, 96
	full := total / batchSize
	want := full
	if total%batchSize != 0 {
		want = full + 1
	}

	if want != 105 {
		t.Fatalf("independently recomputed request count = %d, want 105", want)
	}
	if llmEmbeddingBatchMathWant != want {
		t.Errorf("llmEmbeddingBatchMathWant = %d, independently recomputed = %d", llmEmbeddingBatchMathWant, want)
	}
}

func TestLlmEmbeddingBatchMathTest_Eval(t *testing.T) {
	tc := llmEmbeddingBatchMathTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "105", 1},
		{"prose wrapped", "You need 105 requests.", 1},
		{"fenced", "```\n105\n```", 1},
		{"wrong: dropped the partial batch", "104", 0},
		{"wrong: off by a lot", "96", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestLlmStopSequenceTraceTest_Eval(t *testing.T) {
	tc := llmStopSequenceTraceTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "Answer: 42", 1},
		{"correct with whitespace", "  Answer: 42  ", 1},
		{"correct, quoted", `"Answer: 42"`, 1},
		{"wrong: included text past the stop sequence", "Answer: 42\n\nExplanation: 6 times 7 is 42.", 0},
		{"wrong: paraphrased instead of quoting exactly", "The answer is 42", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

package tests

import (
	"context"
	"testing"
)

func TestRagChunkSizeTradeoffTest_Eval(t *testing.T) {
	tc := ragChunkSizeTradeoffTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct: 512", `{"chunk_size_tokens":512}`, 1},
		{"correct, fenced with prose", "The smallest viable option is:\n```json\n{\"chunk_size_tokens\":512}\n```", 1},
		{"wrong: too small, would split a fact", `{"chunk_size_tokens":128}`, 0},
		{"wrong: larger than needed, more dilution", `{"chunk_size_tokens":2048}`, 0},
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

func TestRagRerankerPlacementTest_Eval(t *testing.T) {
	tc := ragRerankerPlacementTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct order", `["retrieve","rerank","generate"]`, 1},
		{"correct order fenced", "```json\n[\"retrieve\",\"rerank\",\"generate\"]\n```", 1},
		{"correct order different case", `["Retrieve","Rerank","Generate"]`, 1},
		{"reranker before retrieval", `["rerank","retrieve","generate"]`, 0},
		{"generate before rerank", `["retrieve","generate","rerank"]`, 0},
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

func TestRagRetrievalFailureModeTest_Eval(t *testing.T) {
	tc := ragRetrievalFailureModeTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct: keyword", "keyword", 1},
		{"correct, different case", "Keyword", 1},
		{"wrong: semantic", "semantic", 0},
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

func TestRagCitationGroundingTest_Eval(t *testing.T) {
	tc := ragCitationGroundingTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "both requirements present",
			response: "Every claim must be traceable to a cited source in the retrieved context; if the context does not contain the answer, the system must say it cannot answer rather than guess.",
			want:     1,
		},
		{
			name:     "only the citation requirement",
			response: "Every claim in the answer must include a citation back to the retrieved context.",
			want:     0.5,
		},
		{
			name:     "only the decline requirement",
			response: "If the retrieved context does not contain the answer, the system must say it is unable to find it.",
			want:     0.5,
		},
		{
			name:     "neither requirement",
			response: "The answer should just sound confident and helpful.",
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

func TestRagContextSelectionVsStuffingTest_Eval(t *testing.T) {
	tc := ragContextSelectionVsStuffingTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct: selective", "selective", 1},
		{"correct, different case", "Selective", 1},
		{"wrong: stuffed", "stuffed", 0},
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

func TestRagIndexStalenessTest_Eval(t *testing.T) {
	tc := ragIndexStalenessTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "all correct",
			response: `{"problem":"stale-chunks-coexist","fix":"delete-old-chunks-before-reinsert"}`,
			want:     1,
		},
		{
			name:     "all correct fenced",
			response: "```json\n{\"problem\":\"stale-chunks-coexist\",\"fix\":\"delete-old-chunks-before-reinsert\"}\n```",
			want:     1,
		},
		{
			name:     "wrong problem",
			response: `{"problem":"embedding-model-drift","fix":"delete-old-chunks-before-reinsert"}`,
			want:     0.5,
		},
		{
			name:     "wrong fix",
			response: `{"problem":"stale-chunks-coexist","fix":"increase-chunk-overlap"}`,
			want:     0.5,
		},
		{
			name:     "both wrong",
			response: `{"problem":"query-cache-stale","fix":"raise-top-k"}`,
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

func TestRagEvalMetricChoiceTest_Eval(t *testing.T) {
	tc := ragEvalMetricChoiceTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "all correct",
			response: `{"scenario_a":"context_precision","scenario_b":"faithfulness"}`,
			want:     1,
		},
		{
			name:     "all correct fenced",
			response: "```json\n{\"scenario_a\":\"context_precision\",\"scenario_b\":\"faithfulness\"}\n```",
			want:     1,
		},
		{
			name:     "scenario_a wrong",
			response: `{"scenario_a":"context_recall","scenario_b":"faithfulness"}`,
			want:     0.5,
		},
		{
			name:     "scenario_b wrong",
			response: `{"scenario_a":"context_precision","scenario_b":"context_recall"}`,
			want:     0.5,
		},
		{
			name:     "both swapped",
			response: `{"scenario_a":"faithfulness","scenario_b":"context_precision"}`,
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

func TestRagHallucinationMitigationOrderingTest_Eval(t *testing.T) {
	tc := ragHallucinationMitigationOrderingTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct order",
			response: `["step_retrieve","step_instruct","step_generate","step_verify","step_flag"]`,
			want:     1,
		},
		{
			name:     "correct order fenced",
			response: "```json\n[\"step_retrieve\",\"step_instruct\",\"step_generate\",\"step_verify\",\"step_flag\"]\n```",
			want:     1,
		},
		{
			name:     "verify before generate",
			response: `["step_retrieve","step_instruct","step_verify","step_generate","step_flag"]`,
			want:     0,
		},
		{
			name:     "flag before verify",
			response: `["step_retrieve","step_instruct","step_generate","step_flag","step_verify"]`,
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

func TestRagMultihopDecompositionTest_Eval(t *testing.T) {
	tc := ragMultihopDecompositionTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct order, distractor excluded",
			response: `["frag_q","frag_p","frag_s","frag_r"]`,
			want:     1,
		},
		{
			name:     "correct order fenced",
			response: "```json\n[\"frag_q\",\"frag_p\",\"frag_s\",\"frag_r\"]\n```",
			want:     1,
		},
		{
			name:     "distractor incorrectly included",
			response: `["frag_q","frag_p","frag_s","frag_r","frag_x"]`,
			want:     0,
		},
		{
			name:     "hops reordered",
			response: `["frag_p","frag_q","frag_s","frag_r"]`,
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

func TestRagPreAssemblyDedupTest_Eval(t *testing.T) {
	tc := ragPreAssemblyDedupTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "both reasons present",
			response: "Passing all 3 copies wastes context window budget, and the redundant repetition risks biasing the answer toward that one duplicated point.",
			want:     1,
		},
		{
			name:     "only the waste reason",
			response: "It wastes token budget that could hold other useful chunks.",
			want:     0.5,
		},
		{
			name:     "only the redundancy reason",
			response: "It introduces redundant, repetitive information into the context.",
			want:     0.5,
		},
		{
			name:     "neither reason",
			response: "It just looks messy to a human reviewer.",
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

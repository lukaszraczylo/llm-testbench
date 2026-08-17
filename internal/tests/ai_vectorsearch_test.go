package tests

import (
	"context"
	"math"
	"testing"
)

func TestVecCosineVsDotTest_Eval(t *testing.T) {
	// Independent re-derivation: |a| = sqrt(0.6^2+0.8^2), |b| =
	// sqrt(0.8^2+0.6^2), both exactly 1, so cosine similarity's
	// denominator is 1 and the dot product equals the cosine similarity.
	magA := math.Sqrt(0.6*0.6 + 0.8*0.8)
	magB := math.Sqrt(0.8*0.8 + 0.6*0.6)
	if math.Abs(magA-1) > 1e-9 || math.Abs(magB-1) > 1e-9 {
		t.Fatalf("independently recomputed magnitudes = %v, %v, want both 1", magA, magB)
	}

	tc := vecCosineVsDotTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct: yes", "yes", 1},
		{"correct: different case", "Yes", 1},
		{"correct: quoted", `"yes"`, 1},
		// C3: this row's own label was self-contradictory - "correct"
		// paired with want:0. The prompt asks for "only one word: yes or
		// no"; a full sentence is not a bare/quoted/fenced token even
		// though its content is substantively correct, so want:0 was
		// already the right assertion - only the misleading label needed
		// fixing, matching how eval.ExactToken treats sentence-wrapping
		// everywhere else in this codebase (it is not a summarizer).
		{"wrong: substantively correct but sentence-wrapped, not a bare token", "Yes, since both magnitudes are 1.", 0},
		{"wrong: no", "no", 0},
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

func TestVecRecallAtKWant_GroundTruth(t *testing.T) {
	// Independent re-derivation with a plain set intersection, not via
	// wpRecallAtK.
	relevant := map[string]bool{"V3": true, "V8": true, "V11": true, "V14": true, "V20": true}
	retrieved := []string{"V8", "V1", "V14", "V6", "V9"}
	hit := 0
	for _, id := range retrieved {
		if relevant[id] {
			hit++
		}
	}
	want := float64(hit) / float64(len(relevant))

	if want != 0.4 {
		t.Fatalf("independently recomputed recall@5 = %v, want 0.4", want)
	}
	if vecRecallAtKWant != want {
		t.Errorf("vecRecallAtKWant = %v, independently recomputed = %v", vecRecallAtKWant, want)
	}
}

func TestVecRecallAtKTest_Eval(t *testing.T) {
	tc := vecRecallAtKTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "0.4", 1},
		{"prose wrapped", "recall@5 is 0.4.", 1},
		{"trailing period", "0.4.", 1},
		{"wrong: counted an extra hit", "0.6", 0},
		{"wrong: missed a hit", "0.2", 0},
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

func TestVecHNSWEfSearchTradeoffTest_Eval(t *testing.T) {
	tc := vecHNSWEfSearchTradeoffTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct: up", "up", 1},
		{"correct: uppercase", "UP", 1},
		{"correct: trailing period", "up.", 1},
		{"wrong: down", "down", 0},
		{"wrong: unchanged", "unchanged", 0},
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

func TestVecPQMemoryMathWant_GroundTruth(t *testing.T) {
	// Independent re-derivation with plain arithmetic, not via
	// wpQuantizedCodeBytes.
	const m, nbits, n = 16, 4, 1000000
	codeBytes := m * nbits / 8
	want := codeBytes * n

	if want != 8000000 {
		t.Fatalf("independently recomputed total bytes = %d, want 8000000", want)
	}
	if vecPQMemoryMathWant != want {
		t.Errorf("vecPQMemoryMathWant = %d, independently recomputed = %d", vecPQMemoryMathWant, want)
	}
}

func TestVecPQMemoryMathTest_Eval(t *testing.T) {
	tc := vecPQMemoryMathTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "8000000", 1},
		{"prose wrapped", "The total is 8000000 bytes.", 1},
		{"fenced", "```\n8000000\n```", 1},
		{"wrong", "512", 0},
		{"wrong: bit/byte confusion (8x too large)", "64000000", 0},
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

func TestVecPreVsPostFilteringTest_Eval(t *testing.T) {
	tc := vecPreVsPostFilteringTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct: pre", "pre", 1},
		{"correct: uppercase", "PRE", 1},
		{"correct: trailing period", "pre.", 1},
		{"wrong: post", "post", 0},
		{"wrong: both", "both", 0},
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

func TestVecRRFFusionWant_GroundTruth(t *testing.T) {
	// Independent re-derivation with plain arithmetic, not via aiRRFScore.
	const k = 60
	const keywordRank, vectorRank = 3, 7
	want := math.Round((1.0/float64(k+keywordRank)+1.0/float64(k+vectorRank))*10000) / 10000

	if math.Abs(want-0.0308) > 1e-9 {
		t.Fatalf("independently recomputed RRF score = %v, want ~0.0308", want)
	}
	if vecRRFFusionWant != want {
		t.Errorf("vecRRFFusionWant = %v, independently recomputed = %v", vecRRFFusionWant, want)
	}
}

func TestVecRRFFusionTest_Eval(t *testing.T) {
	tc := vecRRFFusionTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "0.0308", 1},
		{"within tolerance", "0.0307", 1},
		{"prose wrapped", "The RRF score for D5 is 0.0308.", 1},
		{"wrong: only counted one list", "0.0159", 0},
		{"wrong: outside tolerance", "0.0328", 0},
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

func TestVecDistanceToSimilarityWant_GroundTruth(t *testing.T) {
	// Independent re-derivation with plain arithmetic, not via
	// aiUnitDistanceToCosineSimilarity.
	const d = 0.6
	want := math.Round((1-d*d/2)*10000) / 10000

	if math.Abs(want-0.82) > 1e-9 {
		t.Fatalf("independently recomputed cosine similarity = %v, want 0.82", want)
	}
	if vecDistanceToSimilarityWant != want {
		t.Errorf("vecDistanceToSimilarityWant = %v, independently recomputed = %v", vecDistanceToSimilarityWant, want)
	}
}

func TestVecDistanceToSimilarityTest_Eval(t *testing.T) {
	tc := vecDistanceToSimilarityTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "0.82", 1},
		{"prose wrapped", "cos_sim(a,b) = 0.82", 1},
		{"fenced", "```\n0.82\n```", 1},
		{"wrong: used d not d^2", "0.7", 0},
		{"wrong: forgot to subtract from 1", "0.18", 0},
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

func TestVecNearDuplicateThresholdWant_GroundTruth(t *testing.T) {
	// Independent re-derivation with plain arithmetic, not via
	// cosineSimilarity.
	dot := 1.0*2 + 2.0*2 + 2.0*1 + 1.0*1
	normA := math.Sqrt(1*1 + 2*2 + 2*2 + 1*1)
	normB := math.Sqrt(2*2 + 2*2 + 1*1 + 1*1)
	cos := dot / (normA * normB)
	want := math.Round((cos-0.97)*10000) / 10000

	if math.Abs(want-(-0.07)) > 1e-9 {
		t.Fatalf("independently recomputed difference = %v, want -0.07", want)
	}
	if vecNearDuplicateThresholdWant != want {
		t.Errorf("vecNearDuplicateThresholdWant = %v, independently recomputed = %v", vecNearDuplicateThresholdWant, want)
	}
}

func TestVecNearDuplicateThresholdTest_Eval(t *testing.T) {
	tc := vecNearDuplicateThresholdTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "-0.07", 1},
		{"prose wrapped", "similarity minus threshold is -0.07.", 1},
		{"fenced", "```\n-0.07\n```", 1},
		{"wrong: forgot the sign", "0.07", 0},
		{"wrong: reported similarity, not the difference", "0.9", 0},
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

func TestVecIndexBuildQueryTradeoffTest_Eval(t *testing.T) {
	tc := vecIndexBuildQueryTradeoffTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"all correct", `{"scenario_a":"ivf-flat","scenario_b":"hnsw"}`, 1},
		{"all correct fenced", "```json\n{\"scenario_a\":\"ivf-flat\",\"scenario_b\":\"hnsw\"}\n```", 1},
		{"all correct, different case", `{"scenario_a":"IVF-FLAT","scenario_b":"HNSW"}`, 1},
		{"scenario_a wrong", `{"scenario_a":"hnsw","scenario_b":"hnsw"}`, 0.5},
		{"scenario_b wrong", `{"scenario_a":"ivf-flat","scenario_b":"ivf-flat"}`, 0.5},
		{"both swapped", `{"scenario_a":"hnsw","scenario_b":"ivf-flat"}`, 0},
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

func TestVecEmbeddingDimensionTradeoffTest_Eval(t *testing.T) {
	// Independent re-derivation confirming only 128-dim fits the budget.
	const budgetBytes = 60_000_000_000
	bytes128 := 128 * 4 * 100_000_000
	bytes768 := 768 * 4 * 100_000_000
	if bytes128 > budgetBytes {
		t.Fatalf("128-dim total %d exceeds budget %d, expected it to fit", bytes128, budgetBytes)
	}
	if bytes768 <= budgetBytes {
		t.Fatalf("768-dim total %d fits budget %d, expected it not to fit", bytes768, budgetBytes)
	}

	tc := vecEmbeddingDimensionTradeoffTest()
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct: 128", `{"choice":"128"}`, 1},
		{"correct, fenced with prose", "The 128-dim option fits:\n```json\n{\"choice\":\"128\"}\n```", 1},
		{"wrong: 768", `{"choice":"768"}`, 0},
		{"wrong: neither valid option", `{"choice":"512"}`, 0},
		{
			// C7 bug probe: the prompt's own template shows choice
			// unquoted ({"choice":128|768}), so a genuinely numeric
			// (unquoted) response must also score full credit.
			name:     "correct: unquoted numeric form matching the prompt's own template (C7 bug probe)",
			response: `{"choice":128}`,
			want:     1,
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

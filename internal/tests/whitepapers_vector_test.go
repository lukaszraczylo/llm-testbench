package tests

import (
	"context"
	"math"
	"testing"
)

func TestPaperPQCompressionRatioWant_GroundTruth(t *testing.T) {
	// Independent re-derivation with plain arithmetic, not via
	// wpQuantizedCodeBytes.
	const dim, m, nbits = 128, 8, 8
	rawBytes := dim * 4
	codeBytes := m * nbits / 8
	want := rawBytes / codeBytes

	if want != 64 {
		t.Fatalf("independently recomputed ratio = %d, want 64", want)
	}
	if paperPQCompressionRatioWant != want {
		t.Errorf("paperPQCompressionRatioWant = %d, independently recomputed = %d", paperPQCompressionRatioWant, want)
	}
}

func TestPaperPQCompressionRatioTest_Eval(t *testing.T) {
	tc := paperPQCompressionRatioTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "64", 1},
		{"prose wrapped", "The compression ratio is 64x.", 1},
		{"wrong", "8", 0},
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

func TestPaperBloomFPRateWant_GroundTruth(t *testing.T) {
	// Independent re-derivation with math.Pow/math.Exp inline, not via
	// wpBloomFalsePositiveRate.
	const m, n, k = 10000.0, 1000.0, 7.0
	want := math.Round(math.Pow(1-math.Exp(-k*n/m), k)*10000) / 10000

	if math.Abs(want-0.0082) > 1e-9 {
		t.Fatalf("independently recomputed FP rate = %v, want ~0.0082", want)
	}
	if paperBloomFPRateWant != want {
		t.Errorf("paperBloomFPRateWant = %v, independently recomputed = %v", paperBloomFPRateWant, want)
	}
}

func TestPaperBloomFPRateTest_Eval(t *testing.T) {
	tc := paperBloomFPRateTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "0.0082", 1},
		{"within tolerance", "0.0081", 1},
		{"prose wrapped", "p is approximately 0.0082.", 1},
		{"outside tolerance", "0.05", 0},
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

func TestPaperTFIDFTermScoreWant_GroundTruth(t *testing.T) {
	// Independent re-derivation by counting the raw corpus text directly,
	// not via wpTermFrequency/wpDocumentFrequency/wpTFIDF.
	tf := 1 // "sat" appears once in doc1
	df := 2 // doc1 and doc2 contain "sat"; doc3 does not
	n := 3  // 3 documents total
	want := math.Round(float64(tf)*math.Log(float64(n)/float64(df))*10000) / 10000

	if math.Abs(want-0.4055) > 1e-9 {
		t.Fatalf("independently recomputed tfidf = %v, want ~0.4055", want)
	}
	if paperTFIDFTermScoreWant != want {
		t.Errorf("paperTFIDFTermScoreWant = %v, independently recomputed = %v", paperTFIDFTermScoreWant, want)
	}
}

func TestPaperTFIDFTermScoreTest_Eval(t *testing.T) {
	tc := paperTFIDFTermScoreTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "0.4055", 1},
		{"within tolerance", "0.4053", 1},
		{"prose wrapped", "The TF-IDF weight is 0.4055.", 1},
		{"outside tolerance", "1.0986", 0},
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

func TestPaperRecallAtKWant_GroundTruth(t *testing.T) {
	// Independent re-derivation with a plain set intersection, not via
	// wpRecallAtK.
	relevant := map[string]bool{"D2": true, "D5": true, "D7": true, "D9": true}
	retrieved := []string{"D1", "D5", "D3", "D7", "D6", "D2"}
	hit := 0
	for _, id := range retrieved {
		if relevant[id] {
			hit++
		}
	}
	want := float64(hit) / float64(len(relevant))

	if want != 0.75 {
		t.Fatalf("independently recomputed recall@6 = %v, want 0.75", want)
	}
	if paperRecallAtKWant != want {
		t.Errorf("paperRecallAtKWant = %v, independently recomputed = %v", paperRecallAtKWant, want)
	}
}

func TestPaperRecallAtKTest_Eval(t *testing.T) {
	tc := paperRecallAtKTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "0.75", 1},
		{"prose wrapped", "recall@6 is 0.75.", 1},
		{"wrong: counted D9 as retrieved", "1.0", 0},
		{"wrong: missed one hit", "0.5", 0},
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

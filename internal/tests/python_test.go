package tests

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestCosineSimilarity_KnownCases(t *testing.T) {
	// Grounds the cosineSimilarity helper itself against two textbook
	// cases before trusting it to derive pyCosineWant.
	tests := []struct {
		name string
		a, b []float64
		want float64
	}{
		{"identical vectors", []float64{1, 2, 3}, []float64{1, 2, 3}, 1},
		{"orthogonal vectors", []float64{1, 0}, []float64{0, 1}, 0},
		{"opposite vectors", []float64{1, 0}, []float64{-1, 0}, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("cosineSimilarity(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestPyCosineWant_GroundTruth(t *testing.T) {
	// Independent re-derivation via math.Sqrt, not via the cosineSimilarity
	// helper under test, per PLAN.md's rule to recompute cheap ground
	// truths in the unit test.
	a, b := pyCosineVectorA, pyCosineVectorB
	var dot, sumA, sumB float64
	for i := range a {
		dot += a[i] * b[i]
		sumA += a[i] * a[i]
		sumB += b[i] * b[i]
	}
	want := dot / (math.Sqrt(sumA) * math.Sqrt(sumB))
	wantRounded := math.Round(want*10000) / 10000

	if pyCosineWant != wantRounded {
		t.Errorf("pyCosineWant = %v, independently recomputed = %v", pyCosineWant, wantRounded)
	}
}

func TestPyLogTriageWant_GroundTruth(t *testing.T) {
	// Independent re-derivation of the expected counts by scanning the raw
	// log text directly, rather than trusting the hand count in the
	// doc comment.
	lines := strings.Split(pyLogTriageLog, "\n")
	if len(lines) != 15 {
		t.Fatalf("pyLogTriageLog has %d lines, want 15", len(lines))
	}

	oom := strings.Count(pyLogTriageLog, "OOMKilled")
	pull := strings.Count(pyLogTriageLog, "ImagePullBackOff")
	if oom != 5 {
		t.Errorf("OOMKilled count = %d, want 5", oom)
	}
	if pull != 4 {
		t.Errorf("ImagePullBackOff count = %d, want 4", pull)
	}

	want := "ImagePullBackOff=4\nOOMKilled=5"
	if pyLogTriageWant != want {
		t.Errorf("pyLogTriageWant = %q, want %q", pyLogTriageWant, want)
	}
}

func TestPyLogTriageTest_Eval(t *testing.T) {
	tc := pyLogTriageTest()

	correctScript := "```python\n" + `counts = {"OOMKilled": 0, "ImagePullBackOff": 0}
log = """` + pyLogTriageLog + `"""
for line in log.splitlines():
    for key in counts:
        if key in line:
            counts[key] += 1
for key in sorted(counts):
    print(f"{key}={counts[key]}")
` + "```"

	wrongScript := "```python\nprint('OOMKilled=1')\nprint('ImagePullBackOff=1')\n```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct triage script", correctScript, 1},
		{"wrong counts", wrongScript, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Skipped {
				t.Skip("python3 not available, skipping exec-evaluator assertion")
			}
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestPyCosineTest_Eval(t *testing.T) {
	tc := pyCosineTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "0.9896", 1},
		{"within tolerance", "0.9899", 1},
		{"prose wrapped", "The cosine similarity is 0.9896.", 1},
		{"outside tolerance", "0.5", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

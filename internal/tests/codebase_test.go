package tests

import (
	"context"
	"testing"
)

// TestMix_GroundTruth runs mix itself (the traced function) to ground
// codeTraceGoWant, per PLAN.md's instruction for this specific test.
func TestMix_GroundTruth(t *testing.T) {
	tests := []struct {
		name  string
		n     int
		depth int
		want  int
	}{
		{"base case depth 0", 12, 0, 12},
		{"one level, even", 24, 1, 13}, // 24>>1=12; mix(12,0)+1=13
		{"two levels", 48, 2, 15},      // 48>>1=24; mix(24,1)+2=15
		{"three levels", 96, 3, 18},    // 96>>1=48; mix(48,2)+3=18
		{"full trace, odd start", 37, 4, 22},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mix(tt.n, tt.depth)
			if got != tt.want {
				t.Errorf("mix(%d, %d) = %d, want %d", tt.n, tt.depth, got, tt.want)
			}
		})
	}

	if codeTraceGoWant != 22 {
		t.Errorf("codeTraceGoWant = %d, want 22", codeTraceGoWant)
	}
	if got := mix(37, 4); got != codeTraceGoWant {
		t.Errorf("mix(37, 4) = %d, want codeTraceGoWant %d", got, codeTraceGoWant)
	}
}

func TestCodeTraceGoTest_Eval(t *testing.T) {
	tc := codeTraceGoTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "22", 1},
		{"prose wrapped", "The result is 22.", 1},
		{"off by one", "21", 0},
		{"forgot to track depth in the unwind", "12", 0},
		// S6 regression: a trailing "64-bit" qualifier after the real
		// answer must not be mistaken for the answer itself.
		{"answer followed by a 64-bit qualifier", "The result is 22, computed via a 64-bit accumulator.", 1},
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

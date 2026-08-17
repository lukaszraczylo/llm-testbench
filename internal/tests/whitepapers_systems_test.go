package tests

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestPaperLSMWriteAmpWant_GroundTruth(t *testing.T) {
	// Independent re-derivation by counting the excerpt's stated rewrite
	// path (L0->L1, L1->L2, L2->L3 = 3 rewrites) plus the original write,
	// not via wpLSMWriteAmplification.
	// The excerpt names exactly three compaction moves; check each phrase is
	// present rather than counting "into L" substrings, since the initial
	// "flushed into L0" also contains that substring without being a
	// compaction rewrite.
	moves := []string{"compacted into L1", "L1 into L2", "L2 into L3"}
	rewrites := 0
	for _, phrase := range moves {
		if strings.Contains(paperLSMExcerpt, phrase) {
			rewrites++
		}
	}
	if rewrites != 3 {
		t.Fatalf("excerpt describes %d of the 3 expected compaction moves", rewrites)
	}
	want := 1 + rewrites

	if want != 4 {
		t.Fatalf("independently recomputed write amplification = %d, want 4", want)
	}
	if paperLSMWriteAmpWant != want {
		t.Errorf("paperLSMWriteAmpWant = %d, independently recomputed = %d", paperLSMWriteAmpWant, want)
	}
}

func TestPaperLSMWriteAmplificationTest_Eval(t *testing.T) {
	tc := paperLSMWriteAmplificationTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "4", 1},
		{"prose wrapped", "The write amplification factor is 4.", 1},
		{"forgot the original write", "3", 0},
		{"off by one", "5", 0},
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

func TestPaperRaftQuorumWant_GroundTruth(t *testing.T) {
	// Independent re-derivation of floor(n/2)+1 and n-quorum for several N,
	// not via wpRaftQuorumSize/wpRaftMaxTolerableFailures.
	tests := []struct {
		n            int
		wantQuorum   int
		wantFailures int
	}{
		{3, 2, 1},
		{5, 3, 2},
		{7, 4, 3},
	}
	for _, tt := range tests {
		gotQuorum := tt.n/2 + 1
		gotFailures := tt.n - gotQuorum
		if gotQuorum != tt.wantQuorum {
			t.Errorf("quorum(n=%d) = %d, want %d", tt.n, gotQuorum, tt.wantQuorum)
		}
		if gotFailures != tt.wantFailures {
			t.Errorf("maxFailures(n=%d) = %d, want %d", tt.n, gotFailures, tt.wantFailures)
		}
	}

	if paperRaftQuorumWant != 3 {
		t.Errorf("paperRaftQuorumWant = %d, want 3", paperRaftQuorumWant)
	}
	if paperRaftFailuresWant != 2 {
		t.Errorf("paperRaftFailuresWant = %d, want 2", paperRaftFailuresWant)
	}
}

func TestPaperRaftQuorumFailuresTest_Eval(t *testing.T) {
	tc := paperRaftQuorumFailuresTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"all correct", `{"quorum_size":3,"max_tolerable_failures":2}`, 1},
		{"one field wrong", `{"quorum_size":3,"max_tolerable_failures":3}`, 0.5},
		{"both wrong: used simple majority of reachable nodes", `{"quorum_size":2,"max_tolerable_failures":1}`, 0},
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

func TestPaperBTreeHeightWant_GroundTruth(t *testing.T) {
	// Independent re-derivation with math.Pow, not via
	// wpBTreeMinBranchingLevels's iterative loop.
	const f, n = 100.0, 5000000.0
	h := 0
	for math.Pow(f, float64(h+1)) < n {
		h++
	}

	if h != 3 {
		t.Fatalf("independently recomputed h = %d, want 3", h)
	}
	if paperBTreeHeightWant != h {
		t.Errorf("paperBTreeHeightWant = %d, independently recomputed = %d", paperBTreeHeightWant, h)
	}
}

func TestPaperBTreeHeightLevelsTest_Eval(t *testing.T) {
	tc := paperBTreeHeightLevelsTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "3", 1},
		{"prose wrapped", "The minimum height is h=3.", 1},
		{"one level short", "2", 0},
		{"one level too many", "4", 0},
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

func TestPaperCAPAvailabilityChoiceTest_Eval(t *testing.T) {
	tc := paperCAPAvailabilityChoiceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct lowercase", "availability", 1},
		{"correct capitalized", "Availability", 1},
		{"correct with trailing period", "Availability.", 1},
		{"wrong: consistency", "Consistency", 0},
		{"wrong: extra words break the forced one-word format", "The answer is availability", 0},
		// B2: the prompt's own phrasing quotes the two options, so a
		// quoted or bolded answer must still score full credit.
		{"correct, quoted (prompt's own phrasing)", `"Availability"`, 1},
		{"correct, bolded", "**Availability**", 1},
		{"correct, quoted with trailing period", `"Availability".`, 1},
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

func TestPaperAttentionScaleFactorTest_Eval(t *testing.T) {
	tc := paperAttentionScaleFactorTest()

	// Ground truth: sqrt(64) = 8, independently recomputed with math.Sqrt.
	if got := math.Sqrt(64); got != 8 {
		t.Fatalf("math.Sqrt(64) = %v, want 8", got)
	}

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"all correct", `{"d_k":64,"scale_factor":8}`, 1},
		{"one field wrong", `{"d_k":64,"scale_factor":64}`, 0.5},
		{"both wrong", `{"d_k":512,"scale_factor":22.6}`, 0},
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

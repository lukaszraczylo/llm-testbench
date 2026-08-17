package tests

import (
	"context"
	"regexp"
	"strconv"
	"testing"
)

// TestPaperHnswParamsExcerpt_GroundTruth independently extracts the three
// numeric parameter values from the raw excerpt text via regex, rather
// than trusting the doc comment's transcription of them.
func TestPaperHnswParamsExcerpt_GroundTruth(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    int
	}{
		{"M", `M=(\d+)`, 16},
		{"efConstruction", `efConstruction[^\d]*(\d+)`, 200},
		{"efSearch", `efSearch,\s*set to (\d+)`, 64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			m := re.FindStringSubmatch(paperHnswParamsExcerpt)
			if m == nil {
				t.Fatalf("pattern %q found no match in excerpt", tt.pattern)
			}
			got, err := strconv.Atoi(m[1])
			if err != nil {
				t.Fatalf("parse %q: %v", m[1], err)
			}
			if got != tt.want {
				t.Errorf("extracted %s = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

func TestPaperHnswParamsTest_Eval(t *testing.T) {
	tc := paperHnswParamsTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"all correct", `{"M":16,"efConstruction":200,"efSearch":64}`, 1},
		{"one field wrong", `{"M":32,"efConstruction":200,"efSearch":64}`, 2.0 / 3.0},
		{"all wrong", `{"M":8,"efConstruction":100,"efSearch":32}`, 0},
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

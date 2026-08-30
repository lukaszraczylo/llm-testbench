package report

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func attempt(model, testID string, score float64) AttemptResult {
	return AttemptResult{Model: model, TestID: testID, Category: "c", Subcategory: "s", Score: score}
}

func TestCompareArtifacts_ClassifiesRegressionImprovementStable(t *testing.T) {
	baseline := Artifact{Results: []AttemptResult{
		attempt("m1", "reg", 1.0),
		attempt("m1", "imp", 0.2),
		attempt("m1", "stable", 1.0),
	}}
	current := Artifact{Results: []AttemptResult{
		attempt("m1", "reg", 0.4),
		attempt("m1", "imp", 1.0),
		attempt("m1", "stable", 0.995),
	}}

	got := CompareArtifacts(baseline, current)
	byID := map[string]Comparison{}
	for _, c := range got {
		byID[c.TestID] = c
	}
	if byID["reg"].Status != "regression" || byID["reg"].Delta > -0.5 {
		t.Errorf("reg = %+v, want regression delta ~ -0.6", byID["reg"])
	}
	if byID["imp"].Status != "improvement" {
		t.Errorf("imp = %+v, want improvement", byID["imp"])
	}
	if byID["stable"].Status != "stable" {
		t.Errorf("stable = %+v, want stable", byID["stable"])
	}
	// Worst delta first.
	if got[0].TestID != "reg" {
		t.Errorf("first row = %q, want worst regression %q", got[0].TestID, "reg")
	}
}

func TestCompareArtifacts_AveragesAttemptsAndFlagsMembership(t *testing.T) {
	baseline := Artifact{Results: []AttemptResult{
		attempt("m1", "flaky", 1.0), attempt("m1", "flaky", 0.0), // mean 0.5
		attempt("m1", "gone", 1.0),
	}}
	current := Artifact{Results: []AttemptResult{
		attempt("m1", "flaky", 1.0), attempt("m1", "flaky", 1.0), // mean 1.0 => improvement
		attempt("m1", "new", 0.5),
	}}

	got := CompareArtifacts(baseline, current)
	byID := map[string]Comparison{}
	for _, c := range got {
		byID[c.TestID] = c
	}
	if byID["flaky"].Status != "improvement" || math.Abs(byID["flaky"].BaselineMean-0.5) > 1e-9 {
		t.Errorf("flaky = %+v, want improvement from mean 0.5", byID["flaky"])
	}
	if byID["gone"].Status != "baseline-only" {
		t.Errorf("gone = %+v, want baseline-only", byID["gone"])
	}
	if byID["new"].Status != "current-only" {
		t.Errorf("new = %+v, want current-only", byID["new"])
	}
}

func TestCompareArtifacts_TruncatedAttemptExcludedFromMean(t *testing.T) {
	clean := func(score float64) AttemptResult {
		return AttemptResult{Model: "m1", TestID: "t", Category: "c", Subcategory: "s", Score: score}
	}
	trunc := func(score float64) AttemptResult {
		return AttemptResult{Model: "m1", TestID: "t", Category: "c", Subcategory: "s", Score: score, Truncated: true}
	}
	baseline := Artifact{Results: []AttemptResult{clean(1.0), trunc(0.0)}} // clean mean 1.0
	current := Artifact{Results: []AttemptResult{trunc(0.4), trunc(0.6)}}  // all truncated => fallback 0.5

	got := CompareArtifacts(baseline, current)
	if len(got) != 1 {
		t.Fatalf("comparisons = %d, want 1", len(got))
	}
	if math.Abs(got[0].BaselineMean-1.0) > 1e-9 {
		t.Errorf("baseline mean = %v, want 1.0 (truncated attempt excluded)", got[0].BaselineMean)
	}
	if math.Abs(got[0].CurrentMean-0.5) > 1e-9 {
		t.Errorf("current mean = %v, want 0.5 (all-truncated fallback)", got[0].CurrentMean)
	}
	if got[0].Status != "regression" {
		t.Errorf("status = %q, want regression", got[0].Status)
	}
}

func TestRenderCompare_HeadlineAndRows(t *testing.T) {
	baseline := Artifact{Results: []AttemptResult{attempt("m1", "a", 1.0), attempt("m1", "b", 1.0)}}
	current := Artifact{Results: []AttemptResult{attempt("m1", "a", 0.0), attempt("m1", "b", 1.0)}}

	var buf bytes.Buffer
	if err := RenderCompare(&buf, CompareArtifacts(baseline, current)); err != nil {
		t.Fatalf("RenderCompare() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{"1 regressions", "0 improvements", "1 stable", "a", "-1.00"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\nb\t") {
		t.Errorf("stable row b should be summarized, not listed:\n%s", out)
	}
}

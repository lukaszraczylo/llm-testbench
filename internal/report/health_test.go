package report

import (
	"bytes"
	"strings"
	"testing"
)

// scoredAttempt builds a scored attempt in subcategory (cat, sub).
func scoredAttempt(model, testID, cat, sub string, score float64) AttemptResult {
	return AttemptResult{Model: model, TestID: testID, Category: cat, Subcategory: sub, Score: score}
}

func subHealth(t *testing.T, r HealthReport, cat, sub string) SubHealth {
	t.Helper()
	for _, sh := range r.Subcategories {
		if sh.Category == cat && sh.Subcategory == sub {
			return sh
		}
	}
	t.Fatalf("subcategory %s/%s missing from report", cat, sub)
	return SubHealth{}
}

func TestAuditHealth_SaturatedVersusWeakTests(t *testing.T) {
	// a: perfect for both models => saturated, no signal.
	// b: model m2 fails => weak, carries signal.
	art := Artifact{Results: []AttemptResult{
		scoredAttempt("m1", "a", "c", "s", 1.0),
		scoredAttempt("m2", "a", "c", "s", 1.0),
		scoredAttempt("m1", "b", "c", "s", 1.0),
		scoredAttempt("m2", "b", "c", "s", 0.0),
	}}
	r := AuditHealth([]Artifact{art})

	sh := subHealth(t, r, "c", "s")
	if sh.Tests != 2 || sh.Cells != 4 {
		t.Fatalf("tests/cells = %d/%d, want 2/4", sh.Tests, sh.Cells)
	}
	if sh.Perfect != 3 {
		t.Errorf("perfect = %d, want 3", sh.Perfect)
	}
	if sh.Saturated != 1 {
		t.Errorf("saturated = %d, want 1 (only test a)", sh.Saturated)
	}
	if len(sh.WeakTests) != 1 || sh.WeakTests[0] != "b" {
		t.Errorf("weak = %v, want [b]", sh.WeakTests)
	}
	if r.Saturated != 1 || r.Tests != 2 {
		t.Errorf("report totals = %d/%d, want 1/2", r.Saturated, r.Tests)
	}
	if len(r.Models) != 2 || r.Models[0] != "m1" {
		t.Errorf("models = %v, want [m1 m2]", r.Models)
	}
}

func TestAuditHealth_PoolsWorstCellAcrossArtifacts(t *testing.T) {
	// A flaky test that passes on a second artifact must still count as
	// weak: health asks whether any observed run failed it.
	a1 := Artifact{Results: []AttemptResult{
		scoredAttempt("m1", "flaky", "c", "s", 0.0),
	}}
	a2 := Artifact{Results: []AttemptResult{
		scoredAttempt("m1", "flaky", "c", "s", 1.0),
	}}
	r := AuditHealth([]Artifact{a1, a2})

	sh := subHealth(t, r, "c", "s")
	if sh.Saturated != 0 || len(sh.WeakTests) != 1 {
		t.Errorf("saturated=%d weak=%v, want 0/[flaky] (worst cell pooled)", sh.Saturated, sh.WeakTests)
	}
	if sh.Cells != 1 {
		t.Errorf("cells = %d, want 1 (same model,test pooled)", sh.Cells)
	}
}

func TestAuditHealth_AveragesRepeatedAttemptsPerCell(t *testing.T) {
	// Two attempts of one cell: mean 0.5 < PerfectScore => not perfect.
	art := Artifact{Results: []AttemptResult{
		scoredAttempt("m1", "t", "c", "s", 1.0),
		scoredAttempt("m1", "t", "c", "s", 0.0),
	}}
	r := AuditHealth([]Artifact{art})

	sh := subHealth(t, r, "c", "s")
	if sh.Cells != 1 || sh.Perfect != 0 {
		t.Errorf("cells/perfect = %d/%d, want 1/0 (attempt mean 0.5)", sh.Cells, sh.Perfect)
	}
}

func TestAuditHealth_ErrorsAndSkipsExcluded(t *testing.T) {
	art := Artifact{Results: []AttemptResult{
		{Model: "m1", TestID: "boom", Category: "c", Subcategory: "s", Error: "404"},
		{Model: "m1", TestID: "skip", Category: "c", Subcategory: "s", Skipped: true},
		scoredAttempt("m1", "ok", "c", "s", 1.0),
	}}
	r := AuditHealth([]Artifact{art})

	sh := subHealth(t, r, "c", "s")
	if sh.Tests != 1 || sh.Cells != 1 || sh.Saturated != 1 {
		t.Errorf("tests/cells/saturated = %d/%d/%d, want 1/1/1 (error+skip excluded)",
			sh.Tests, sh.Cells, sh.Saturated)
	}
}

func TestAuditHealth_FlagsUnstableAndTruncated(t *testing.T) {
	art := Artifact{
		Results: []AttemptResult{
			scoredAttempt("m1", "u", "c", "s", 1.0),
			scoredAttempt("m1", "t", "c", "s", 0.5),
		},
		Stats: []jsonStatRow{
			{TestID: "u", Unstable: true},
			{TestID: "t", Truncated: true},
		},
	}
	r := AuditHealth([]Artifact{art})

	sh := subHealth(t, r, "c", "s")
	if sh.Unstable != 1 || sh.Truncated != 1 {
		t.Errorf("unstable/truncated = %d/%d, want 1/1", sh.Unstable, sh.Truncated)
	}
}

func TestAuditHealth_SaturationBoundary(t *testing.T) {
	// 9 perfect cells + 1 failed cell = 0.9 rate = at SaturationRate =>
	// saturated. 8/9 = 0.888 < rate => weak.
	perfect9 := Artifact{}
	for i := range 9 {
		perfect9.Results = append(perfect9.Results, scoredAttempt("m"+string(rune('a'+i)), "t1", "c", "s", 1.0))
	}
	perfect9.Results = append(perfect9.Results, scoredAttempt("mj", "t1", "c", "s", 0.0))

	if r := AuditHealth([]Artifact{perfect9}); r.Saturated != 1 {
		t.Errorf("9/10 perfect: saturated = %d, want 1 (rate == SaturationRate)", r.Saturated)
	}

	eight := Artifact{}
	for i := range 8 {
		eight.Results = append(eight.Results, scoredAttempt("m"+string(rune('a'+i)), "t1", "c", "s", 1.0))
	}
	eight.Results = append(eight.Results,
		scoredAttempt("mi", "t1", "c", "s", 0.0),
		scoredAttempt("mj", "t1", "c", "s", 0.0))

	if r := AuditHealth([]Artifact{eight}); r.Saturated != 0 {
		t.Errorf("8/10 perfect: saturated = %d, want 0", r.Saturated)
	}
}

func TestAuditHealth_SortsMostSaturatedFirst(t *testing.T) {
	art := Artifact{Results: []AttemptResult{
		scoredAttempt("m1", "hard", "c", "low", 0.0),
		scoredAttempt("m1", "easy1", "c", "high", 1.0),
		scoredAttempt("m1", "easy2", "c", "high", 1.0),
	}}
	r := AuditHealth([]Artifact{art})

	if len(r.Subcategories) != 2 {
		t.Fatalf("subcategories = %d, want 2", len(r.Subcategories))
	}
	if got := r.Subcategories[0].Subcategory; got != "high" {
		t.Errorf("first row = %q, want most saturated %q", got, "high")
	}
}

func TestRenderHealth_HeadlineAndWeakList(t *testing.T) {
	art := Artifact{Results: []AttemptResult{
		scoredAttempt("m1", "a", "c", "s", 1.0),
		scoredAttempt("m1", "b", "c", "s", 0.0),
	}}
	var buf bytes.Buffer
	if err := RenderHealth(&buf, AuditHealth([]Artifact{art})); err != nil {
		t.Fatalf("RenderHealth() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{"2 tests", "2 scored", "c/s", "50.0", "1: b"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// "a" is saturated: it must not appear anywhere in the weak list.
	if strings.Contains(out, ": a") || strings.Contains(out, ", a,") || strings.HasSuffix(strings.TrimSpace(out), " a") {
		t.Errorf("saturated test a should not be listed weak:\n%s", out)
	}
}

func TestRenderHealth_AllSaturatedVerdict(t *testing.T) {
	art := Artifact{Results: []AttemptResult{
		scoredAttempt("m1", "a", "c", "s", 1.0),
		scoredAttempt("m2", "a", "c", "s", 1.0),
	}}
	var buf bytes.Buffer
	if err := RenderHealth(&buf, AuditHealth([]Artifact{art})); err != nil {
		t.Fatalf("RenderHealth() error = %v", err)
	}
	if !strings.Contains(buf.String(), "cannot rank these models") {
		t.Errorf("missing all-saturated verdict:\n%s", buf.String())
	}
}

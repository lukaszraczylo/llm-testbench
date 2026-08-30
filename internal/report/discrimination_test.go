package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/runner"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func TestDiscrimination_TableShowsSpreadAndInstability(t *testing.T) {
	tests := []testkit.Test{
		{ID: "sep", Category: "c", Subcategory: "s"},
		{ID: "same", Category: "c", Subcategory: "s"},
	}
	results := []runner.Result{
		{Model: "m1", TestID: "sep", Score: eval.Score{Value: 1}},
		{Model: "m2", TestID: "sep", Score: eval.Score{Value: 0.2}},
		{Model: "m1", TestID: "same", Score: eval.Score{Value: 1}},
		{Model: "m2", TestID: "same", Score: eval.Score{Value: 1}},
		// Flaky pair for m1/sep: a second attempt disagrees with the first.
		{Model: "m1", TestID: "sep", Attempt: 1, Score: eval.Score{Value: 0.4}},
	}

	var buf bytes.Buffer
	if err := Render(&buf, FormatTable, tests, []string{"m1", "m2"}, results); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"== Discrimination: 1/2 scored tests separate models by >= 0.05; 1 unstable under repeat ==",
		"sep",
		"[0.40-1.00]", // unstable range in the score table cell
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\nsame\t") {
		// "same" appears in the per-test table; ensure it is NOT in the
		// discrimination rows by checking the section body only.
		section := out[strings.Index(out, "== Discrimination"):]
		if strings.Contains(section, "same") {
			t.Errorf("non-discriminating test listed in discrimination section:\n%s", section)
		}
	}
}

func TestDiscrimination_JSONStatsAndAttemptRoundTrip(t *testing.T) {
	tests := []testkit.Test{{ID: "t1", Category: "c", Subcategory: "s"}}
	results := []runner.Result{
		{Model: "m1", TestID: "t1", Score: eval.Score{Value: 1}},
		{Model: "m2", TestID: "t1", Score: eval.Score{Value: 0.5}},
		{Model: "m1", TestID: "t1", Attempt: 1, Score: eval.Score{Value: 0.5}},
	}

	var buf bytes.Buffer
	if err := Render(&buf, FormatJSON, tests, []string{"m1", "m2"}, results); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var doc jsonArtifact
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(doc.Stats) != 1 {
		t.Fatalf("len(stats) = %d, want 1", len(doc.Stats))
	}
	st := doc.Stats[0]
	if !st.Unstable {
		t.Error("Unstable = false, want true (attempts 1.0 vs 0.5)")
	}
	if st.Spread < 0.24 || st.Spread > 0.26 {
		t.Errorf("Spread = %v, want ~0.25 (m1 mean 0.75 vs m2 0.5)", st.Spread)
	}

	// Attempt indices survive the round trip for compare.
	var attempts []AttemptResult
	attempts = append(attempts, doc.Results...)
	if attempts[2].Attempt != 1 {
		t.Errorf("attempt = %d, want 1", attempts[2].Attempt)
	}
}

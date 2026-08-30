package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/llm"
	"github.com/lukaszraczylo/llm-testbench/internal/runner"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func sampleTests() []testkit.Test {
	return []testkit.Test{
		{ID: "go-1", Category: "programming", Subcategory: "golang"},
		{ID: "py-1", Category: "programming", Subcategory: "python"},
		{ID: "k8s-1", Category: "operations", Subcategory: "kubernetes"},
	}
}

func sampleResults() []runner.Result {
	return []runner.Result{
		{Model: "m1", TestID: "go-1", Score: eval.Score{Value: 1}, Latency: 100 * time.Millisecond, PromptTokens: 6, CompletionTokens: 4},
		{Model: "m1", TestID: "py-1", Score: eval.Score{Value: 0.5}, Latency: 200 * time.Millisecond, PromptTokens: 12, CompletionTokens: 8},
		{Model: "m1", TestID: "k8s-1", Err: errors.New("boom")},
		{Model: "m2", TestID: "go-1", Score: eval.Score{Value: 0}, Latency: 50 * time.Millisecond, PromptTokens: 3, CompletionTokens: 2},
		{Model: "m2", TestID: "py-1", Score: eval.Score{Skipped: true, Detail: "toolchain missing: cc"}},
		{Model: "m2", TestID: "k8s-1", Score: eval.Score{Value: 1}, Latency: 150 * time.Millisecond, PromptTokens: 9, CompletionTokens: 6},
	}
}

// Truncation policy: attempts cut off by the token budget are excluded
// from a cell's mean when any attempt finished normally; the cell is still
// flagged "!" so the discarded sample is visible. When every scored
// attempt was truncated, the mean falls back to the partial-text scores.
func TestIndexResults_TruncatedAttemptExcludedFromMean(t *testing.T) {
	idx := indexResults([]runner.Result{
		{Model: "m1", TestID: "t", Attempt: 0, Score: eval.Score{Value: 1}, PromptTokens: 4},
		{Model: "m1", TestID: "t", Attempt: 1, Score: eval.Score{Value: 0}, FinishReason: llm.FinishReasonLength, PromptTokens: 100},
	})
	c := idx["t"]["m1"]
	if got := c.result.Score.Value; got != 1 {
		t.Errorf("mean = %v, want 1 (clean attempts only)", got)
	}
	if !c.result.Truncated() {
		t.Error("cell should be flagged truncated")
	}
	if c.minScore != 1 || c.maxScore != 1 {
		t.Errorf("min/max = %v/%v, want 1/1 (truncated attempt excluded from instability range)", c.minScore, c.maxScore)
	}
	if got := int(c.meanTokens); got != 4 {
		t.Errorf("meanTokens = %d, want 4 (clean attempts only)", got)
	}
}

func TestIndexResults_AllAttemptsTruncatedFallBackToPartialScores(t *testing.T) {
	idx := indexResults([]runner.Result{
		{Model: "m1", TestID: "t", Attempt: 0, Score: eval.Score{Value: 0.4}, FinishReason: llm.FinishReasonLength, PromptTokens: 10},
		{Model: "m1", TestID: "t", Attempt: 1, Score: eval.Score{Value: 0.6}, FinishReason: llm.FinishReasonLength, PromptTokens: 20},
	})
	c := idx["t"]["m1"]
	if got := c.result.Score.Value; math.Abs(got-0.5) > 1e-9 {
		t.Errorf("mean = %v, want 0.5 (fallback over all-truncated attempts)", got)
	}
	if !c.result.Truncated() {
		t.Error("cell should be flagged truncated")
	}
	if c.scored != 2 {
		t.Errorf("scored = %d, want 2", c.scored)
	}
}

func TestRenderTable_ContainsExpectedCells(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, FormatTable, sampleTests(), []string{"m1", "m2"}, sampleResults()); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"== Per-test scores ==",
		"== Category rollup (mean score) ==",
		"== Model summary ==",
		"1.00 (10tok)", // m1/go-1: 6 prompt + 4 completion
		"0.50 (20tok)", // m1/py-1: 12 prompt + 8 completion
		"ERR",          // m1/k8s-1
		"0.00 (5tok)",  // m2/go-1: 3 prompt + 2 completion
		"skip",         // m2/py-1
		"TOK/S",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderMarkdown_ProducesGitHubTables(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, FormatMarkdown, sampleTests(), []string{"m1", "m2"}, sampleResults()); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"## Per-test scores",
		"| Test | Category | Subcategory | m1 | m2 |",
		"| --- | --- | --- | --- | --- |",
		"## Category rollup (mean score)",
		"## Model summary",
		"Tok/s",
		"1.00 (10tok)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown output missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderJSON_DumpsRawResults(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, FormatJSON, sampleTests(), []string{"m1", "m2"}, sampleResults()); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var doc jsonArtifact
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; output:\n%s", err, buf.String())
	}
	out := doc.Results
	if len(out) != len(sampleResults()) {
		t.Fatalf("len(out) = %d, want %d", len(out), len(sampleResults()))
	}

	var errEntry, skipEntry, tokenEntry *jsonResult
	for i := range out {
		if out[i].Error != "" {
			errEntry = &out[i]
		}
		if out[i].Skipped {
			skipEntry = &out[i]
		}
		if out[i].Model == "m1" && out[i].TestID == "go-1" {
			tokenEntry = &out[i]
		}
	}
	if errEntry == nil || errEntry.Error != "boom" {
		t.Errorf("expected an entry with Error = \"boom\", got %+v", errEntry)
	}
	if skipEntry == nil || skipEntry.Category != "programming" {
		t.Errorf("expected skipped entry to carry Category, got %+v", skipEntry)
	}
	if tokenEntry == nil || tokenEntry.PromptTokens != 6 || tokenEntry.CompletionTokens != 4 || tokenEntry.Tokens != 10 {
		t.Errorf("expected m1/go-1 to carry prompt=6 completion=4 total=10 tokens, got %+v", tokenEntry)
	}
}

func TestRenderTable_TruncatedCellsGetSuffixAndLegend(t *testing.T) {
	tests := []testkit.Test{{ID: "t1", Category: "c", Subcategory: "s"}}
	results := []runner.Result{
		{Model: "m1", TestID: "t1", Score: eval.Score{Value: 0, Detail: "TRUNCATED: got \"\""}, FinishReason: "length"},
	}
	var buf bytes.Buffer
	if err := Render(&buf, FormatTable, tests, []string{"m1"}, results); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "0.00 (0tok)!") {
		t.Errorf("table output missing truncated \"!\" suffix:\n%s", out)
	}
	if !strings.Contains(out, truncatedLegend) {
		t.Errorf("table output missing the truncation legend:\n%s", out)
	}
}

func TestRenderTable_NoLegendWhenNothingTruncated(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, FormatTable, sampleTests(), []string{"m1", "m2"}, sampleResults()); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()
	if strings.Contains(out, truncatedLegend) {
		t.Errorf("table output should not print the truncation legend when nothing truncated:\n%s", out)
	}
}

func TestRenderMarkdown_TruncatedCellsGetSuffixAndLegend(t *testing.T) {
	tests := []testkit.Test{{ID: "t1", Category: "c", Subcategory: "s"}}
	results := []runner.Result{
		{Model: "m1", TestID: "t1", Score: eval.Score{Value: 0}, FinishReason: "length"},
	}
	var buf bytes.Buffer
	if err := Render(&buf, FormatMarkdown, tests, []string{"m1"}, results); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "0.00 (0tok)!") {
		t.Errorf("markdown output missing truncated \"!\" suffix:\n%s", out)
	}
	if !strings.Contains(out, truncatedLegend) {
		t.Errorf("markdown output missing the truncation legend:\n%s", out)
	}
}

func TestRenderJSON_IncludesResponseTextFinishReasonAndTruncated(t *testing.T) {
	tests := []testkit.Test{{ID: "t1", Category: "c"}}
	results := []runner.Result{
		{Model: "m1", TestID: "t1", Score: eval.Score{Value: 0, Detail: "TRUNCATED: got \"\""}, ResponseText: "partial answer", FinishReason: "length"},
	}
	var buf bytes.Buffer
	if err := Render(&buf, FormatJSON, tests, []string{"m1"}, results); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var doc jsonArtifact
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; output:\n%s", err, buf.String())
	}
	if len(doc.Results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(doc.Results))
	}
	got := doc.Results[0]
	if got.ResponseText != "partial answer" {
		t.Errorf("ResponseText = %q, want %q", got.ResponseText, "partial answer")
	}
	if got.FinishReason != "length" {
		t.Errorf("FinishReason = %q, want length", got.FinishReason)
	}
	if !got.Truncated {
		t.Error("Truncated = false, want true")
	}
	if !strings.HasPrefix(got.Detail, "TRUNCATED: ") {
		t.Errorf("Detail = %q, want a TRUNCATED: prefix", got.Detail)
	}
}

func TestRender_UnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, Format("bogus"), sampleTests(), []string{"m1"}, sampleResults())
	if err == nil {
		t.Fatal("Render() error = nil, want error for unknown format")
	}
}

func TestSummarize_Buckets(t *testing.T) {
	results := []runner.Result{
		{Model: "m1", TestID: "a", Score: eval.Score{Value: 1}},
		{Model: "m1", TestID: "b", Score: eval.Score{Value: 0.5}},
		{Model: "m1", TestID: "c", Score: eval.Score{Value: 0}},
		{Model: "m1", TestID: "d", Err: errors.New("fail")},
		{Model: "m1", TestID: "e", Score: eval.Score{Skipped: true}},
	}
	summaries := summarize([]string{"m1"}, indexResults(results))
	if len(summaries) != 1 {
		t.Fatalf("summarize() len = %d, want 1", len(summaries))
	}
	ms := summaries[0]
	if ms.passed != 1 || ms.partial != 1 || ms.failed != 1 || ms.errors != 1 {
		t.Errorf("buckets = passed:%d partial:%d failed:%d errors:%d, want 1,1,1,1", ms.passed, ms.partial, ms.failed, ms.errors)
	}
	wantMean := (1.0 + 0.5 + 0.0) / 3.0
	if ms.overallMean != wantMean {
		t.Errorf("overallMean = %v, want %v", ms.overallMean, wantMean)
	}
}

func TestSummarize_AvgTokPerSec(t *testing.T) {
	// Aggregate ratio (sum completion tokens / sum latency seconds), not a
	// mean of per-test ratios: a fast, short response (10 tok / 0.1s = 100
	// tok/s) and a slow, long one (100 tok / 2s = 50 tok/s) must average to
	// 110 tok / 2.1s, not (100+50)/2.
	results := []runner.Result{
		{Model: "m1", TestID: "fast", Score: eval.Score{Value: 1}, Latency: 100 * time.Millisecond, CompletionTokens: 10},
		{Model: "m1", TestID: "slow", Score: eval.Score{Value: 1}, Latency: 2 * time.Second, CompletionTokens: 100},
		{Model: "m1", TestID: "err", Err: errors.New("fail"), Latency: time.Second, CompletionTokens: 999},
		{Model: "m1", TestID: "skipped", Score: eval.Score{Skipped: true}, Latency: time.Second, CompletionTokens: 999},
	}
	summaries := summarize([]string{"m1"}, indexResults(results))
	ms := summaries[0]

	want := 110.0 / 2.1
	if math.Abs(ms.avgTokPerSec-want) > 1e-9 {
		t.Errorf("avgTokPerSec = %v, want %v (errored/skipped results must be excluded)", ms.avgTokPerSec, want)
	}
}

func TestSummarize_AvgTokPerSec_NoDataIsNaN(t *testing.T) {
	results := []runner.Result{
		{Model: "m1", TestID: "err", Err: errors.New("fail")},
		{Model: "m1", TestID: "skipped", Score: eval.Score{Skipped: true}},
	}
	summaries := summarize([]string{"m1"}, indexResults(results))
	if !math.IsNaN(summaries[0].avgTokPerSec) {
		t.Errorf("avgTokPerSec = %v, want NaN when there is no timed data", summaries[0].avgTokPerSec)
	}
	if rateText(summaries[0].avgTokPerSec) != "N/A" {
		t.Errorf("rateText(NaN) = %q, want N/A", rateText(summaries[0].avgTokPerSec))
	}
}

func TestDurationText(t *testing.T) {
	if got := durationText(0, false); got != "N/A" {
		t.Errorf("durationText(0, false) = %q, want N/A", got)
	}
	if got := durationText(0, true); got != "0s" {
		t.Errorf("durationText(0, true) = %q, want 0s (a real zero-latency measurement, not lack of data)", got)
	}
	if got := durationText(42*time.Millisecond, true); got != "42ms" {
		t.Errorf("durationText(42ms, true) = %q, want 42ms", got)
	}
}

func TestSummarize_MeanLatency_NoDataIsNA(t *testing.T) {
	// N6 regression: a model with only errored/skipped results (no scored
	// data at all) must render MEAN_LATENCY as N/A, not the zero Duration's
	// misleading "0s".
	results := []runner.Result{
		{Model: "m1", TestID: "err", Err: errors.New("fail")},
		{Model: "m1", TestID: "skipped", Score: eval.Score{Skipped: true}},
	}
	var buf bytes.Buffer
	if err := Render(&buf, FormatTable, []testkit.Test{{ID: "err"}, {ID: "skipped"}}, []string{"m1"}, results); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "N/A") {
		t.Errorf("table output missing N/A for mean latency with no scored data:\n%s", out)
	}
	if strings.Contains(out, "0s") {
		t.Errorf("table output should not render a misleading 0s mean latency:\n%s", out)
	}
}

func TestCategoryRollup_ExcludesErrorsAndSkips(t *testing.T) {
	tests := []testkit.Test{
		{ID: "a", Category: "cat"},
		{ID: "b", Category: "cat"},
		{ID: "c", Category: "cat"},
	}
	results := []runner.Result{
		{Model: "m1", TestID: "a", Score: eval.Score{Value: 1}},
		{Model: "m1", TestID: "b", Err: errors.New("fail")},
		{Model: "m1", TestID: "c", Score: eval.Score{Skipped: true}},
	}
	idx := indexResults(results)
	rollup := categoryRollup(tests, []string{"m1"}, idx)
	if len(rollup) != 1 {
		t.Fatalf("categoryRollup() len = %d, want 1", len(rollup))
	}
	if rollup[0].means["m1"] != 1 {
		t.Errorf("mean = %v, want 1 (error/skip excluded)", rollup[0].means["m1"])
	}
}

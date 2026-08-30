package report

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/lukaszraczylo/llm-testbench/internal/runner"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// jsonResult is the JSON-serializable projection of one runner.Result
// (one attempt), joined with its test's category/subcategory.
type jsonResult struct {
	Model            string  `json:"model"`
	TestID           string  `json:"test_id"`
	Category         string  `json:"category,omitempty"`
	Subcategory      string  `json:"subcategory,omitempty"`
	Detail           string  `json:"detail,omitempty"`
	Error            string  `json:"error,omitempty"`
	ResponseText     string  `json:"response_text"`
	FinishReason     string  `json:"finish_reason,omitempty"`
	Score            float64 `json:"score"`
	LatencyMS        int64   `json:"latency_ms"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	Tokens           int     `json:"tokens"`
	Skipped          bool    `json:"skipped"`
	Truncated        bool    `json:"truncated"`
	Attempt          int     `json:"attempt,omitempty"`
}

// jsonArtifact is the --out / --format json document: every raw attempt
// plus the per-test discrimination rollup, so downstream tooling
// (compare) can re-derive stability without re-running the suite.
type jsonArtifact struct {
	Results []jsonResult  `json:"results"`
	Stats   []jsonStatRow `json:"stats,omitempty"`
}

// jsonStatRow is the JSON projection of testStat.
type jsonStatRow struct {
	TestID      string  `json:"test_id"`
	Category    string  `json:"category"`
	Subcategory string  `json:"subcategory"`
	Mean        float64 `json:"mean"`
	Spread      float64 `json:"spread,omitempty"`
	Unstable    bool    `json:"unstable,omitempty"`
	Truncated   bool    `json:"truncated,omitempty"`
}

// renderJSON dumps the run as a JSON artifact document.
func renderJSON(w io.Writer, tests []testkit.Test, models []string, results []runner.Result) error {
	artifact := buildArtifact(tests, models, results)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(artifact)
}

func buildArtifact(tests []testkit.Test, models []string, results []runner.Result) jsonArtifact {
	testByID := make(map[string]testkit.Test, len(tests))
	for _, t := range tests {
		testByID[t.ID] = t
	}

	out := jsonArtifact{Results: make([]jsonResult, 0, len(results))}
	for _, r := range results {
		t := testByID[r.TestID]
		jr := jsonResult{
			Model:            r.Model,
			TestID:           r.TestID,
			Category:         t.Category,
			Subcategory:      t.Subcategory,
			Score:            r.Score.Value,
			Detail:           r.Score.Detail,
			ResponseText:     r.ResponseText,
			FinishReason:     r.FinishReason,
			Skipped:          r.Score.Skipped,
			Truncated:        r.Truncated(),
			LatencyMS:        r.Latency.Milliseconds(),
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			Tokens:           r.TotalTokens(),
			Attempt:          r.Attempt,
		}
		if r.Err != nil {
			jr.Error = r.Err.Error()
		}
		out.Results = append(out.Results, jr)
	}

	idx := indexResults(results)
	for _, st := range testStats(tests, models, idx) {
		if !st.scored {
			continue
		}
		row := jsonStatRow{
			TestID:      st.testID,
			Category:    st.category,
			Subcategory: st.subcategory,
			Mean:        round6(st.mean),
			Unstable:    st.unstable,
			Truncated:   st.truncated,
		}
		if !math.IsNaN(st.spread) {
			row.Spread = round6(st.spread)
		}
		out.Stats = append(out.Stats, row)
	}
	return out
}

func round6(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

// Artifact is a saved run document: every raw attempt plus the per-test
// discrimination rollup. Exported so the compare command can consume it.
type Artifact = jsonArtifact

// AttemptResult is one saved attempt. Exported alias of jsonResult.
type AttemptResult = jsonResult

// LoadArtifact reads a --out JSON artifact written by a previous run.
// Accepts both the current document shape and the legacy bare array of
// results, so artifacts from earlier releases remain comparable.
func LoadArtifact(path string) (jsonArtifact, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is the operator's own `llmtest compare` argument, not external input
	if err != nil {
		return jsonArtifact{}, err
	}

	var probe struct {
		Results []jsonResult `json:"results"`
	}
	if err := json.Unmarshal(b, &probe); err == nil && probe.Results != nil {
		var a jsonArtifact
		if err := json.Unmarshal(b, &a); err != nil {
			return jsonArtifact{}, fmt.Errorf("%s: %w", path, err)
		}
		return a, nil
	}

	var legacy []jsonResult
	if err := json.Unmarshal(b, &legacy); err != nil {
		return jsonArtifact{}, fmt.Errorf("%s: not a llmtest json artifact: %w", path, err)
	}
	return jsonArtifact{Results: legacy}, nil
}

// WriteArtifact saves the run as a JSON artifact at path, for later
// `llmtest compare` runs, regardless of the stdout report format.
func WriteArtifact(path string, tests []testkit.Test, models []string, results []runner.Result) error {
	b, err := json.MarshalIndent(buildArtifact(tests, models, results), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644) //nolint:gosec // 0o644: the artifact is a report meant to be shared/committed, not a secret file
}

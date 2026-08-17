package report

import (
	"encoding/json"
	"io"

	"github.com/lukaszraczylo/llm-testbench/internal/runner"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// jsonResult is the JSON-serializable projection of a runner.Result, joined
// with its test's category/subcategory for self-contained output.
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
}

// renderJSON dumps raw results as a JSON array, joined with test metadata.
func renderJSON(w io.Writer, tests []testkit.Test, results []runner.Result) error {
	testByID := make(map[string]testkit.Test, len(tests))
	for _, t := range tests {
		testByID[t.ID] = t
	}

	out := make([]jsonResult, 0, len(results))
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
		}
		if r.Err != nil {
			jr.Error = r.Err.Error()
		}
		out = append(out, jr)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

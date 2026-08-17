// Package report renders runner.Result sets as a comparison table, GitHub
// markdown tables, or raw JSON.
package report

import (
	"fmt"
	"io"
	"math"
	"sort"
	"time"

	"github.com/lukaszraczylo/llm-testbench/internal/runner"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// Format selects the output renderer.
type Format string

// Supported Format values.
const (
	FormatTable    Format = "table"
	FormatMarkdown Format = "markdown"
	FormatJSON     Format = "json"
)

// passThreshold is the minimum score counted as a full pass in the model
// summary table.
const passThreshold = 0.99

// Render writes tests/models/results to w in the requested format.
func Render(w io.Writer, format Format, tests []testkit.Test, models []string, results []runner.Result) error {
	switch format {
	case FormatTable:
		return renderTable(w, tests, models, results)
	case FormatMarkdown:
		return renderMarkdown(w, tests, models, results)
	case FormatJSON:
		return renderJSON(w, tests, results)
	default:
		return fmt.Errorf("report: unknown format %q", format)
	}
}

// resultIndex looks up a Result by testID then model.
type resultIndex map[string]map[string]runner.Result

func indexResults(results []runner.Result) resultIndex {
	idx := make(resultIndex)
	for _, r := range results {
		if idx[r.TestID] == nil {
			idx[r.TestID] = make(map[string]runner.Result)
		}
		idx[r.TestID][r.Model] = r
	}
	return idx
}

// sortedTests returns tests ordered by Category, Subcategory, then ID, for
// deterministic, human-grouped table output.
func sortedTests(tests []testkit.Test) []testkit.Test {
	out := make([]testkit.Test, len(tests))
	copy(out, tests)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		if out[i].Subcategory != out[j].Subcategory {
			return out[i].Subcategory < out[j].Subcategory
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// cellText renders one (test, model) cell for the score table: "score
// (Ntok)" for a scored result, or ERR/skip/N/A otherwise.
func cellText(r runner.Result, found bool) string {
	switch {
	case !found:
		return "N/A"
	case r.Err != nil:
		return "ERR"
	case r.Score.Skipped:
		return "skip"
	default:
		return fmt.Sprintf("%.2f (%dtok)", r.Score.Value, r.TotalTokens())
	}
}

// categoryMean holds the per-model mean score for one category, computed
// over non-skipped, non-error results only.
type categoryMean struct {
	means    map[string]float64
	category string
}

func categoryRollup(tests []testkit.Test, models []string, idx resultIndex) []categoryMean {
	order := make([]string, 0)
	byCategory := make(map[string][]testkit.Test)
	for _, t := range sortedTests(tests) {
		if _, seen := byCategory[t.Category]; !seen {
			order = append(order, t.Category)
		}
		byCategory[t.Category] = append(byCategory[t.Category], t)
	}

	out := make([]categoryMean, 0, len(order))
	for _, cat := range order {
		cm := categoryMean{category: cat, means: make(map[string]float64, len(models))}
		for _, model := range models {
			var sum float64
			var n int
			for _, t := range byCategory[cat] {
				r, ok := idx[t.ID][model]
				if !ok || r.Err != nil || r.Score.Skipped {
					continue
				}
				sum += r.Score.Value
				n++
			}
			if n > 0 {
				cm.means[model] = sum / float64(n)
			} else {
				cm.means[model] = math.NaN()
			}
		}
		out = append(out, cm)
	}
	return out
}

// meanCellText renders a categoryMean/modelSummary mean cell, showing N/A
// when there is no scored data.
func meanCellText(v float64) string {
	if math.IsNaN(v) {
		return "N/A"
	}
	return fmt.Sprintf("%.2f", v)
}

// rateText renders a modelSummary avgTokPerSec cell, showing N/A when there
// is no timed, non-skipped data to compute a rate from.
func rateText(v float64) string {
	if math.IsNaN(v) {
		return "N/A"
	}
	return fmt.Sprintf("%.1f", v)
}

// durationText renders a modelSummary meanLatency cell, showing N/A when
// there is no scored data (N6): meanLatency's zero value, printed via
// time.Duration.String(), would otherwise misleadingly read as a real "0s"
// mean latency rather than "no data". hasData mirrors overallMean's NaN
// check, since both are set together (or left unset together) in
// summarize.
func durationText(d time.Duration, hasData bool) string {
	if !hasData {
		return "N/A"
	}
	return d.String()
}

// modelSummary aggregates one model's results across the whole run.
type modelSummary struct {
	model        string
	overallMean  float64 // math.NaN() when no scored data
	passed       int
	partial      int
	failed       int
	errors       int
	meanLatency  time.Duration
	totalTokens  int
	avgTokPerSec float64 // math.NaN() when no timed, non-skipped data
}

func summarize(models []string, results []runner.Result) []modelSummary {
	byModel := make(map[string][]runner.Result, len(models))
	for _, r := range results {
		byModel[r.Model] = append(byModel[r.Model], r)
	}

	out := make([]modelSummary, 0, len(models))
	for _, model := range models {
		ms := modelSummary{model: model}
		var scoreSum float64
		var scoredCount int
		var latencySum time.Duration
		var latencyCount int
		var completionTokenSum int
		var latencySecondsSum float64

		for _, r := range byModel[model] {
			ms.totalTokens += r.TotalTokens()
			switch {
			case r.Err != nil:
				ms.errors++
				continue
			case r.Score.Skipped:
				continue
			}

			scoreSum += r.Score.Value
			scoredCount++
			latencySum += r.Latency
			latencyCount++
			completionTokenSum += r.CompletionTokens
			latencySecondsSum += r.Latency.Seconds()

			switch {
			case r.Score.Value >= passThreshold:
				ms.passed++
			case r.Score.Value <= 0:
				ms.failed++
			default:
				ms.partial++
			}
		}

		if scoredCount > 0 {
			ms.overallMean = scoreSum / float64(scoredCount)
			ms.meanLatency = latencySum / time.Duration(latencyCount)
		} else {
			ms.overallMean = math.NaN()
		}

		// Aggregate ratio (sum of completion tokens over sum of elapsed
		// seconds), not a mean of per-test ratios: robust to a handful of
		// very short responses skewing a per-test-averaged rate.
		if latencySecondsSum > 0 {
			ms.avgTokPerSec = float64(completionTokenSum) / latencySecondsSum
		} else {
			ms.avgTokPerSec = math.NaN()
		}

		out = append(out, ms)
	}
	return out
}

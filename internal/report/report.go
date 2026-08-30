// Package report renders runner.Result sets as a comparison table, GitHub
// markdown tables, or raw JSON.
package report

import (
	"fmt"
	"io"
	"math"
	"sort"
	"time"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/llm"
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
		return renderJSON(w, tests, models, results)
	default:
		return fmt.Errorf("report: unknown format %q", format)
	}
}

// modelCell is the aggregate view of every attempt recorded for one
// (testID, model) pair: with Config.Repeat = 1 this is the single result;
// with repeats the Score is the mean over scored attempts, and minScore/
// maxScore expose the within-model spread (instability) that repeats are
// measured to find.
type modelCell struct {
	result     runner.Result // representative: last scored attempt, mean score patched in
	minScore   float64
	maxScore   float64
	meanTokens float64
	attempts   int
	scored     int
	skippedAll bool
	errorAll   bool
	ok         bool
}

// cellIndex looks up the aggregate cell for a testID then model.
type cellIndex map[string]map[string]modelCell

func indexResults(results []runner.Result) cellIndex {
	type key struct{ testID, model string }
	grouped := make(map[key][]runner.Result)
	order := make([]key, 0, len(results))
	for _, r := range results {
		k := key{r.TestID, r.Model}
		if _, seen := grouped[k]; !seen {
			order = append(order, k)
		}
		grouped[k] = append(grouped[k], r)
	}

	idx := make(cellIndex)
	for _, k := range order {
		attempts := grouped[k]
		sort.Slice(attempts, func(i, j int) bool { return attempts[i].Attempt < attempts[j].Attempt })

		c := modelCell{ok: true, attempts: len(attempts)}
		// Truncation policy: a truncated attempt was scored on partial
		// text, so its score measures the token budget as much as the
		// model. Attempts that finished normally are therefore preferred
		// for the cell mean (and for the min-max instability range); only
		// when EVERY scored attempt was cut off does the mean fall back to
		// the truncated scores. A cell is flagged "!" whenever any attempt
		// was truncated, clean mean or not.
		var sum, toks, cleanSum, cleanToks float64
		var cleanN, scoredN int
		var cleanMin, cleanMax float64
		var anyTrunc bool
		var last, lastClean runner.Result
		for _, a := range attempts {
			last = a
			switch {
			case a.Err != nil:
				continue
			case a.Score.Skipped:
				continue
			}
			v := a.Score.Value
			sum += v
			toks += float64(a.TotalTokens())
			scoredN++
			if a.Truncated() {
				anyTrunc = true
				continue
			}
			if cleanN == 0 {
				cleanMin, cleanMax = v, v
			} else {
				cleanMin = math.Min(cleanMin, v)
				cleanMax = math.Max(cleanMax, v)
			}
			cleanSum += v
			cleanToks += float64(a.TotalTokens())
			cleanN++
			lastClean = a
		}
		c.scored = scoredN

		rep := last
		if cleanN > 0 {
			rep = lastClean
		}
		switch {
		case scoredN > 0 && cleanN > 0:
			rep.Score = eval.Score{
				Value:  cleanSum / float64(cleanN),
				Detail: lastClean.Score.Detail,
			}
			c.minScore, c.maxScore = cleanMin, cleanMax
			c.meanTokens = cleanToks / float64(cleanN)
			if anyTrunc {
				rep.FinishReason = llm.FinishReasonLength
			}
		case scoredN > 0:
			rep.Score = eval.Score{
				Value:  sum / float64(scoredN),
				Detail: last.Score.Detail,
			}
			c.minScore, c.maxScore = rep.Score.Value, rep.Score.Value
			c.meanTokens = toks / float64(scoredN)
			rep.FinishReason = llm.FinishReasonLength
		case c.attempts > 0 && last.Score.Skipped:
			c.skippedAll = true
		case c.attempts > 0 && last.Err != nil:
			c.errorAll = true
		}

		c.result = rep
		if idx[k.testID] == nil {
			idx[k.testID] = make(map[string]modelCell)
		}
		idx[k.testID][k.model] = c
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

// truncatedSuffix marks a cell where any attempt was cut off by the token
// budget (finish_reason=length). Per the truncation policy in
// indexResults, the cell mean then comes from the attempts that finished
// normally; "!" warns that at least one sample was discarded (or, when no
// attempt finished, that the whole mean is partial-text based).
// truncatedLegend is the one-line explanation printed once per
// table/markdown report.
const (
	truncatedSuffix = "!"
	truncatedLegend = "! = some attempt was truncated by the token budget (finish_reason=length); the mean uses only attempts that finished, increase max_tokens_default if every attempt is flagged"
)

// cellText renders one (test, model) cell for the score table: "score
// (Ntok)" for a scored result, or ERR/skip/N/A otherwise, with a
// truncatedSuffix appended when any attempt was cut off by the token
// budget. With repeat attempts, a "[min-max]" range is appended when the
// attempts disagreed, marking the cell unstable.
func cellText(c modelCell) string {
	if !c.ok {
		return "N/A"
	}
	switch {
	case c.errorAll:
		return "ERR"
	case c.skippedAll:
		return "skip"
	case c.scored == 0:
		return "N/A"
	}

	text := fmt.Sprintf("%.2f (%dtok)", c.result.Score.Value, int(math.Round(c.meanTokens)))
	if c.attempts > 1 && c.maxScore > c.minScore {
		text += fmt.Sprintf(" [%0.2f-%0.2f]", c.minScore, c.maxScore)
	}
	if c.result.Truncated() {
		text += truncatedSuffix
	}
	return text
}

// categoryMean holds the per-model mean score for one category, computed
// over non-skipped, non-error results only.
type categoryMean struct {
	means    map[string]float64
	category string
}

func categoryRollup(tests []testkit.Test, models []string, idx cellIndex) []categoryMean {
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
				c, ok := idx[t.ID][model]
				if !ok || c.errorAll || c.skippedAll || c.scored == 0 {
					continue
				}
				sum += c.result.Score.Value
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

// summarize aggregates one model's results per test (attempts already
// merged by indexResults), so pass/partial/fail counts stay per-test even
// with Config.Repeat > 1.
func summarize(models []string, idx cellIndex) []modelSummary {
	out := make([]modelSummary, 0, len(models))
	for _, model := range models {
		ms := modelSummary{model: model}
		var scoreSum, latencySecondsSum, meanTokenSum float64
		var scoredCount, latencyCount int
		var latencySum time.Duration

		for _, cells := range idx {
			c, ok := cells[model]
			if !ok {
				continue
			}
			meanTokenSum += c.meanTokens * float64(max(c.scored, 1))
			if c.errorAll {
				ms.errors += c.attempts
				continue
			}
			if c.skippedAll || c.scored == 0 {
				continue
			}

			v := c.result.Score.Value
			scoreSum += v
			scoredCount++
			latencySum += c.result.Latency
			latencyCount++
			latencySecondsSum += c.result.Latency.Seconds()

			switch {
			case v >= passThreshold:
				ms.passed++
			case v <= 0:
				ms.failed++
			default:
				ms.partial++
			}
		}

		ms.totalTokens = int(math.Round(meanTokenSum))
		if scoredCount > 0 {
			ms.overallMean = scoreSum / float64(scoredCount)
			ms.meanLatency = latencySum / time.Duration(latencyCount)
		} else {
			ms.overallMean = math.NaN()
		}

		// Aggregate ratio (completion tokens over elapsed seconds for the
		// representative attempt), not a mean of per-test ratios: robust to
		// a handful of very short responses skewing a per-test-averaged rate.
		if latencySecondsSum > 0 {
			ms.avgTokPerSec = float64(ms.totalTokens) / latencySecondsSum
		} else {
			ms.avgTokPerSec = math.NaN()
		}

		out = append(out, ms)
	}
	return out
}

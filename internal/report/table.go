package report

import (
	"fmt"
	"io"
	"math"
	"strings"
	"text/tabwriter"

	"github.com/lukaszraczylo/llm-testbench/internal/runner"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// errWriter wraps an io.Writer and remembers the first write error, so a
// sequence of Fprint* calls can be written without checking after every
// line; the caller checks ew.err once at the end.
type errWriter struct {
	w   io.Writer
	err error
}

func (ew *errWriter) println(args ...any) {
	if ew.err != nil {
		return
	}
	_, ew.err = fmt.Fprintln(ew.w, args...)
}

// renderTable writes three plain-text tables: per-test scores, category
// rollup, and model summary.
func renderTable(w io.Writer, tests []testkit.Test, models []string, results []runner.Result) error {
	idx := indexResults(results)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	ew := &errWriter{w: tw}

	writeScoreTable(ew, tests, models, idx)
	ew.println()
	writeCategoryTable(ew, tests, models, idx)
	ew.println()
	writeSummaryTable(ew, models, results)
	if ew.err != nil {
		return ew.err
	}

	return tw.Flush()
}

func writeScoreTable(ew *errWriter, tests []testkit.Test, models []string, idx resultIndex) {
	ew.println("== Per-test scores ==")
	header := []string{"TEST", "CATEGORY", "SUBCATEGORY"}
	header = append(header, models...)
	ew.println(strings.Join(header, "\t"))

	for _, t := range sortedTests(tests) {
		row := []string{t.ID, t.Category, t.Subcategory}
		for _, model := range models {
			r, ok := idx[t.ID][model]
			row = append(row, cellText(r, ok))
		}
		ew.println(strings.Join(row, "\t"))
	}
}

func writeCategoryTable(ew *errWriter, tests []testkit.Test, models []string, idx resultIndex) {
	ew.println("== Category rollup (mean score) ==")
	header := append([]string{"CATEGORY"}, models...)
	ew.println(strings.Join(header, "\t"))

	for _, cm := range categoryRollup(tests, models, idx) {
		row := []string{cm.category}
		for _, model := range models {
			row = append(row, meanCellText(cm.means[model]))
		}
		ew.println(strings.Join(row, "\t"))
	}
}

func writeSummaryTable(ew *errWriter, models []string, results []runner.Result) {
	ew.println("== Model summary ==")
	ew.println(strings.Join([]string{"MODEL", "MEAN", "PASSED", "PARTIAL", "FAILED", "ERRORS", "MEAN_LATENCY", "TOTAL_TOKENS", "TOK/S"}, "\t"))

	for _, ms := range summarize(models, results) {
		row := []string{
			ms.model,
			meanCellText(ms.overallMean),
			fmt.Sprintf("%d", ms.passed),
			fmt.Sprintf("%d", ms.partial),
			fmt.Sprintf("%d", ms.failed),
			fmt.Sprintf("%d", ms.errors),
			durationText(ms.meanLatency, !math.IsNaN(ms.overallMean)),
			fmt.Sprintf("%d", ms.totalTokens),
			rateText(ms.avgTokPerSec),
		}
		ew.println(strings.Join(row, "\t"))
	}
}

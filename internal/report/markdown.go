package report

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/runner"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// renderMarkdown writes the same four sections as renderTable, formatted
// as GitHub-flavored markdown tables.
func renderMarkdown(w io.Writer, tests []testkit.Test, models []string, results []runner.Result) error {
	idx := indexResults(results)
	ew := &errWriter{w: w}

	ew.println("## Per-test scores")
	ew.println()
	scoreHeader := append([]string{"Test", "Category", "Subcategory"}, models...)
	writeMarkdownHeader(ew, scoreHeader)
	anyTruncated := false
	for _, t := range sortedTests(tests) {
		row := []string{t.ID, t.Category, t.Subcategory}
		for _, model := range models {
			c := idx[t.ID][model]
			row = append(row, cellText(c))
			anyTruncated = anyTruncated || c.result.Truncated()
		}
		writeMarkdownRow(ew, row)
	}
	if anyTruncated {
		ew.println()
		ew.println(truncatedLegend)
	}

	ew.println()
	ew.println("## Category rollup (mean score)")
	ew.println()
	catHeader := append([]string{"Category"}, models...)
	writeMarkdownHeader(ew, catHeader)
	for _, cm := range categoryRollup(tests, models, idx) {
		row := []string{cm.category}
		for _, model := range models {
			row = append(row, meanCellText(cm.means[model]))
		}
		writeMarkdownRow(ew, row)
	}

	ew.println()
	ew.println("## Discrimination")
	ew.println()
	writeDiscriminationRows(markdownPrinter{ew: ew}, testStats(tests, models, idx), len(models) > 1)

	ew.println()
	ew.println("## Model summary")
	ew.println()
	writeMarkdownHeader(ew, []string{"Model", "Mean", "Passed", "Partial", "Failed", "Errors", "Mean latency", "Total tokens", "Tok/s"})
	for _, ms := range summarize(models, idx) {
		writeMarkdownRow(ew, []string{
			ms.model,
			meanCellText(ms.overallMean),
			fmt.Sprintf("%d", ms.passed),
			fmt.Sprintf("%d", ms.partial),
			fmt.Sprintf("%d", ms.failed),
			fmt.Sprintf("%d", ms.errors),
			durationText(ms.meanLatency, !math.IsNaN(ms.overallMean)),
			fmt.Sprintf("%d", ms.totalTokens),
			rateText(ms.avgTokPerSec),
		})
	}

	return ew.err
}

// markdownPrinter adapts writeDiscriminationRows to pipe-delimited rows.
// GFM needs a blank line between the note paragraph and the table, so
// header() emits one first.
type markdownPrinter struct{ ew *errWriter }

func (mp markdownPrinter) header(cells ...string) {
	mp.ew.println()
	writeMarkdownHeader(mp.ew, cells)
}
func (mp markdownPrinter) row(cells ...string) { writeMarkdownRow(mp.ew, cells) }
func (mp markdownPrinter) note(text string)    { mp.ew.println(text) }

func writeMarkdownHeader(ew *errWriter, cols []string) {
	ew.println("| " + strings.Join(cols, " | ") + " |")
	sep := make([]string, len(cols))
	for i := range sep {
		sep[i] = "---"
	}
	ew.println("| " + strings.Join(sep, " | ") + " |")
}

func writeMarkdownRow(ew *errWriter, cols []string) {
	ew.println("| " + strings.Join(cols, " | ") + " |")
}

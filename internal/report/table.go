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

// renderTable writes four plain-text tables: per-test scores, category
// rollup, discrimination/stability rollup, and model summary.
func renderTable(w io.Writer, tests []testkit.Test, models []string, results []runner.Result) error {
	idx := indexResults(results)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	ew := &errWriter{w: tw}

	writeScoreTable(ew, tests, models, idx)
	ew.println()
	writeCategoryTable(ew, tests, models, idx)
	ew.println()
	writeDiscriminationTable(ew, testStats(tests, models, idx), len(models) > 1)
	ew.println()
	writeSummaryTable(ew, models, idx)
	if ew.err != nil {
		return ew.err
	}

	return tw.Flush()
}

func writeScoreTable(ew *errWriter, tests []testkit.Test, models []string, idx cellIndex) {
	ew.println("== Per-test scores ==")
	header := []string{"TEST", "CATEGORY", "SUBCATEGORY"}
	header = append(header, models...)
	ew.println(strings.Join(header, "\t"))

	anyTruncated := false
	for _, t := range sortedTests(tests) {
		row := []string{t.ID, t.Category, t.Subcategory}
		for _, model := range models {
			c := idx[t.ID][model]
			row = append(row, cellText(c))
			anyTruncated = anyTruncated || c.result.Truncated()
		}
		ew.println(strings.Join(row, "\t"))
	}
	if anyTruncated {
		ew.println(truncatedLegend)
	}
}

func writeCategoryTable(ew *errWriter, tests []testkit.Test, models []string, idx cellIndex) {
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

// writeDiscriminationTable lists the tests that actually measure
// something: cross-model spread >= DiscriminationSpread, or attempts that
// disagreed under --repeat. With a single model there is no spread to
// speak of, so the non-passing tests are listed instead.
func writeDiscriminationTable(ew *errWriter, stats []testStat, multiModel bool) {
	writeDiscriminationRows(tablePrinter{ew: ew}, stats, multiModel)
}

// discriminationSink abstracts the tab (table) vs pipe (markdown) output
// so the discrimination section is written once.
type discriminationSink interface {
	header(cells ...string)
	row(cells ...string)
	note(text string)
}

type tablePrinter struct{ ew *errWriter }

func (tp tablePrinter) header(cells ...string) { tp.ew.println(strings.Join(cells, "\t")) }
func (tp tablePrinter) row(cells ...string)    { tp.ew.println(strings.Join(cells, "\t")) }
func (tp tablePrinter) note(text string)       { tp.ew.println(text) }

func writeDiscriminationRows(sink discriminationSink, stats []testStat, multiModel bool) {
	discriminating, unstable, scored := 0, 0, 0
	for _, st := range stats {
		if !st.scored {
			continue
		}
		scored++
		if st.unstable {
			unstable++
		}
		if multiModel && !math.IsNaN(st.spread) && st.spread >= DiscriminationSpread {
			discriminating++
		}
	}

	if multiModel {
		sink.note(fmt.Sprintf("== Discrimination: %d/%d scored tests separate models by >= %.2f; %d unstable under repeat ==",
			discriminating, scored, DiscriminationSpread, unstable))
	} else {
		sink.note(fmt.Sprintf("== Discrimination: single model (no cross-model spread); %d unstable under repeat ==", unstable))
	}

	rows := interestingStats(stats, multiModel)
	if len(rows) == 0 {
		sink.note("no discriminating or unstable tests: every scored test passed identically for every model")
		return
	}

	sink.header("TEST", "CATEGORY", "SUBCATEGORY", "MEAN", "SPREAD", "UNSTABLE", "TRUNC")
	for _, st := range rows {
		sink.row(
			st.testID,
			st.category,
			st.subcategory,
			meanCellText(st.mean),
			meanCellText(st.spread),
			yesNo(st.unstable),
			yesNo(st.truncated),
		)
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "-"
}

func writeSummaryTable(ew *errWriter, models []string, idx cellIndex) {
	ew.println("== Model summary ==")
	ew.println(strings.Join([]string{"MODEL", "MEAN", "PASSED", "PARTIAL", "FAILED", "ERRORS", "MEAN_LATENCY", "TOTAL_TOKENS", "TOK/S"}, "\t"))

	for _, ms := range summarize(models, idx) {
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

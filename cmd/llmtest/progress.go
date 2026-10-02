package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lukaszraczylo/llm-testbench/internal/runner"
)

// pipedProgressEvery is how often a non-terminal stderr gets a progress
// line. A piped log stays readable without one line per request.
const pipedProgressEvery = 25

// pipedProgressInterval also forces a piped line after this much time, so
// a run of slow models still logs progress between the count-based lines.
const pipedProgressInterval = time.Minute

// barWidth is the number of cells in the terminal progress bar.
const barWidth = 30

const (
	barFilled = "\u2588"
	barEmpty  = "\u2591"
)

// stderrProgressReporter implements runner.ProgressReporter, writing a
// progress bar with elapsed time and ETA to w (os.Stderr in production).
// Run's stdout stays pipeable (table/markdown/json only): all progress
// goes here instead.
//
// When w is a terminal the bar rewrites itself in place with a carriage
// return. When w is piped (a log file), rewriting would produce one
// unreadable blob, so it prints a plain line every pipedProgressEvery
// completions or pipedProgressInterval, plus the final one.
type stderrProgressReporter struct {
	w         io.Writer
	now       func() time.Time
	start     time.Time
	lastPrint time.Time
	mu        sync.Mutex
	errors    int
	isTTY     bool
}

// newStderrProgressReporter builds a stderrProgressReporter writing to w.
func newStderrProgressReporter(w io.Writer) *stderrProgressReporter {
	return &stderrProgressReporter{w: w, isTTY: isTerminal(w), now: time.Now}
}

// isTerminal reports whether w is a character device (an interactive
// terminal), using only the standard library.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// ReportStart implements runner.ProgressReporter.
func (r *stderrProgressReporter) ReportStart(totalTests, totalModels, concurrency int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.start = r.now()
	r.lastPrint = r.start
	// A write failure on this side-channel progress stream (a closed pipe,
	// a full/broken stderr) is not actionable and must not abort the run
	// that produces the actual report on stdout.
	_, _ = fmt.Fprintf(r.w, "running %d tests x %d models = %d requests, concurrency %d\n",
		totalTests, totalModels, totalTests*totalModels, concurrency)
}

// ReportDone implements runner.ProgressReporter. Safe for concurrent calls
// from up to Config.Concurrency goroutines at once: the mutex serializes
// writes so concurrent completions cannot interleave mid-line.
func (r *stderrProgressReporter) ReportDone(done, total int, res runner.Result) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if res.Err != nil {
		r.errors++
	}
	now := r.now()
	elapsed := now.Sub(r.start)

	if r.isTTY {
		// \r returns to the line start, \x1b[K clears any longer previous
		// text; the final completion ends the line so later output starts
		// clean.
		_, _ = fmt.Fprintf(r.w, "\r%s\x1b[K", r.statusLine(done, total, elapsed, true))
		if done == total {
			_, _ = fmt.Fprintln(r.w)
		}
		return
	}

	if done%pipedProgressEvery == 0 || done == total || now.Sub(r.lastPrint) >= pipedProgressInterval {
		r.lastPrint = now
		_, _ = fmt.Fprintln(r.w, r.statusLine(done, total, elapsed, false))
	}
}

// statusLine renders the progress text; withBar adds the bar for terminals.
func (r *stderrProgressReporter) statusLine(done, total int, elapsed time.Duration, withBar bool) string {
	pct := 0
	if total > 0 {
		pct = done * 100 / total
	}
	var b strings.Builder
	if withBar {
		filled := 0
		if total > 0 {
			filled = done * barWidth / total
		}
		b.WriteString(strings.Repeat(barFilled, filled) + strings.Repeat(barEmpty, barWidth-filled) + " ")
	}
	fmt.Fprintf(&b, "%3d%% %d/%d  elapsed %s", pct, done, total, formatDuration(elapsed))
	if done < total {
		fmt.Fprintf(&b, "  ETA %s", formatETA(done, total, elapsed))
	}
	if r.errors > 0 {
		fmt.Fprintf(&b, "  errors %d", r.errors)
	}
	return b.String()
}

// formatETA extrapolates the remaining time from the average completion
// rate so far; "~" marks it as an estimate and "--" means no data yet.
// Jobs vary widely in duration, so it is rough, especially early on.
func formatETA(done, total int, elapsed time.Duration) string {
	if done <= 0 {
		return "--"
	}
	remaining := time.Duration(float64(elapsed) / float64(done) * float64(total-done))
	return "~" + formatDuration(remaining)
}

// formatDuration renders d rounded to seconds as 45s, 12m04s or 1h02m.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d / time.Hour)
	m := int(d % time.Hour / time.Minute)
	sec := int(d % time.Minute / time.Second)
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%02dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm%02ds", m, sec)
	default:
		return fmt.Sprintf("%ds", sec)
	}
}

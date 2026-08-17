package main

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/lukaszraczylo/llm-testbench/internal/runner"
)

// pipedProgressEvery is how often a non-terminal stderr gets a progress
// line. A piped log stays readable without one line per request.
const pipedProgressEvery = 25

// stderrProgressReporter implements runner.ProgressReporter, writing a
// self-updating "test N of M" counter to w (os.Stderr in production).
// Run's stdout stays pipeable (table/markdown/json only): all progress
// goes here instead.
//
// When w is a terminal the counter rewrites itself in place with a
// carriage return. When w is piped (a log file), rewriting would produce
// one unreadable blob, so it prints a plain line every pipedProgressEvery
// completions plus the final one.
type stderrProgressReporter struct {
	w     io.Writer
	mu    sync.Mutex
	isTTY bool
}

// newStderrProgressReporter builds a stderrProgressReporter writing to w.
func newStderrProgressReporter(w io.Writer) *stderrProgressReporter {
	return &stderrProgressReporter{w: w, isTTY: isTerminal(w)}
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
	// A write failure on this side-channel progress stream (a closed pipe,
	// a full/broken stderr) is not actionable and must not abort the run
	// that produces the actual report on stdout.
	_, _ = fmt.Fprintf(r.w, "running %d tests x %d models = %d requests, concurrency %d\n",
		totalTests, totalModels, totalTests*totalModels, concurrency)
}

// ReportDone implements runner.ProgressReporter. Safe for concurrent calls
// from up to Config.Concurrency goroutines at once: the mutex serializes
// writes so concurrent completions cannot interleave mid-line.
func (r *stderrProgressReporter) ReportDone(done, total int, _ runner.Result) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isTTY {
		// \r returns to the line start, \x1b[K clears any longer previous
		// text; the final completion ends the line so later output starts
		// clean.
		_, _ = fmt.Fprintf(r.w, "\rtest %d of %d\x1b[K", done, total)
		if done == total {
			_, _ = fmt.Fprintln(r.w)
		}
		return
	}

	if done%pipedProgressEvery == 0 || done == total {
		_, _ = fmt.Fprintf(r.w, "test %d of %d\n", done, total)
	}
}

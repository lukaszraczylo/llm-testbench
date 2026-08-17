package main

import (
	"fmt"
	"io"
	"sync"

	"github.com/lukaszraczylo/llm-testbench/internal/runner"
)

// stderrProgressReporter implements runner.ProgressReporter, writing one
// line per event to w (os.Stderr in production). Run's stdout stays
// pipeable (table/markdown/json only): all progress goes here instead.
type stderrProgressReporter struct {
	w  io.Writer
	mu sync.Mutex
}

// newStderrProgressReporter builds a stderrProgressReporter writing to w.
func newStderrProgressReporter(w io.Writer) *stderrProgressReporter {
	return &stderrProgressReporter{w: w}
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
// each printed line so concurrent completions cannot interleave mid-line.
func (r *stderrProgressReporter) ReportDone(done, total int, result runner.Result) {
	status := resultStatus(result)
	truncated := ""
	if result.Truncated() {
		truncated = " TRUNCATED"
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// See ReportStart: a write failure here is not actionable.
	_, _ = fmt.Fprintf(r.w, "[%d/%d] model=%s test=%s score=%s tokens=%d (%s)%s\n",
		done, total, result.Model, result.TestID, status, result.TotalTokens(), result.Latency, truncated)
}

// resultStatus renders a Result's score for one progress line: the numeric
// score, or ERR/skip.
func resultStatus(result runner.Result) string {
	switch {
	case result.Err != nil:
		return "ERR"
	case result.Score.Skipped:
		return "skip"
	default:
		return fmt.Sprintf("%.2f", result.Score.Value)
	}
}

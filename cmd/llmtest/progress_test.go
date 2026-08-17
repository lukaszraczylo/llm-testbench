package main

import (
	"bytes"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/runner"
)

func TestStderrProgressReporter_ReportStart(t *testing.T) {
	var buf bytes.Buffer
	r := newStderrProgressReporter(&buf)

	r.ReportStart(113, 3, 8)

	got := buf.String()
	want := "running 113 tests x 3 models = 339 requests, concurrency 8\n"
	if got != want {
		t.Errorf("ReportStart() output = %q, want %q", got, want)
	}
}

// A bytes.Buffer is not a character device, so newStderrProgressReporter
// selects piped mode: a plain line every pipedProgressEvery completions
// plus the final one, nothing else.
func TestStderrProgressReporter_ReportDone_Piped(t *testing.T) {
	tests := []struct {
		name  string
		want  string
		done  int
		total int
	}{
		{name: "intermediate completion stays silent", done: 3, total: 10, want: ""},
		{name: "final completion always prints", done: 10, total: 10, want: "test 10 of 10\n"},
		{name: "every pipedProgressEvery-th completion prints", done: pipedProgressEvery, total: 100, want: "test 25 of 100\n"},
		{name: "multiple of pipedProgressEvery prints", done: 2 * pipedProgressEvery, total: 100, want: "test 50 of 100\n"},
		{name: "off-interval completion stays silent", done: 2*pipedProgressEvery + 1, total: 100, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := newStderrProgressReporter(&buf)
			r.ReportDone(tt.done, tt.total, runner.Result{Model: "m", TestID: "t", Score: eval.Score{Value: 1}})

			if got := buf.String(); got != tt.want {
				t.Errorf("ReportDone(%d, %d) output = %q, want %q", tt.done, tt.total, got, tt.want)
			}
		})
	}
}

// TTY mode rewrites one counter line in place; the test forces isTTY since
// a bytes.Buffer can never be a terminal.
func TestStderrProgressReporter_ReportDone_TTY(t *testing.T) {
	tests := []struct {
		name  string
		want  string
		done  int
		total int
	}{
		{name: "intermediate rewrites in place without a newline", done: 3, total: 10, want: "\rtest 3 of 10\x1b[K"},
		{name: "final completion ends the line", done: 10, total: 10, want: "\rtest 10 of 10\x1b[K\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := newStderrProgressReporter(&buf)
			r.isTTY = true
			r.ReportDone(tt.done, tt.total, runner.Result{Model: "m", TestID: "t", Score: eval.Score{Value: 1}})

			if got := buf.String(); got != tt.want {
				t.Errorf("ReportDone(%d, %d) output = %q, want %q", tt.done, tt.total, got, tt.want)
			}
		})
	}
}

// TestStderrProgressReporter_ReportDone_ConcurrencySafe writes many
// concurrent ReportDone calls (as Runner.Run's goroutines would) and
// verifies no write interleaved mid-line - the reporter's own mutex must
// serialize writes, not just the counter.
func TestStderrProgressReporter_ReportDone_ConcurrencySafe(t *testing.T) {
	var buf bytes.Buffer
	r := newStderrProgressReporter(&buf)

	const n = 2 * pipedProgressEvery
	var wg sync.WaitGroup
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r.ReportDone(i, n, runner.Result{Model: "m", TestID: "t", Score: eval.Score{Value: 1}})
		}(i)
	}
	wg.Wait()

	// In piped mode exactly the 25th and 50th completions print.
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines (%q), want 2 (a corrupted/interleaved write would change the count)", len(lines), buf.String())
	}

	linePattern := regexp.MustCompile(`^test \d+ of \d+$`)
	for _, line := range lines {
		if !linePattern.MatchString(line) {
			t.Errorf("malformed or interleaved line: %q", line)
		}
	}
}

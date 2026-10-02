package main

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

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

// fakeClock advances only when told, so elapsed/ETA are exact.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newTestReporter(buf *bytes.Buffer, tty bool) (*stderrProgressReporter, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	r := newStderrProgressReporter(buf)
	r.isTTY = tty
	r.now = clk.now
	r.ReportStart(10, 1, 1)
	buf.Reset()
	return r, clk
}

var okResult = runner.Result{Model: "m", TestID: "t", Score: eval.Score{Value: 1}}

// A bytes.Buffer is not a character device, so newStderrProgressReporter
// selects piped mode: a plain line every pipedProgressEvery completions
// (or pipedProgressInterval), plus the final one.
func TestStderrProgressReporter_ReportDone_Piped(t *testing.T) {
	tests := []struct {
		name  string
		want  string
		done  int
		total int
	}{
		{name: "intermediate completion stays silent", done: 3, total: 10, want: ""},
		{name: "final completion always prints", done: 10, total: 10, want: "100% 10/10  elapsed 10s\n"},
		{name: "every pipedProgressEvery-th completion prints", done: pipedProgressEvery, total: 100, want: " 25% 25/100  elapsed 10s  ETA ~30s\n"},
		{name: "off-interval completion stays silent", done: 2*pipedProgressEvery + 1, total: 100, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			r, clk := newTestReporter(&buf, false)
			clk.t = clk.t.Add(10 * time.Second)
			r.ReportDone(tt.done, tt.total, okResult)
			if got := buf.String(); got != tt.want {
				t.Errorf("ReportDone(%d, %d) output = %q, want %q", tt.done, tt.total, got, tt.want)
			}
		})
	}
}

func TestStderrProgressReporter_ReportDone_PipedTimeBasedLine(t *testing.T) {
	var buf bytes.Buffer
	r, clk := newTestReporter(&buf, false)
	clk.t = clk.t.Add(pipedProgressInterval - time.Second)
	r.ReportDone(1, 10, okResult)
	if buf.Len() != 0 {
		t.Fatalf("printed before the interval: %q", buf.String())
	}
	clk.t = clk.t.Add(2 * time.Second)
	r.ReportDone(2, 10, okResult)
	if !strings.Contains(buf.String(), "2/10") {
		t.Errorf("no time-based line after the interval, got %q", buf.String())
	}
}

// TTY mode rewrites one bar line in place; the test forces isTTY since a
// bytes.Buffer can never be a terminal.
func TestStderrProgressReporter_ReportDone_TTY(t *testing.T) {
	var buf bytes.Buffer
	r, clk := newTestReporter(&buf, true)
	clk.t = clk.t.Add(30 * time.Second)

	r.ReportDone(5, 10, okResult)
	want := "\r" + strings.Repeat(barFilled, 15) + strings.Repeat(barEmpty, 15) + "  50% 5/10  elapsed 30s  ETA ~30s\x1b[K"
	if got := buf.String(); got != want {
		t.Errorf("mid-run output = %q, want %q", got, want)
	}

	buf.Reset()
	r.ReportDone(10, 10, okResult)
	want = "\r" + strings.Repeat(barFilled, barWidth) + " 100% 10/10  elapsed 30s\x1b[K\n"
	if got := buf.String(); got != want {
		t.Errorf("final output = %q, want %q (full bar, no ETA, line ended)", got, want)
	}
}

func TestStderrProgressReporter_CountsErrors(t *testing.T) {
	var buf bytes.Buffer
	r, _ := newTestReporter(&buf, false)
	r.ReportDone(1, 2, runner.Result{Err: errors.New("boom")})
	r.ReportDone(2, 2, okResult)
	if got := buf.String(); !strings.Contains(got, "errors 1") {
		t.Errorf("output = %q, want an errors count of 1", got)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := map[time.Duration]string{
		0:                                     "0s",
		44*time.Second + 600*time.Millisecond: "45s",
		12*time.Minute + 4*time.Second:        "12m04s",
		time.Hour + 2*time.Minute + 30*time.Second: "1h02m",
	}
	for in, want := range tests {
		if got := formatDuration(in); got != want {
			t.Errorf("formatDuration(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatETA_NoDataYet(t *testing.T) {
	if got := formatETA(0, 10, time.Minute); got != "--" {
		t.Errorf("formatETA with nothing done = %q, want --", got)
	}
}

// Concurrent ReportDone calls (as Runner.Run's goroutines would) must not
// interleave mid-line: the reporter's own mutex serializes the writes.
func TestStderrProgressReporter_ReportDone_ConcurrencySafe(t *testing.T) {
	var buf bytes.Buffer
	r, _ := newTestReporter(&buf, false)

	const n = 2 * pipedProgressEvery
	var wg sync.WaitGroup
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r.ReportDone(i, n, okResult)
		}(i)
	}
	wg.Wait()

	// Completion order is arbitrary, so the 25th and 50th calls to arrive
	// print: exactly two lines.
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines (%q), want 2 (a corrupted/interleaved write would change the count)", len(lines), buf.String())
	}
	linePattern := regexp.MustCompile(`^\s*\d+% \d+/\d+  elapsed \d+s(  ETA ~\d+s)?$`)
	for _, line := range lines {
		if !linePattern.MatchString(line) {
			t.Errorf("malformed or interleaved line: %q", line)
		}
	}
}

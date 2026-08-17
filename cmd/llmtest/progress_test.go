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

func TestStderrProgressReporter_ReportDone(t *testing.T) {
	tests := []struct {
		name    string
		wantSub []string
		result  runner.Result
	}{
		{
			name:    "scored result",
			result:  runner.Result{Model: "m1", TestID: "t1", Score: eval.Score{Value: 0.85}, Latency: 2 * time.Second},
			wantSub: []string{"model=m1", "test=t1", "score=0.85", "(2s)"},
		},
		{
			name:    "error result",
			result:  runner.Result{Model: "m1", TestID: "t1", Err: errors.New("boom")},
			wantSub: []string{"score=ERR"},
		},
		{
			name:    "skipped result",
			result:  runner.Result{Model: "m1", TestID: "t1", Score: eval.Score{Skipped: true}},
			wantSub: []string{"score=skip"},
		},
		{
			name:    "truncated result carries the TRUNCATED marker",
			result:  runner.Result{Model: "m1", TestID: "t1", Score: eval.Score{Value: 0}, FinishReason: "length"},
			wantSub: []string{"score=0.00", "TRUNCATED"},
		},
		{
			name:    "non-truncated result has no marker",
			result:  runner.Result{Model: "m1", TestID: "t1", Score: eval.Score{Value: 1}, FinishReason: "stop"},
			wantSub: []string{"score=1.00"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := newStderrProgressReporter(&buf)
			r.ReportDone(3, 10, tt.result)

			got := buf.String()
			if !strings.HasPrefix(got, "[3/10] ") {
				t.Errorf("ReportDone() output = %q, want prefix [3/10] ", got)
			}
			for _, want := range tt.wantSub {
				if !strings.Contains(got, want) {
					t.Errorf("ReportDone() output = %q, want substring %q", got, want)
				}
			}
			if tt.name == "non-truncated result has no marker" && strings.Contains(got, "TRUNCATED") {
				t.Errorf("ReportDone() output = %q, must not contain TRUNCATED", got)
			}
		})
	}
}

// TestStderrProgressReporter_ReportDone_ConcurrencySafe writes many
// concurrent ReportDone calls (as Runner.Run's goroutines would) and
// verifies every line is well-formed and none interleaved mid-line - the
// reporter's own mutex must serialize writes, not just the counter.
func TestStderrProgressReporter_ReportDone_ConcurrencySafe(t *testing.T) {
	var buf bytes.Buffer
	r := newStderrProgressReporter(&buf)

	const n = 50
	var wg sync.WaitGroup
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r.ReportDone(i, n, runner.Result{Model: "m", TestID: "t", Score: eval.Score{Value: 1}})
		}(i)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != n {
		t.Fatalf("got %d lines, want %d (a corrupted/interleaved write would produce a different count)", len(lines), n)
	}

	linePattern := regexp.MustCompile(`^\[\d+/\d+\] model=m test=t score=1\.00 tokens=0 \(0s\)$`)
	for _, line := range lines {
		if !linePattern.MatchString(line) {
			t.Errorf("malformed or interleaved line: %q", line)
		}
	}
}

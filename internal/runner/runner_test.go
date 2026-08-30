package runner

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/llm"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// mockClient is an offline llm.Client for unit tests; it never touches the
// network.
type mockClient struct {
	respond func(req llm.Request) (llm.Response, error)

	inFlight    int32
	maxInFlight int32
}

func (m *mockClient) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	cur := atomic.AddInt32(&m.inFlight, 1)
	for {
		max := atomic.LoadInt32(&m.maxInFlight)
		if cur <= max || atomic.CompareAndSwapInt32(&m.maxInFlight, max, cur) {
			break
		}
	}
	defer atomic.AddInt32(&m.inFlight, -1)

	if m.respond != nil {
		return m.respond(req)
	}
	return llm.Response{Text: "OK"}, nil
}

func echoEval() eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		if response == "OK" {
			return eval.Score{Value: 1}
		}
		return eval.Score{Value: 0, Detail: "got " + response}
	})
}

func TestRunner_Run_FansOutAllCombinations(t *testing.T) {
	client := &mockClient{}
	r := New(client, Config{Concurrency: 4, MaxTokensDefault: 100})

	tests := []testkit.Test{
		{ID: "t1", Category: "c", Prompt: "p1", Eval: echoEval()},
		{ID: "t2", Category: "c", Prompt: "p2", Eval: echoEval()},
	}
	models := []string{"model-a", "model-b", "model-c"}

	results := r.Run(context.Background(), models, tests)

	if len(results) != len(models)*len(tests) {
		t.Fatalf("Run() returned %d results, want %d", len(results), len(models)*len(tests))
	}

	seen := make(map[string]bool)
	for _, res := range results {
		seen[res.Model+"/"+res.TestID] = true
		if res.Err != nil {
			t.Errorf("Result.Err = %v, want nil", res.Err)
		}
		if res.Score.Value != 1 {
			t.Errorf("Result.Score.Value = %v, want 1", res.Score.Value)
		}
	}
	for _, m := range models {
		for _, tc := range tests {
			key := m + "/" + tc.ID
			if !seen[key] {
				t.Errorf("Run() missing combination %s", key)
			}
		}
	}
}

func TestRunner_Run_RespectsConcurrencyLimit(t *testing.T) {
	client := &mockClient{
		respond: func(_ llm.Request) (llm.Response, error) {
			time.Sleep(20 * time.Millisecond)
			return llm.Response{Text: "OK"}, nil
		},
	}
	const limit = 2
	r := New(client, Config{Concurrency: limit, MaxTokensDefault: 100})

	var tests []testkit.Test
	for i := range 8 {
		tests = append(tests, testkit.Test{ID: string(rune('a' + i)), Category: "c", Prompt: "p", Eval: echoEval()})
	}

	r.Run(context.Background(), []string{"model-a"}, tests)

	if got := atomic.LoadInt32(&client.maxInFlight); got > limit {
		t.Errorf("max in-flight calls = %d, want <= %d", got, limit)
	}
}

func TestRunner_Run_CapturesPerCallError(t *testing.T) {
	wantErr := errors.New("transport failure")
	client := &mockClient{
		respond: func(_ llm.Request) (llm.Response, error) {
			return llm.Response{}, wantErr
		},
	}
	r := New(client, Config{Concurrency: 2, MaxTokensDefault: 100})

	results := r.Run(context.Background(), []string{"model-a"}, []testkit.Test{
		{ID: "t1", Category: "c", Prompt: "p", Eval: echoEval()},
	})

	if len(results) != 1 {
		t.Fatalf("Run() returned %d results, want 1", len(results))
	}
	if !errors.Is(results[0].Err, wantErr) {
		t.Errorf("Result.Err = %v, want %v", results[0].Err, wantErr)
	}
}

func TestRunner_Run_NormalizesResponseBeforeEval(t *testing.T) {
	client := &mockClient{
		respond: func(_ llm.Request) (llm.Response, error) {
			return llm.Response{Text: "<think>internal reasoning</think>\n\nOK"}, nil
		},
	}
	r := New(client, Config{Concurrency: 1, MaxTokensDefault: 100})

	results := r.Run(context.Background(), []string{"model-a"}, []testkit.Test{
		{ID: "t1", Category: "c", Prompt: "p", Eval: echoEval()},
	})

	if results[0].Score.Value != 1 {
		t.Errorf("Score.Value = %v, want 1 (detail: %s)", results[0].Score.Value, results[0].Score.Detail)
	}
}

func TestRunner_Run_UsesTestMaxTokensOverDefault(t *testing.T) {
	var gotMaxTokens int
	client := &mockClient{
		respond: func(req llm.Request) (llm.Response, error) {
			gotMaxTokens = req.MaxTokens
			return llm.Response{Text: "OK"}, nil
		},
	}
	r := New(client, Config{Concurrency: 1, MaxTokensDefault: 100})

	r.Run(context.Background(), []string{"model-a"}, []testkit.Test{
		{ID: "t1", Category: "c", Prompt: "p", MaxTokens: 500, Eval: echoEval()},
	})

	if gotMaxTokens != 500 {
		t.Errorf("MaxTokens = %d, want 500 (test override)", gotMaxTokens)
	}
}

func TestRunner_Run_FallsBackToDefaultMaxTokens(t *testing.T) {
	var gotMaxTokens int
	client := &mockClient{
		respond: func(req llm.Request) (llm.Response, error) {
			gotMaxTokens = req.MaxTokens
			return llm.Response{Text: "OK"}, nil
		},
	}
	r := New(client, Config{Concurrency: 1, MaxTokensDefault: 100})

	r.Run(context.Background(), []string{"model-a"}, []testkit.Test{
		{ID: "t1", Category: "c", Prompt: "p", Eval: echoEval()},
	})

	if gotMaxTokens != 100 {
		t.Errorf("MaxTokens = %d, want 100 (config default)", gotMaxTokens)
	}
}

// TestRunner_Run_DefaultIsAFloorNotJustAFallback is the regression test for
// the systemic live-run bug: cfg.MaxTokensDefault must raise a too-small
// per-test MaxTokens, not just fill in for a zero one.
func TestRunner_Run_DefaultIsAFloorNotJustAFallback(t *testing.T) {
	var gotMaxTokens int
	client := &mockClient{
		respond: func(req llm.Request) (llm.Response, error) {
			gotMaxTokens = req.MaxTokens
			return llm.Response{Text: "OK"}, nil
		},
	}
	r := New(client, Config{Concurrency: 1, MaxTokensDefault: 6000})

	r.Run(context.Background(), []string{"model-a"}, []testkit.Test{
		{ID: "t1", Category: "c", Prompt: "p", MaxTokens: 200, Eval: echoEval()},
	})

	if gotMaxTokens != 6000 {
		t.Errorf("MaxTokens = %d, want 6000 (config default is a floor; a smaller per-test 200 must not lower it)", gotMaxTokens)
	}
}

func TestRunner_Run_TracksFinishReasonAndResponseText(t *testing.T) {
	client := &mockClient{
		respond: func(_ llm.Request) (llm.Response, error) {
			return llm.Response{Text: "  OK  ", FinishReason: "stop"}, nil
		},
	}
	r := New(client, Config{Concurrency: 1, MaxTokensDefault: 100})

	results := r.Run(context.Background(), []string{"model-a"}, []testkit.Test{
		{ID: "t1", Category: "c", Prompt: "p", Eval: echoEval()},
	})

	if results[0].FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want stop", results[0].FinishReason)
	}
	if results[0].ResponseText != "OK" {
		t.Errorf("ResponseText = %q, want the normalized \"OK\"", results[0].ResponseText)
	}
	if results[0].Truncated() {
		t.Error("Truncated() = true, want false for finish_reason=stop")
	}
}

// TestRunner_Run_TruncatedPrefixesScoreDetail is the regression test for
// fix 4: a finish_reason=length response must be visibly flagged in
// Score.Detail, not just silently scored.
func TestRunner_Run_TruncatedPrefixesScoreDetail(t *testing.T) {
	client := &mockClient{
		respond: func(_ llm.Request) (llm.Response, error) {
			return llm.Response{Text: "partial", FinishReason: llm.FinishReasonLength}, nil
		},
	}
	r := New(client, Config{Concurrency: 1, MaxTokensDefault: 100})

	results := r.Run(context.Background(), []string{"model-a"}, []testkit.Test{
		{ID: "t1", Category: "c", Prompt: "p", Eval: echoEval()},
	})

	if !results[0].Truncated() {
		t.Fatal("Truncated() = false, want true for finish_reason=length")
	}
	if !strings.HasPrefix(results[0].Score.Detail, "TRUNCATED: ") {
		t.Errorf("Score.Detail = %q, want a TRUNCATED: prefix", results[0].Score.Detail)
	}
}

func TestRunner_Run_IncludesSystemMessageWhenSet(t *testing.T) {
	var gotMessages []llm.Message
	client := &mockClient{
		respond: func(req llm.Request) (llm.Response, error) {
			gotMessages = req.Messages
			return llm.Response{Text: "OK"}, nil
		},
	}
	r := New(client, Config{Concurrency: 1, MaxTokensDefault: 100})

	r.Run(context.Background(), []string{"model-a"}, []testkit.Test{
		{ID: "t1", Category: "c", System: "you are terse", Prompt: "p", Eval: echoEval()},
	})

	if len(gotMessages) != 2 || gotMessages[0].Role != "system" || gotMessages[1].Role != "user" {
		t.Errorf("Messages = %+v, want [system, user]", gotMessages)
	}
}

// recordingReporter is a ProgressReporter that records every event under a
// mutex, so tests can assert on it after a concurrent Run.
type recordingReporter struct {
	doneEvents       []doneEvent
	startCalls       int
	startTotalTests  int
	startTotalModels int
	startConcurrency int
	mu               sync.Mutex
}

type doneEvent struct {
	testID string
	done   int
	total  int
}

func (r *recordingReporter) ReportStart(totalTests, totalModels, concurrency int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startCalls++
	r.startTotalTests = totalTests
	r.startTotalModels = totalModels
	r.startConcurrency = concurrency
}

func (r *recordingReporter) ReportDone(done, total int, result Result) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.doneEvents = append(r.doneEvents, doneEvent{done: done, total: total, testID: result.TestID})
}

func TestRunner_Run_ReportsProgress(t *testing.T) {
	client := &mockClient{}
	reporter := &recordingReporter{}
	r := New(client, Config{Concurrency: 3, MaxTokensDefault: 100, Reporter: reporter})

	tests := []testkit.Test{
		{ID: "t1", Category: "c", Prompt: "p", Eval: echoEval()},
		{ID: "t2", Category: "c", Prompt: "p", Eval: echoEval()},
	}
	models := []string{"model-a", "model-b", "model-c"}

	r.Run(context.Background(), models, tests)

	if reporter.startCalls != 1 {
		t.Errorf("ReportStart called %d times, want 1", reporter.startCalls)
	}
	if reporter.startTotalTests != len(tests) || reporter.startTotalModels != len(models) || reporter.startConcurrency != 3 {
		t.Errorf("ReportStart(totalTests=%d, totalModels=%d, concurrency=%d), want (%d, %d, 3)",
			reporter.startTotalTests, reporter.startTotalModels, reporter.startConcurrency, len(tests), len(models))
	}

	total := len(tests) * len(models)
	if len(reporter.doneEvents) != total {
		t.Fatalf("ReportDone called %d times, want %d", len(reporter.doneEvents), total)
	}

	seenDone := make(map[int]bool, total)
	for _, ev := range reporter.doneEvents {
		if ev.total != total {
			t.Errorf("ReportDone total = %d, want %d", ev.total, total)
		}
		if ev.done < 1 || ev.done > total {
			t.Errorf("ReportDone done = %d, want in [1, %d]", ev.done, total)
		}
		if seenDone[ev.done] {
			t.Errorf("ReportDone done=%d reported more than once: the completion counter is not concurrency-safe", ev.done)
		}
		seenDone[ev.done] = true
	}
}

func TestRunner_Run_NilReporterDefaultsToNoop(t *testing.T) {
	client := &mockClient{}
	r := New(client, Config{Concurrency: 2, MaxTokensDefault: 100})

	// Must not panic when Config.Reporter is left nil.
	r.Run(context.Background(), []string{"model-a"}, []testkit.Test{
		{ID: "t1", Category: "c", Prompt: "p", Eval: echoEval()},
	})
}

func TestRunner_Run_TracksLatencyAndTokens(t *testing.T) {
	client := &mockClient{
		respond: func(_ llm.Request) (llm.Response, error) {
			return llm.Response{Text: "OK", PromptTokens: 10, CompletionTokens: 5, Latency: 42 * time.Millisecond}, nil
		},
	}
	r := New(client, Config{Concurrency: 1, MaxTokensDefault: 100})

	results := r.Run(context.Background(), []string{"model-a"}, []testkit.Test{
		{ID: "t1", Category: "c", Prompt: "p", Eval: echoEval()},
	})

	if results[0].PromptTokens != 10 {
		t.Errorf("PromptTokens = %d, want 10", results[0].PromptTokens)
	}
	if results[0].CompletionTokens != 5 {
		t.Errorf("CompletionTokens = %d, want 5", results[0].CompletionTokens)
	}
	if results[0].TotalTokens() != 15 {
		t.Errorf("TotalTokens() = %d, want 15", results[0].TotalTokens())
	}
	if results[0].Latency != 42*time.Millisecond {
		t.Errorf("Latency = %v, want 42ms", results[0].Latency)
	}
}

func TestRunner_Run_RepeatSamplesEachCombination(t *testing.T) {
	client := &mockClient{}
	r := New(client, Config{Concurrency: 2, MaxTokensDefault: 100, Repeat: 3})

	tests := []testkit.Test{{ID: "t1", Category: "c", Prompt: "p1", Eval: echoEval()}}
	models := []string{"model-a", "model-b"}

	results := r.Run(context.Background(), models, tests)

	if len(results) != 6 {
		t.Fatalf("len(results) = %d, want 6 (2 models x 1 test x 3 repeats)", len(results))
	}
	seen := make(map[[2]string][]int)
	for _, res := range results {
		k := [2]string{res.Model, res.TestID}
		seen[k] = append(seen[k], res.Attempt)
	}
	for k, attempts := range seen {
		if len(attempts) != 3 {
			t.Errorf("%v: %d attempts, want 3", k, len(attempts))
		}
		got := map[int]bool{}
		for _, a := range attempts {
			got[a] = true
		}
		for a := range 3 {
			if !got[a] {
				t.Errorf("%v: missing attempt %d (have %v)", k, a, attempts)
			}
		}
	}
}

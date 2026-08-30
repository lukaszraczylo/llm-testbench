// Package runner fans a test catalog out across models with bounded
// concurrency and collects per-(model,test) results.
package runner

import (
	"context"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/llm"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// truncatedDetailPrefix is prepended to Score.Detail when the model's
// response was cut off by the token budget (llm.FinishReasonLength), so
// the reason a test scored the way it did is visible without cross-
// referencing FinishReason separately.
const truncatedDetailPrefix = "TRUNCATED: "

// Result is the outcome of running one test against one model. Attempt is
// the zero-based index of this sample when Config.Repeat > 1; it is 0 for
// single-sample runs.
type Result struct {
	Err              error
	Model            string
	TestID           string
	ResponseText     string
	FinishReason     string
	Score            eval.Score
	Latency          time.Duration
	PromptTokens     int
	CompletionTokens int
	Attempt          int
}

// Truncated reports whether the model's response was cut off by the token
// budget before it finished (finish_reason=length), rather than completing
// normally. A truncated response's Score is still whatever the evaluator
// computed on the partial text; Truncated is the signal that the score may
// not reflect what the model would have said with a larger budget.
func (r Result) Truncated() bool {
	return r.FinishReason == llm.FinishReasonLength
}

// TotalTokens returns PromptTokens + CompletionTokens, the total token
// usage billed for this (model, test) call.
func (r Result) TotalTokens() int {
	return r.PromptTokens + r.CompletionTokens
}

// Config controls how the Runner fans work out.
type Config struct {
	Reporter         ProgressReporter
	Concurrency      int
	Temperature      float64
	MaxTokensDefault int
	// Repeat runs each (model, test) combination this many times (minimum
	// 1). Even at temperature 0, reasoning models sample differently per
	// call; repeats expose that instability so a score can be trusted
	// (stable across attempts) or flagged (flaky) instead of silently
	// treated as a capability measurement.
	Repeat int
}

// Runner executes a set of tests against a set of models.
type Runner struct {
	client llm.Client
	cfg    Config
}

// New builds a Runner that issues requests through client.
func New(client llm.Client, cfg Config) *Runner {
	if cfg.Reporter == nil {
		cfg.Reporter = NoopProgressReporter{}
	}
	return &Runner{client: client, cfg: cfg}
}

// Run executes every combination of models x tests, bounded by
// cfg.Concurrency in-flight calls, and returns one Result per combination
// (times Config.Repeat samples each). A per-call failure is captured in
// Result.Err rather than aborting the run.
func (r *Runner) Run(ctx context.Context, models []string, tests []testkit.Test) []Result {
	type job struct {
		model   string
		test    testkit.Test
		attempt int
	}

	repeat := max(r.cfg.Repeat, 1)

	jobs := make([]job, 0, len(models)*len(tests)*repeat)
	for _, m := range models {
		for _, t := range tests {
			for a := range repeat {
				jobs = append(jobs, job{model: m, test: t, attempt: a})
			}
		}
	}

	results := make([]Result, len(jobs))

	g, gctx := errgroup.WithContext(ctx)
	concurrency := r.cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	g.SetLimit(concurrency)

	r.cfg.Reporter.ReportStart(len(tests), len(models), concurrency)

	var completed int32
	// go.mod's go directive is 1.22+, so loop variables are already
	// per-iteration scoped; no manual i, j := i, j shadowing needed (N4).
	for i, j := range jobs {
		g.Go(func() error {
			res := r.runOne(gctx, j.model, j.test)
			res.Attempt = j.attempt
			results[i] = res
			// atomic.AddInt32 makes the completion counter safe under
			// Config.Concurrency-many concurrent goroutines; ReportDone
			// itself must also be concurrency-safe per the ProgressReporter
			// contract.
			done := atomic.AddInt32(&completed, 1)
			r.cfg.Reporter.ReportDone(int(done), len(jobs), res)
			return nil
		})
	}
	// Errors are captured per-result, not surfaced from Run; g.Wait() only
	// ever returns non-nil if a goroutine itself returned an error, which
	// runOne never does.
	_ = g.Wait()

	return results
}

// runOne executes a single (model, test) combination.
func (r *Runner) runOne(ctx context.Context, model string, test testkit.Test) Result {
	// cfg.MaxTokensDefault is a floor, not a fallback used only when the
	// test leaves MaxTokens unset: a per-test value only ever raises the
	// budget, it never lowers it below the default. A reasoning model can
	// burn thousands of tokens on reasoning_content before ever writing an
	// answer to content, so a small per-test MaxTokens (sized for the
	// answer alone) previously starved the model into finish_reason=length
	// with an empty answer.
	maxTokens := max(test.MaxTokens, r.cfg.MaxTokensDefault)

	messages := make([]llm.Message, 0, 2)
	if test.System != "" {
		messages = append(messages, llm.Message{Role: "system", Content: test.System})
	}
	messages = append(messages, llm.Message{Role: "user", Content: test.Prompt})

	req := llm.Request{
		Model:       model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: r.cfg.Temperature,
	}

	// Wall-clock across all attempts, recorded even on error: an error row
	// with zero latency reads as an instant rejection when it may be three
	// exhausted timeouts (a 651-second generation looked "instant" in the
	// 2026-08-30 run and misdirected the diagnosis).
	callStart := time.Now()
	resp, err := r.client.Complete(ctx, req)
	if err != nil {
		return Result{Model: model, TestID: test.ID, Err: err, Latency: time.Since(callStart)}
	}

	normalized := testkit.Normalize(resp.Text)
	score := test.Eval.Evaluate(ctx, normalized)
	if resp.FinishReason == llm.FinishReasonLength {
		score.Detail = truncatedDetailPrefix + score.Detail
	}

	return Result{
		Model:            model,
		TestID:           test.ID,
		Score:            score,
		ResponseText:     normalized,
		FinishReason:     resp.FinishReason,
		Latency:          resp.Latency,
		PromptTokens:     resp.PromptTokens,
		CompletionTokens: resp.CompletionTokens,
	}
}

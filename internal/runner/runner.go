// Package runner fans a test catalog out across models with bounded
// concurrency and collects per-(model,test) results.
package runner

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/llm"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// Result is the outcome of running one test against one model.
type Result struct {
	Err              error
	Model            string
	TestID           string
	Score            eval.Score
	Latency          time.Duration
	PromptTokens     int
	CompletionTokens int
}

// TotalTokens returns PromptTokens + CompletionTokens, the total token
// usage billed for this (model, test) call.
func (r Result) TotalTokens() int {
	return r.PromptTokens + r.CompletionTokens
}

// Config controls how the Runner fans work out.
type Config struct {
	// Concurrency bounds the number of in-flight (model,test) calls.
	Concurrency int
	// Temperature is sent on every request; PLAN.md pins this to 0 for
	// determinism across runs.
	Temperature float64
	// MaxTokensDefault is used when a Test does not set its own MaxTokens.
	MaxTokensDefault int
}

// Runner executes a set of tests against a set of models.
type Runner struct {
	client llm.Client
	cfg    Config
}

// New builds a Runner that issues requests through client.
func New(client llm.Client, cfg Config) *Runner {
	return &Runner{client: client, cfg: cfg}
}

// Run executes every combination of models x tests, bounded by
// cfg.Concurrency in-flight calls, and returns one Result per combination.
// A per-call failure is captured in Result.Err rather than aborting the run.
func (r *Runner) Run(ctx context.Context, models []string, tests []testkit.Test) []Result {
	type job struct {
		model string
		test  testkit.Test
	}

	jobs := make([]job, 0, len(models)*len(tests))
	for _, m := range models {
		for _, t := range tests {
			jobs = append(jobs, job{model: m, test: t})
		}
	}

	results := make([]Result, len(jobs))

	g, gctx := errgroup.WithContext(ctx)
	concurrency := r.cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	g.SetLimit(concurrency)

	// go.mod's go directive is 1.22+, so loop variables are already
	// per-iteration scoped; no manual i, j := i, j shadowing needed (N4).
	for i, j := range jobs {
		g.Go(func() error {
			results[i] = r.runOne(gctx, j.model, j.test)
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
	maxTokens := test.MaxTokens
	if maxTokens <= 0 {
		maxTokens = r.cfg.MaxTokensDefault
	}

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

	resp, err := r.client.Complete(ctx, req)
	if err != nil {
		return Result{Model: model, TestID: test.ID, Err: err}
	}

	normalized := testkit.Normalize(resp.Text)
	score := test.Eval.Evaluate(ctx, normalized)

	return Result{
		Model:            model,
		TestID:           test.ID,
		Score:            score,
		Latency:          resp.Latency,
		PromptTokens:     resp.PromptTokens,
		CompletionTokens: resp.CompletionTokens,
	}
}

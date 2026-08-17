// Package eval provides deterministic evaluators that score a (normalized)
// LLM response against an expected answer. All evaluators are pure functions
// of the response text plus evaluator configuration; none call the network.
package eval

import "context"

// Score is the outcome of evaluating one response. Value is in [0, 1] unless
// Skipped is true (e.g. a required toolchain is missing), in which case
// Value carries no meaning and the test is excluded from aggregates.
type Score struct {
	Detail  string
	Value   float64
	Skipped bool
}

// Evaluator scores a normalized response string.
type Evaluator interface {
	Evaluate(ctx context.Context, response string) Score
}

// EvaluatorFunc adapts a plain function to the Evaluator interface.
type EvaluatorFunc func(ctx context.Context, response string) Score

// Evaluate implements Evaluator.
func (f EvaluatorFunc) Evaluate(ctx context.Context, response string) Score {
	return f(ctx, response)
}

// clamp01 constrains v to the closed interval [0, 1].
func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}

package eval

import (
	"context"
	"fmt"
	"strings"
)

// WeightedEvaluator pairs an Evaluator with its share of the composite score.
type WeightedEvaluator struct {
	Eval   Evaluator
	Weight float64
}

// W builds a WeightedEvaluator; a convenience constructor for All.
func W(e Evaluator, weight float64) WeightedEvaluator {
	return WeightedEvaluator{Eval: e, Weight: weight}
}

// compositeEval computes a weighted mean over its sub-evaluators, ignoring
// sub-scores that are Skipped.
type compositeEval struct {
	weighted []WeightedEvaluator
}

// All returns an Evaluator that combines weighted sub-evaluators into a
// single weighted-mean Score. Sub-scores marked Skipped are excluded from
// both the numerator and the weight total; if every sub-evaluator is
// skipped, the composite itself is Skipped.
func All(weighted ...WeightedEvaluator) Evaluator {
	return compositeEval{weighted: weighted}
}

// Mean wraps evaluators with equal weight 1 and combines them via All. A
// convenience for the common case where every sub-check counts the same.
// Named Mean, not Equal, to avoid reading as a synonym of the Equals
// evaluator (S12) - Mean computes an unweighted average of N sub-scores,
// while Equals is a single trimmed, case-insensitive string comparison.
func Mean(evaluators ...Evaluator) Evaluator {
	weighted := make([]WeightedEvaluator, 0, len(evaluators))
	for _, e := range evaluators {
		weighted = append(weighted, W(e, 1))
	}
	return All(weighted...)
}

func (c compositeEval) Evaluate(ctx context.Context, response string) Score {
	var weightedSum, activeWeight float64
	details := make([]string, 0, len(c.weighted))

	for _, we := range c.weighted {
		s := we.Eval.Evaluate(ctx, response)
		if s.Skipped {
			details = append(details, fmt.Sprintf("skipped (w=%.2f): %s", we.Weight, s.Detail))
			continue
		}
		activeWeight += we.Weight
		weightedSum += s.Value * we.Weight
		details = append(details, fmt.Sprintf("%.2f (w=%.2f): %s", s.Value, we.Weight, s.Detail))
	}

	if activeWeight == 0 {
		return Score{Skipped: true, Detail: "all sub-evaluators skipped: " + strings.Join(details, "; ")}
	}

	return Score{
		Value:  clamp01(weightedSum / activeWeight),
		Detail: strings.Join(details, "; "),
	}
}

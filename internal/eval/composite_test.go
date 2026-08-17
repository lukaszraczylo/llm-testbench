package eval

import (
	"context"
	"testing"
)

func TestAll_WeightedMean(t *testing.T) {
	e := All(
		W(Equals("minor"), 2),
		W(ContainsAny("bump"), 1),
	)
	// "minor bump" -> Equals fails (0, weight 2), ContainsAny succeeds (1, weight 1)
	// weighted mean = (0*2 + 1*1) / 3 = 1/3
	got := e.Evaluate(context.Background(), "minor bump")
	want := 1.0 / 3.0
	if got.Value != want {
		t.Errorf("Evaluate() = %v, want %v (detail: %s)", got.Value, want, got.Detail)
	}
}

func TestMean_AllPass(t *testing.T) {
	e := Mean(
		ContainsAll("launchd"),
		ContainsAll("StartInterval"),
	)
	got := e.Evaluate(context.Background(), "use launchd with StartInterval key")
	if got.Value != 1 {
		t.Errorf("Evaluate() = %v, want 1 (detail: %s)", got.Value, got.Detail)
	}
}

func TestMean_PartialCredit(t *testing.T) {
	e := Mean(
		ContainsAll("launchd"),
		ContainsAll("StartInterval"),
	)
	// Only "launchd" present -> (1 + 0) / 2 = 0.5
	got := e.Evaluate(context.Background(), "use launchd, not cron")
	if got.Value != 0.5 {
		t.Errorf("Evaluate() = %v, want 0.5 (detail: %s)", got.Value, got.Detail)
	}
}

func TestAll_SkippedSubEvaluatorExcluded(t *testing.T) {
	skipped := EvaluatorFunc(func(_ context.Context, _ string) Score {
		return Score{Skipped: true, Detail: "toolchain missing: go"}
	})
	e := All(
		W(Equals("x"), 1),
		W(skipped, 5),
	)
	got := e.Evaluate(context.Background(), "x")
	if got.Skipped {
		t.Fatal("Evaluate() should not be Skipped when at least one sub-evaluator is active")
	}
	if got.Value != 1 {
		t.Errorf("Evaluate() = %v, want 1 (skipped sub-evaluator excluded from weight)", got.Value)
	}
}

func TestAll_EverythingSkipped(t *testing.T) {
	skipped := EvaluatorFunc(func(_ context.Context, _ string) Score {
		return Score{Skipped: true}
	})
	e := All(W(skipped, 1))
	got := e.Evaluate(context.Background(), "anything")
	if !got.Skipped {
		t.Error("Evaluate() should be Skipped when every sub-evaluator is skipped")
	}
}

package tests

import (
	"context"
	"testing"
	"unsafe"
)

// wastefulTask and optimalTask mirror the struct in goStructAlignTest's
// prompt (wasteful field order) and its minimal-padding reordering. Sizes
// are asserted with unsafe.Sizeof to ground the "ground truth:" derivation
// comment in golang.go against the actual compiler layout, per PLAN.md's
// requirement to recompute cheap ground truths in the unit test.
//
// wastefulTask deliberately keeps the prompt's wasteful field order
// verbatim (the "before" layout), so fieldalignment's auto-fix must not be
// allowed to reorder it - that would make it identical to optimalTask and
// defeat this test's comparison.
//
//nolint:govet // fieldalignment: intentional, see comment above
type wastefulTask struct {
	Active   bool
	ID       int64
	Priority int32
	Name     string
	Flag     byte
}

type optimalTask struct {
	Name     string
	ID       int64
	Priority int32
	Active   bool
	Flag     byte
}

func TestGoStructAlign_GroundTruthSizes(t *testing.T) {
	if got, want := unsafe.Sizeof(wastefulTask{}), uintptr(48); got != want {
		t.Errorf("unsafe.Sizeof(wastefulTask{}) = %d, want %d", got, want)
	}
	if got, want := unsafe.Sizeof(optimalTask{}), uintptr(32); got != want {
		t.Errorf("unsafe.Sizeof(optimalTask{}) = %d, want %d", got, want)
	}
}

func TestGoStructAlignTest_Eval(t *testing.T) {
	tc := goStructAlignTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "optimal order Name then ID",
			response: "```go\ntype Task struct {\n\tName     string\n\tID       int64\n\tPriority int32\n\tActive   bool\n\tFlag     byte\n}\n```",
			want:     1,
		},
		{
			name:     "optimal order ID then Name",
			response: "```go\ntype Task struct {\n\tID       int64\n\tName     string\n\tPriority int32\n\tFlag     byte\n\tActive   bool\n}\n```",
			want:     1,
		},
		{
			name:     "original wasteful order",
			response: "```go\ntype Task struct {\n\tActive   bool\n\tID       int64\n\tPriority int32\n\tName     string\n\tFlag     byte\n}\n```",
			want:     0,
		},
		{
			name:     "int32 not moved after align-8 fields",
			response: "```go\ntype Task struct {\n\tPriority int32\n\tName     string\n\tID       int64\n\tActive   bool\n\tFlag     byte\n}\n```",
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestGoWorkerPoolTest_Eval(t *testing.T) {
	tc := goWorkerPoolTest()

	correctPool := "```go\n" + `func Pool[T, R any](in []T, workers int, f func(T) R) []R {
	out := make([]R, len(in))
	sem := make(chan struct{}, workers)
	done := make(chan struct{})
	for i, v := range in {
		i, v := i, v
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; done <- struct{}{} }()
			out[i] = f(v)
		}()
	}
	for range in {
		<-done
	}
	return out
}
` + "```"

	sequentialPool := "```go\n" + `func Pool[T, R any](in []T, workers int, f func(T) R) []R {
	out := make([]R, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}
` + "```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct concurrent pool", correctPool, 1},
		{"sequential pool fails the maxConcurrent>=2 check", sequentialPool, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Skipped {
				t.Skip("go toolchain not available, skipping exec-evaluator assertion")
			}
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestGoWorkerPoolTest_Eval_ResponseShapes covers B1: the two-file
// (solution.go + harness.go) split must accept a response that brings its
// own imports, whether or not it also carries a package clause, since
// eval.GoRun strips at most a leading package clause and gives the
// response its own import block separate from the harness's.
func TestGoWorkerPoolTest_Eval_ResponseShapes(t *testing.T) {
	tc := goWorkerPoolTest()

	// Shape 1: a bare function body with its own "import" block and no
	// package clause - the common case when a model obeys "respond with
	// only the function definition".
	withImportsNoPackage := "```go\n" + `import "sync"

func Pool[T, R any](in []T, workers int, f func(T) R) []R {
	out := make([]R, len(in))
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	for i, v := range in {
		i, v := i, v
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = f(v)
		}()
	}
	wg.Wait()
	return out
}
` + "```"

	// Shape 2: a full, standalone file - package clause, imports, and the
	// function - as some models emit even when told not to include a
	// package clause. eval.GoRun must strip the leading "package main" line
	// and still compile it as part of solution.go.
	fullFileWithPackageClause := "```go\n" + `package main

import "sync"

func Pool[T, R any](in []T, workers int, f func(T) R) []R {
	out := make([]R, len(in))
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	for i, v := range in {
		i, v := i, v
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = f(v)
		}()
	}
	wg.Wait()
	return out
}
` + "```"

	// Shape 3: uses sync.WaitGroup without importing "sync" - now that the
	// prompt explicitly allows imports, this is a genuine model mistake,
	// not a harness limitation. It must fail to compile (score 0), not be
	// silently patched or Skipped.
	usesSyncWithoutImport := "```go\n" + `func Pool[T, R any](in []T, workers int, f func(T) R) []R {
	out := make([]R, len(in))
	var wg sync.WaitGroup
	for i, v := range in {
		wg.Add(1)
		i, v := i, v
		go func() {
			defer wg.Done()
			out[i] = f(v)
		}()
	}
	wg.Wait()
	return out
}
` + "```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"imports, no package clause: PASS", withImportsNoPackage, 1},
		{"full file with package clause: PASS after strip", fullFileWithPackageClause, 1},
		{"uses sync without importing it: fails to compile", usesSyncWithoutImport, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Skipped {
				t.Skip("go toolchain not available, skipping exec-evaluator assertion")
			}
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestGoSemverClassifyTest_Eval(t *testing.T) {
	tc := goSemverClassifyTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct minor", `{"bump":"minor"}`, 1},
		{"correct minor fenced", "```json\n{\"bump\": \"minor\"}\n```", 1},
		{"trap: falls for the word breaking and answers major", `{"bump":"major"}`, 0},
		{"wrong: answers patch, missing the feat commit", `{"bump":"patch"}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

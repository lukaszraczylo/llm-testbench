package tests

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

func TestGoChannelDeadlockTest_Eval(t *testing.T) {
	tc := goChannelDeadlockTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", `{"line": 5, "reason": "unbuffered-send-no-receiver"}`, 1},
		{"correct fenced json", "```json\n{\"line\": 5, \"reason\": \"unbuffered-send-no-receiver\"}\n```", 1},
		{"correct with prose wrapper", `The answer is: {"line": 5, "reason": "unbuffered-send-no-receiver"}`, 1},
		{"wrong line", `{"line": 6, "reason": "unbuffered-send-no-receiver"}`, 0.5},
		{"wrong reason", `{"line": 5, "reason": "nil-channel-receive"}`, 0.5},
		{"both wrong", `{"line": 6, "reason": "nil-channel-receive"}`, 0},
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

// computeDeferRecover mirrors goDeferRecoverTraceCode's compute() exactly,
// so its return value grounds the "ground truth:" derivation in golang.go
// by actually running the same logic, per PLAN.md's recomputation rule.
func computeDeferRecover() (result int) {
	defer func() {
		if r := recover(); r != nil {
			result = -1
		}
	}()
	defer func() {
		result += 10
	}()
	panic("boom")
}

func TestGoDeferRecoverTrace_GroundTruth(t *testing.T) {
	if got, want := computeDeferRecover(), -1; got != want {
		t.Errorf("computeDeferRecover() = %d, want %d", got, want)
	}
}

func TestGoDeferRecoverTraceTest_Eval(t *testing.T) {
	tc := goDeferRecoverTraceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "-1", 1},
		{"correct with prose", "The program prints -1.", 1},
		{"correct with label", "Output: -1", 1},
		{"wrong: ignores recover overwrite", "10", 0},
		{"wrong: sign error", "1", 0},
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

// sliceAppendAliasCheck mirrors goSliceAppendAliasingCode's main() body,
// returning a[2] instead of printing it, so the ground truth is recomputed
// in-process rather than trusted from the doc comment alone.
func sliceAppendAliasCheck() int {
	a := make([]int, 3, 5)
	a[0], a[1], a[2] = 1, 2, 3
	b := a[:2]
	b = append(b, 99)
	_ = b
	return a[2]
}

func TestGoSliceAppendAliasing_GroundTruth(t *testing.T) {
	if got, want := sliceAppendAliasCheck(), 99; got != want {
		t.Errorf("sliceAppendAliasCheck() = %d, want %d", got, want)
	}
}

func TestGoSliceAppendAliasingTest_Eval(t *testing.T) {
	tc := goSliceAppendAliasingTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "99", 1},
		{"correct with prose", "a[2] is 99.", 1},
		{"correct with label", "The result is 99", 1},
		{"wrong: ignores aliasing, keeps original value", "3", 0},
		{"wrong: off-by-index confusion", "2", 0},
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

// TestGoMethodSetInterface_GroundTruth cross-checks the value_compiles=false,
// pointer_compiles=true claims in golang.go's doc comment by actually
// running `go build` on each snippet, mirroring c_test.go's
// TestCStructSizeWant_GroundTruth pattern of shelling out to the real
// toolchain rather than trusting hand analysis alone.
func TestGoMethodSetInterface_GroundTruth(t *testing.T) {
	const preamble = `package main

` + goMethodSetInterfaceCode + `

`
	tests := []struct {
		name       string
		mainLine   string
		wantBuilds bool
	}{
		{"value type does not implement Stringer", `func main() { var _ Stringer = Wrapper{val: "hi"} }`, false},
		{"pointer type implements Stringer", `func main() { var _ Stringer = &Wrapper{val: "hi"} }`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			src := preamble + tt.mainLine + "\n"
			srcPath := filepath.Join(dir, "main.go")
			if err := os.WriteFile(srcPath, []byte(src), 0o600); err != nil {
				t.Fatalf("write main.go: %v", err)
			}
			// #nosec G204 -- srcPath is a fixed filename this test just wrote
			// into its own t.TempDir(); not external input.
			cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "out"), srcPath) //nolint:gosec // see comment above
			out, err := cmd.CombinedOutput()
			builds := err == nil
			if builds != tt.wantBuilds {
				t.Errorf("go build success = %v, want %v (output: %s)", builds, tt.wantBuilds, out)
			}
		})
	}
}

func TestGoMethodSetInterfaceTest_Eval(t *testing.T) {
	tc := goMethodSetInterfaceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", `{"value_compiles": false, "pointer_compiles": true}`, 1},
		{"correct fenced json", "```json\n{\"value_compiles\": false, \"pointer_compiles\": true}\n```", 1},
		{"correct with prose wrapper", `Answer: {"value_compiles": false, "pointer_compiles": true}`, 1},
		{"wrong: both true", `{"value_compiles": true, "pointer_compiles": true}`, 0.5},
		{"wrong: both false", `{"value_compiles": false, "pointer_compiles": false}`, 0.5},
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

// contextCancellationTraceCheck mirrors goContextCancellationTraceCode's
// main() body, returning the text it would print instead of printing it, to
// recompute the ground truth in-process. It also grounds the "no
// goroutine/timing dependency" claim in golang.go's doc comment: this
// function has no goroutines or sleeps at all, so a deterministic result
// here demonstrates the propagation is synchronous.
func contextCancellationTraceCheck() string {
	parent, cancelParent := context.WithCancel(context.Background())
	child, cancelChild := context.WithCancel(parent)
	defer cancelChild()

	cancelParent()

	select {
	case <-child.Done():
		return child.Err().Error()
	default:
		return "not done"
	}
}

func TestGoContextCancellationTrace_GroundTruth(t *testing.T) {
	if got, want := contextCancellationTraceCheck(), "context canceled"; got != want {
		t.Errorf("contextCancellationTraceCheck() = %q, want %q", got, want)
	}
}

func TestGoContextCancellationTraceTest_Eval(t *testing.T) {
	tc := goContextCancellationTraceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "context canceled", 1},
		{"correct case-insensitive", "CONTEXT CANCELED", 1},
		{"correct with surrounding whitespace", "  context canceled  ", 1},
		{"wrong: assumes select falls to default", "not done", 0},
		{"wrong: wrong sentinel error", "context deadline exceeded", 0},
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

func TestGoGenericsConstraintFixTest_Eval(t *testing.T) {
	tc := goGenericsConstraintFixTest()

	fullOrderedConstraint := "```go\n" + `type Ordered interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64 | ~string
}

func Max[T Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}
` + "```"

	minimalConstraint := "```go\n" + `type Ordered interface {
	~int | ~float64 | ~string
}

func Max[T Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}
` + "```"

	stillBroken := "```go\n" + `func Max[T any](a, b T) T {
	if a > b {
		return a
	}
	return b
}
` + "```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"full Ordered constraint covering all integer widths", fullOrderedConstraint, 1},
		{"minimal constraint covering only the exercised types", minimalConstraint, 1},
		{"still broken: constraint left as any", stillBroken, 0},
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

func TestGoBoundedFanoutTest_Eval(t *testing.T) {
	tc := goBoundedFanoutTest()

	correctBoundedRun := "```go\n" + `import "sync"

func BoundedRun(tasks []func() error, limit int) error {
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for _, task := range tasks {
		task := task
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := task(); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}
` + "```"

	sequentialBoundedRun := "```go\n" + `func BoundedRun(tasks []func() error, limit int) error {
	for _, task := range tasks {
		if err := task(); err != nil {
			return err
		}
	}
	return nil
}
` + "```"

	unboundedBoundedRun := "```go\n" + `import "sync"

func BoundedRun(tasks []func() error, limit int) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for _, task := range tasks {
		task := task
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := task(); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}
` + "```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct semaphore-bounded concurrency", correctBoundedRun, 1},
		{"wrong: sequential, no concurrency", sequentialBoundedRun, 0},
		{"wrong: unbounded, ignores limit", unboundedBoundedRun, 0},
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

// boundedRunReference is a package-local reference implementation of the
// same bounded-fan-out shape the harness expects (semaphore-limited
// concurrency, first-error propagation), independent of the fenced
// test-string copy in TestGoBoundedFanoutTest_Eval, so the harness's
// concurrency-bound assertion is grounded against real, in-process
// goroutines rather than only against exec.Command output.
func boundedRunReference(tasks []func() error, limit int) error {
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for _, task := range tasks {
		task := task
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := task(); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}

func TestBoundedRunReference_GroundTruth(t *testing.T) {
	const limit = 4
	const n = 20

	var concurrent, maxConcurrent int32
	makeTask := func(shouldFail bool) func() error {
		return func() error {
			cur := atomic.AddInt32(&concurrent, 1)
			for {
				m := atomic.LoadInt32(&maxConcurrent)
				if cur <= m || atomic.CompareAndSwapInt32(&maxConcurrent, m, cur) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt32(&concurrent, -1)
			if shouldFail {
				return errors.New("boom")
			}
			return nil
		}
	}

	okTasks := make([]func() error, n)
	for i := range okTasks {
		okTasks[i] = makeTask(false)
	}
	if err := boundedRunReference(okTasks, limit); err != nil {
		t.Errorf("boundedRunReference(all ok) = %v, want nil", err)
	}
	if got := atomic.LoadInt32(&maxConcurrent); got < 2 || got > limit {
		t.Errorf("maxConcurrent = %d, want in [2, %d]", got, limit)
	}

	atomic.StoreInt32(&concurrent, 0)
	atomic.StoreInt32(&maxConcurrent, 0)
	failTasks := make([]func() error, n)
	for i := range failTasks {
		failTasks[i] = makeTask(i%3 == 0)
	}
	if err := boundedRunReference(failTasks, limit); err == nil {
		t.Error("boundedRunReference(some fail) = nil, want an error")
	}
	if got := atomic.LoadInt32(&maxConcurrent); got < 2 || got > limit {
		t.Errorf("maxConcurrent = %d, want in [2, %d]", got, limit)
	}
}

// TestGoChannelDeadlockCode_ContainsExpectedLines grounds the line-number
// claim in goChannelDeadlockTest's doc comment against the actual prompt
// text used at registration time, rather than trusting the doc comment's
// count independently of the real prompt.
func TestGoChannelDeadlockCode_ContainsExpectedLines(t *testing.T) {
	tc := goChannelDeadlockTest()
	if !strings.Contains(tc.Prompt, "5: \tch <- 1") {
		t.Error("goChannelDeadlockTest prompt no longer has 'ch <- 1' on line 5; update the ground-truth line number")
	}
}

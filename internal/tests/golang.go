// Package tests is the test catalog: one file per category, each
// registering its Tests into a shared Registry via Register in main.go's
// wiring (see All in catalog.go).
package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// terseCodeOnly is prepended as a system message on code-generation tests
// to keep responses parseable by the exec/regex evaluators.
const terseCodeOnly = "Respond with only the requested code in a single fenced code block. " +
	"No prose before or after the code block, no explanations, no comments beyond what was asked for."

func registerGolangTests(r *testkit.Registry) {
	r.Register(goStructAlignTest())
	r.Register(goWorkerPoolTest())
	r.Register(goSemverClassifyTest())
	r.Register(goChannelDeadlockTest())
	r.Register(goDeferRecoverTraceTest())
	r.Register(goSliceAppendAliasingTest())
	r.Register(goMethodSetInterfaceTest())
	r.Register(goContextCancellationTraceTest())
	r.Register(goGenericsConstraintFixTest())
	r.Register(goBoundedFanoutTest())
}

// goStructAlignTest: reorder struct fields to minimize padding on a 64-bit
// (LP64) system.
//
// ground truth: on LP64, alignment/size per field are bool(align 1, size
// 1), int64(align 8, size 8), int32(align 4, size 4), string(align 8, size
// 16 = 8-byte pointer + 8-byte length), byte(align 1, size 1). The Go
// compiler lays out fields in declaration order, padding each field to its
// own alignment, then rounds the struct size up to the widest field
// alignment (8).
//
//	Wasteful order: Active bool(0..1) pad(1..8) ID int64(8..16)
//	Priority int32(16..20) pad(20..24) Name string(24..40) Flag byte(40..41)
//	pad(41..48) -> total 48 bytes, 18 bytes of padding.
//
//	Optimal order: group the two 8-byte-aligned fields first (Name string,
//	ID int64, in either order: 0..16, 16..24), then the 4-byte-aligned
//	int32 (24..28), then the two 1-byte fields adjacent with no padding
//	between them (28..29, 29..30), then round up to the next multiple of 8
//	-> total 32 bytes. No smaller layout exists: the payload is
//	16+8+4+1+1 = 30 bytes and the struct's own alignment (8, from the
//	64-bit-aligned fields) forces the size up to the next multiple of 8,
//	which is 32.
//
// golang_test.go verifies both sizes with unsafe.Sizeof on literal struct
// definitions, and cross-checks computeLP64StructSize itself against
// unsafe.Sizeof for several canonical field orders. Scoring
// (goStructAlignEval) parses the response's actual field order and
// computes its real layout size rather than matching one hardcoded
// ordering, so any of the several field orders that reach 32 bytes scores
// full credit (S3).
func goStructAlignTest() testkit.Test {
	prompt := `Here is a Go struct definition:

` + "```go" + `
type Task struct {
	Active   bool
	ID       int64
	Priority int32
	Name     string
	Flag     byte
}
` + "```" + `

On a 64-bit (LP64) system, this field order wastes memory to padding.
Reorder the fields (keep the same field names and types, change nothing
else) to minimize the total size of the struct due to alignment padding.
Respond with the corrected struct definition only.`

	return testkit.Test{
		ID:          "go-struct-align",
		Category:    "programming",
		Subcategory: "golang",
		Description: "Reorder a Go struct's fields to minimize alignment padding on a 64-bit system.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		MaxTokens:   500,
		Eval:        goStructAlignEval(),
	}
}

// goWorkerPoolHarness is a complete, independent driver file (its own
// package clause and imports) that eval.GoRun writes to harness.go
// alongside the model's response in solution.go, so the model's code and
// the harness's code never share one spliced import block. It drives the
// response's Pool[T, R any] implementation (defined in solution.go, same
// package) with a deliberately slow worker function and checks (a) output
// order matches input order and (b) the pool actually parallelizes work,
// via an observed-concurrency counter (maxConcurrent >= 2). There is
// deliberately no wall-clock bound (5e): a CI runner under load can make
// even genuinely concurrent work take longer than any fixed millisecond
// budget, and maxConcurrent >= 2 already proves real parallelism without
// that flake risk.
const goWorkerPoolHarness = `package main

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

func main() {
	const n = 100
	const workers = 8
	const perItem = 2 * time.Millisecond

	in := make([]int, n)
	for i := range in {
		in[i] = i
	}

	var concurrent int32
	var maxConcurrent int32
	f := func(x int) int {
		cur := atomic.AddInt32(&concurrent, 1)
		for {
			m := atomic.LoadInt32(&maxConcurrent)
			if cur <= m || atomic.CompareAndSwapInt32(&maxConcurrent, m, cur) {
				break
			}
		}
		time.Sleep(perItem)
		atomic.AddInt32(&concurrent, -1)
		return x * 2
	}

	out := Pool(in, workers, f)

	if len(out) != n {
		fmt.Fprintf(os.Stderr, "len(out)=%d, want %d\n", len(out), n)
		os.Exit(1)
	}
	for i := range out {
		if out[i] != i*2 {
			fmt.Fprintf(os.Stderr, "out[%d]=%d, want %d: input order not preserved\n", i, out[i], i*2)
			os.Exit(1)
		}
	}
	if atomic.LoadInt32(&maxConcurrent) < 2 {
		fmt.Fprintf(os.Stderr, "maxConcurrent=%d, want >= 2: no real concurrency observed\n", maxConcurrent)
		os.Exit(1)
	}

	fmt.Println("PASS")
}
`

// goWorkerPoolTest: implement a generic, order-preserving, concurrent
// worker pool.
//
// ground truth: the harness above is the oracle; it directly measures
// order preservation and real concurrency rather than a hardcoded number,
// so there is no separate constant to derive.
func goWorkerPoolTest() testkit.Test {
	prompt := `Implement this Go generic function:

` + "```go" + `
func Pool[T, R any](in []T, workers int, f func(T) R) []R
` + "```" + `

Requirements:
- Run f concurrently across exactly workers goroutines.
- The returned slice must preserve the input order: result[i] is f(in[i]).
- You may include whatever imports your implementation needs. Do not
  include a package clause or a main function: respond with only the
  Pool function definition (plus any imports it needs).`

	return testkit.Test{
		ID:          "go-worker-pool",
		Category:    "programming",
		Subcategory: "golang",
		Description: "Implement a generic, order-preserving, concurrent worker pool.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		MaxTokens:   800,
		Eval:        eval.GoRun(goWorkerPoolHarness, "PASS"),
	}
}

// goSemverClassifyTest: classify the aggregate semver bump implied by a
// batch of conventional commits.
//
// ground truth: per Conventional Commits, a MAJOR bump requires either a
// "!" after the type/scope or a "BREAKING CHANGE:" footer; a MINOR bump
// comes from any "feat:" commit; a PATCH bump comes from any "fix:" commit
// with no feat present; chore/docs/etc. commits do not trigger a release on
// their own. The batch below has one "feat:" commit, two "fix:" commits, a
// "chore:" and a "docs:" commit, and zero commits with a "!" marker or a
// "BREAKING CHANGE:" footer (commit 4's body uses the word "breaking" in
// the ordinary-English sense "test breaking flakiness", not as a release
// footer) -> the highest applicable bump across the batch is minor.
func goSemverClassifyTest() testkit.Test {
	prompt := `Here are 5 conventional commit messages from one release batch:

1. "feat(cache): add TTL eviction policy for LRU cache"
2. "fix(scheduler): prevent duplicate job dispatch on retry"
3. "chore(ci): pin golangci-lint version in the lint config"
4. "fix(auth): correct token expiry check off by one

This resolves an intermittent test breaking flakiness in local dev but
does not change any public behaviour."
5. "docs(readme): clarify installation steps for macOS"

Using the Conventional Commits specification, determine the single semver
bump ("major", "minor", or "patch") that this batch of commits, taken
together, should produce for the next release. Respond with only a JSON
object: {"bump":"major|minor|patch"}`

	return testkit.Test{
		ID:          "go-semver-classify",
		Category:    "programming",
		Subcategory: "golang",
		Description: "Classify the aggregate semver bump for a batch of conventional commits, including a 'breaking' word trap.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		MaxTokens:   300,
		Eval:        eval.JSONField("bump", "minor"),
	}
}

// goChannelDeadlockReason is the fixed vocabulary of "why" answers
// goChannelDeadlockTest allows, so the JSON "reason" field has exactly one
// canonical correct literal instead of open-ended prose.
const (
	goChannelDeadlockReasonUnbufferedNoReceiver = "unbuffered-send-no-receiver"
)

// goChannelDeadlockTest: identify which line of an inline snippet blocks
// forever, and why.
//
// ground truth: `ch := make(chan int)` creates an unbuffered channel, whose
// send blocks until another goroutine is ready to receive concurrently. Line
// 5 (`ch <- 1`) runs on main's single goroutine with no other goroutine
// started, so the send blocks forever - the receive on line 6 can never run
// because line 5 never returns. This is a single-goroutine self-deadlock,
// not (for example) a nil-channel receive or a full buffered channel, so the
// enumerated reason is "unbuffered-send-no-receiver". Because a genuine
// deadlock hangs the process, golang_test.go cannot recompute this by
// running the snippet (per PLAN.md, ground truth is derived in the doc
// comment instead, as for goStructAlignTest's alignment arithmetic).
func goChannelDeadlockTest() testkit.Test {
	prompt := `Here is Go code, with line numbers prefixed:

1: package main
2:
3: func main() {
4: 	ch := make(chan int)
5: 	ch <- 1
6: 	v := <-ch
7: 	println(v)
8: }

Which line number blocks forever? Pick exactly one reason from this list:
"unbuffered-send-no-receiver", "closed-channel-send", "nil-channel-receive",
"buffered-channel-full". Respond with only a JSON object:
{"line": <line number as an integer>, "reason": "<one of the four reasons above, exactly as written>"}`

	evaluator := eval.All(
		eval.W(eval.JSONField("line", 5), 1),
		eval.W(eval.JSONField("reason", goChannelDeadlockReasonUnbufferedNoReceiver), 1),
	)

	return testkit.Test{
		ID:          "go-channel-deadlock",
		Category:    "programming",
		Subcategory: "golang",
		Description: "Identify the line and reason a single-goroutine unbuffered-channel send deadlocks forever.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// goDeferRecoverTraceCode is the inline snippet for goDeferRecoverTraceTest.
// It is duplicated (not shared via a Go source string constant referencing
// a compiled function) so the prompt text is self-contained per EXPANSION.md
// rule 1; golang_test.go defines an equivalent compute() and calls it
// in-process to recompute the ground truth independently of this comment.
const goDeferRecoverTraceCode = `package main

import "fmt"

func compute() (result int) {
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

func main() {
	fmt.Println(compute())
}`

// goDeferRecoverTraceTest: trace the exact printed output of a function
// combining defer, recover, and a named return value.
//
// ground truth: deferred functions run in LIFO (last-registered-first)
// order. compute() panics with "boom" before returning, so both defers run
// during unwinding: first (LIFO) the second-registered defer runs,
// `result += 10`, taking the named return `result` from its zero value 0 to
// 10 - the panic is still active at this point, since this defer never
// calls recover. Then the first-registered defer runs, calls recover()
// (which returns the non-nil panic value "boom", stopping the panic), and
// overwrites `result = -1`. The named return is read after all defers
// finish, so compute() returns -1 and main prints "-1". golang_test.go
// verified this by literally running an equivalent compute() with `go run`
// during authoring (see the ground-truth test below, which re-verifies it
// in-process).
func goDeferRecoverTraceTest() testkit.Test {
	prompt := `Here is a Go program:

` + "```go" + `
` + goDeferRecoverTraceCode + `
` + "```" + `

What does this program print when run? Respond with only the number,
nothing else.`

	return testkit.Test{
		ID:          "go-defer-recover-trace",
		Category:    "programming",
		Subcategory: "golang",
		Description: "Trace the exact output of a defer/recover chain that mutates a named return value.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], -1, 0),
	}
}

// goSliceAppendAliasingCode is the inline snippet for
// goSliceAppendAliasingTest.
const goSliceAppendAliasingCode = `package main

import "fmt"

func main() {
	a := make([]int, 3, 5)
	a[0], a[1], a[2] = 1, 2, 3
	b := a[:2]
	b = append(b, 99)
	_ = b
	fmt.Println(a[2])
}`

// goSliceAppendAliasingTest: trace the exact printed output of a slice
// aliasing gotcha where append writes into a shared backing array.
//
// ground truth: cap(a[:2]) = cap(a) - 0 = 5 (slicing a[low:high] gives
// cap = cap(a) - low), so b := a[:2] has len 2 but cap 5, still sharing a's
// backing array. append(b, 99) has spare capacity, so it does not allocate a
// new array: it writes 99 into backing-array index 2 (len(b) before the
// append) and returns a length-3 slice. Backing-array index 2 is also a[2],
// so a[2] becomes 99 even though a itself was never explicitly assigned to.
// golang_test.go re-verifies this in-process.
func goSliceAppendAliasingTest() testkit.Test {
	prompt := `Here is a Go program:

` + "```go" + `
` + goSliceAppendAliasingCode + `
` + "```" + `

What does this program print when run? Respond with only the number,
nothing else.`

	return testkit.Test{
		ID:          "go-slice-append-aliasing",
		Category:    "programming",
		Subcategory: "golang",
		Description: "Trace the exact output of a slice-append aliasing gotcha on a shared backing array.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], 99, 0),
	}
}

// goMethodSetInterfaceCode is the inline snippet for
// goMethodSetInterfaceTest: a type whose only interface method has a
// pointer receiver, so only *Wrapper (not Wrapper) satisfies Stringer.
const goMethodSetInterfaceCode = `type Stringer interface {
	String() string
}

type Wrapper struct {
	val string
}

func (w *Wrapper) String() string {
	return "Wrapper(" + w.val + ")"
}`

// goMethodSetInterfaceTest: decide whether a value type and a pointer type
// each satisfy an interface whose method has a pointer receiver.
//
// ground truth: a pointer receiver method belongs to the method set of the
// pointer type only, not the value type (Go spec, "Method sets"). So
// `Wrapper{...}` (a value) does not implement Stringer -
// "Wrapper does not implement Stringer (method String has pointer
// receiver)" - while `&Wrapper{...}` (a *Wrapper) does. golang_test.go
// cross-checks both compile-time facts by running `go build` on each
// snippet, guarded by go-toolchain presence (as c_test.go does for cc).
func goMethodSetInterfaceTest() testkit.Test {
	prompt := `Here is Go code:

` + "```go" + `
` + goMethodSetInterfaceCode + `
` + "```" + `

Does this line compile: ` + "`var _ Stringer = Wrapper{val: \"hi\"}`" + `?
Does this line compile: ` + "`var _ Stringer = &Wrapper{val: \"hi\"}`" + `?

Respond with only a JSON object:
{"value_compiles": true|false, "pointer_compiles": true|false}`

	evaluator := eval.All(
		eval.W(eval.JSONField("value_compiles", false), 1),
		eval.W(eval.JSONField("pointer_compiles", true), 1),
	)

	return testkit.Test{
		ID:          "go-method-set-interface",
		Category:    "programming",
		Subcategory: "golang",
		Description: "Decide whether a value type and its pointer type satisfy an interface with a pointer-receiver method.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// goContextCancellationTraceCode is the inline snippet for
// goContextCancellationTraceTest.
const goContextCancellationTraceCode = `package main

import (
	"context"
	"fmt"
)

func main() {
	parent, cancelParent := context.WithCancel(context.Background())
	child, cancelChild := context.WithCancel(parent)
	defer cancelChild()

	cancelParent()

	select {
	case <-child.Done():
		fmt.Println(child.Err())
	default:
		fmt.Println("not done")
	}
}`

// goContextCancellationTraceTest: trace the exact printed output of
// cancellation propagating from a parent context to a context derived from
// it.
//
// ground truth: context.WithCancel propagates cancellation synchronously
// down the derivation tree - cancelParent() closes parent's Done channel
// and, in the same call, closes every context derived from it (including
// child), before cancelParent() returns. So by the time the select runs,
// child.Done() is already closed and takes the first case, printing
// child.Err(). Err() returns the sentinel context.Canceled, whose message
// (per the context package source) is "context canceled". golang_test.go
// re-verifies this in-process, including that the propagation really is
// synchronous (no goroutine/timing dependency, so the trace is
// deterministic).
func goContextCancellationTraceTest() testkit.Test {
	prompt := `Here is a Go program:

` + "```go" + `
` + goContextCancellationTraceCode + `
` + "```" + `

What does this program print when run? Respond with only the exact text
printed, nothing else.`

	return testkit.Test{
		ID:          "go-context-cancellation-trace",
		Category:    "programming",
		Subcategory: "golang",
		Description: "Trace the exact output of a context cancellation propagating synchronously from a parent to a derived context.",
		Prompt:      prompt,
		Eval:        eval.Equals("context canceled"),
	}
}

// goGenericsConstraintFixHarness calls the response's fixed Max function
// across int, float64, and string, verifying both the return values and
// that it compiles at all (a broken constraint fails to compile, which
// eval.GoRun scores as 0 via its "execution failed" / build-error path).
const goGenericsConstraintFixHarness = `package main

import (
	"fmt"
	"os"
)

func main() {
	if got := Max(3, 5); got != 5 {
		fmt.Fprintf(os.Stderr, "Max(3,5) = %v, want 5\n", got)
		os.Exit(1)
	}
	if got := Max(5, 3); got != 5 {
		fmt.Fprintf(os.Stderr, "Max(5,3) = %v, want 5\n", got)
		os.Exit(1)
	}
	if got := Max(2.5, 1.5); got != 2.5 {
		fmt.Fprintf(os.Stderr, "Max(2.5,1.5) = %v, want 2.5\n", got)
		os.Exit(1)
	}
	if got := Max("apple", "banana"); got != "banana" {
		fmt.Fprintf(os.Stderr, "Max(apple,banana) = %v, want banana\n", got)
		os.Exit(1)
	}
	fmt.Println("PASS")
}
`

// goGenericsConstraintFixTest: repair a generic function whose type
// parameter constraint (any) does not support the operator (>) its body
// uses, without depending on any external package.
//
// ground truth: the harness above is the oracle - it exercises Max across
// three concrete types the fixed constraint must accept (int, float64,
// string), so there is no separate hardcoded constant to derive.
func goGenericsConstraintFixTest() testkit.Test {
	prompt := `This Go generic function does not compile:

` + "```go" + `
func Max[T any](a, b T) T {
	if a > b {
		return a
	}
	return b
}
` + "```" + `

The compiler rejects it because type parameter T (constrained only by any)
does not support the > operator. Fix it with the smallest change that makes
it compile and work correctly for int, float64, and string arguments. Define
any constraint interface you need yourself, using only the standard library
- do not import golang.org/x/exp/constraints or any other third-party
package. Respond with the complete corrected code (constraint interface, if
any, plus the Max function).`

	return testkit.Test{
		ID:          "go-generics-constraint-fix",
		Category:    "programming",
		Subcategory: "golang",
		Description: "Fix a generic function's constraint so > compiles and works for int, float64, and string.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        eval.GoRun(goGenericsConstraintFixHarness, "PASS"),
	}
}

// goBoundedFanoutHarness drives the response's BoundedRun with two batches
// of tasks (all-succeed, then some-fail) over an instrumented task function
// that tracks observed concurrency via an atomic counter, the same pattern
// goWorkerPoolHarness uses. It checks: (a) the all-succeed batch returns
// nil, (b) the some-fail batch returns a non-nil error, and (c) in both
// batches, observed concurrency stayed within [2, limit] - never sequential
// (>=2, evidencing real concurrency), never over the requested bound.
const goBoundedFanoutHarness = `package main

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

func main() {
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
			time.Sleep(5 * time.Millisecond)
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
	if err := BoundedRun(okTasks, limit); err != nil {
		fmt.Fprintf(os.Stderr, "BoundedRun(all ok) = %v, want nil\n", err)
		os.Exit(1)
	}
	if got := atomic.LoadInt32(&maxConcurrent); got < 2 || got > limit {
		fmt.Fprintf(os.Stderr, "maxConcurrent=%d, want in [2, %d]\n", got, limit)
		os.Exit(1)
	}

	atomic.StoreInt32(&concurrent, 0)
	atomic.StoreInt32(&maxConcurrent, 0)
	failTasks := make([]func() error, n)
	for i := range failTasks {
		failTasks[i] = makeTask(i%3 == 0)
	}
	err := BoundedRun(failTasks, limit)
	if err == nil {
		fmt.Fprintln(os.Stderr, "BoundedRun(some fail) = nil, want an error")
		os.Exit(1)
	}
	if got := atomic.LoadInt32(&maxConcurrent); got < 2 || got > limit {
		fmt.Fprintf(os.Stderr, "maxConcurrent=%d, want in [2, %d]\n", got, limit)
		os.Exit(1)
	}

	fmt.Println("PASS")
}
`

// goBoundedFanoutTest: implement bounded-concurrency fan-out with
// first-error propagation, the same shape as
// golang.org/x/sync/errgroup.Group.SetLimit but implemented directly, so
// the exec evaluator never depends on an external module being resolvable
// in the sandbox (EXPANSION.md permits substituting an equivalent-difficulty
// topic when the literal one cannot be made deterministic offline).
//
// ground truth: the harness above is the oracle, exercising both
// correctness (first error propagates, nil on all-success) and the
// concurrency bound via the same observed-concurrency counter pattern as
// goWorkerPoolHarness; there is no separate hardcoded constant to derive.
func goBoundedFanoutTest() testkit.Test {
	prompt := `Implement this Go function:

` + "```go" + `
func BoundedRun(tasks []func() error, limit int) error
` + "```" + `

Requirements:
- Run the tasks concurrently, but never more than limit at the same time.
- If one or more tasks return a non-nil error, BoundedRun must return a
  non-nil error (any one of the failing tasks' errors is acceptable).
- If every task returns nil, BoundedRun must return nil.
- Use only the standard library (for example sync, sync/atomic, or
  channels). Do not import golang.org/x/sync/errgroup or any other
  third-party package.
- You may include whatever standard-library imports your implementation
  needs. Do not include a package clause or a main function: respond with
  only the BoundedRun function definition (plus any imports it needs).`

	return testkit.Test{
		ID:          "go-bounded-fanout",
		Category:    "programming",
		Subcategory: "golang",
		Description: "Implement bounded-concurrency fan-out with first-error propagation, using only the standard library.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        eval.GoRun(goBoundedFanoutHarness, "PASS"),
	}
}

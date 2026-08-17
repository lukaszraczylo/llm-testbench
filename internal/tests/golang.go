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

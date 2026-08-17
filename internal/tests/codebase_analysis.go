package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// codeBugDiffSnippet is a unified diff for codeBugLineDiffTest, with each
// displayed line prefixed by its 1-based position in this listing (not the
// diff's own hunk line numbers), so "which line" has one unambiguous
// meaning.
const codeBugDiffSnippet = `1: @@ -1,9 +1,9 @@
2:  func Average(nums []float64) float64 {
3:      if len(nums) == 0 {
4:          return 0
5:      }
6:      sum := 0.0
7: -    for _, n := range nums {
8: +    for i := 0; i <= len(nums); i++ {
9: -        sum += n
10: +        sum += nums[i]
11:      }
12:      return sum / float64(len(nums))
13:  }`

// codeBugLineDiffTest: identify the root-cause line of an off-by-one
// out-of-bounds bug in an inline, line-numbered diff.
//
// ground truth: line 8's new loop condition `i <= len(nums)` allows i to
// reach len(nums), and line 10 then indexes nums[i] at that out-of-range
// value, panicking. Line 8 is the root cause (the wrong bound); line 10 is
// only where the panic actually fires - the correct bound is `i <
// len(nums)`.
func codeBugLineDiffTest() testkit.Test {
	prompt := `Here is a diff modifying a Go function that computes an average. Each
displayed line below is prefixed with its position in this listing (not
the diff's own internal hunk numbering):

` + codeBugDiffSnippet + `

This change introduces a bug that causes a runtime panic: index out of
range. Which line number contains the root cause of the panic - the
incorrect loop bound - as opposed to the line where the panic actually
fires? Respond with only a JSON object: {"line":<number>}`

	return testkit.Test{
		ID:          "code-bug-line-diff",
		Category:    "research",
		Subcategory: "codebase",
		Description: "Identify the root-cause line of an off-by-one out-of-bounds bug in an inline, line-numbered diff.",
		Prompt:      prompt,
		Eval:        eval.JSONField("line", 8),
	}
}

// codeRaceSnippet is an inline Go function for codeRaceVariableTest with a
// classic unsynchronized shared-counter data race.
const codeRaceSnippet = `func Count(n int) int {
	total := 0
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			total++
		}()
	}
	wg.Wait()
	return total
}`

// codeRaceVariableTest: name the variable with a data race in an inline Go
// snippet.
//
// ground truth: every launched goroutine executes `total++` (a
// read-modify-write) on the same shared `total` variable with no mutex,
// atomic operation, or channel serializing access, so concurrent
// increments race. `n` and `i` are each goroutine-local or loop-local and
// are not raced on by this snippet.
func codeRaceVariableTest() testkit.Test {
	prompt := `Here is a Go function:

` + "```go\n" + codeRaceSnippet + "\n```" + `

Which single variable has a data race in this snippet - concurrent,
unsynchronized reads and writes from multiple goroutines? Respond with
only a JSON object: {"variable":"<name>"}`

	return testkit.Test{
		ID:          "code-race-variable",
		Category:    "research",
		Subcategory: "codebase",
		Description: "Name the racy shared variable in an inline Go snippet with an unsynchronized concurrent counter.",
		Prompt:      prompt,
		Eval:        eval.JSONField("variable", "total"),
	}
}

// codeDeadFunctionsSnippet is an inline Go file for codeDeadFunctionsTest
// with two functions reachable from main and two that are never called.
const codeDeadFunctionsSnippet = `package main

import "fmt"

func helperA() int { return 1 }
func helperB() int { return helperA() + 1 }
func helperC() int { return 3 }
func helperD() int { return helperC() * 2 }

func main() {
	fmt.Println(helperB())
}`

// codeDeadFunctionsTest: identify functions unreachable from main in an
// inline Go file.
//
// ground truth: main calls only helperB, which calls helperA - both are
// reachable. Nothing calls helperD, and the only caller of helperC is
// helperD itself, so neither helperC nor helperD is reachable from main;
// both are dead code.
func codeDeadFunctionsTest() testkit.Test {
	prompt := `Here is a small Go file:

` + "```go\n" + codeDeadFunctionsSnippet + "\n```" + `

Which functions are dead code - never reachable, directly or transitively,
from main? Respond with only a JSON array of function names.`

	return testkit.Test{
		ID:          "code-dead-functions",
		Category:    "research",
		Subcategory: "codebase",
		Description: "Identify functions unreachable from main in an inline Go file with both live and dead call chains.",
		Prompt:      prompt,
		Eval:        eval.JSONStringSet([]string{"helperC", "helperD"}),
	}
}

// codeDedupSnippet is an inline Go function for codeBigODedupTest with a
// nested-loop worst-case-quadratic deduplication implementation.
const codeDedupSnippet = `func Dedup(a []int) []int {
	var out []int
	for _, x := range a {
		found := false
		for _, y := range out {
			if x == y {
				found = true
				break
			}
		}
		if !found {
			out = append(out, x)
		}
	}
	return out
}`

// codeBigODedupPattern accepts every common notation for O(n^2): a caret,
// a double-star, the unicode superscript two, or n*n, each optionally
// wrapped as O(...) with flexible internal spacing.
const codeBigODedupPattern = `(?i)o\(\s*n\s*(\^\s*2|\*\*\s*2|²|\*\s*n)\s*\)`

// codeBigODedupTest: state the worst-case time complexity of an inline
// nested-loop deduplication function.
//
// ground truth: when a has no duplicates, the inner loop scans the whole
// of out, which grows to length n, for every one of the n outer
// iterations, giving n + (n-1) + ... = O(n^2) comparisons in the worst
// case.
func codeBigODedupTest() testkit.Test {
	prompt := `Here is a Go function:

` + "```go\n" + codeDedupSnippet + "\n```" + `

What is the worst-case time complexity of Dedup, in Big-O notation, in
terms of n = len(a)? Respond with only the Big-O expression, e.g. O(n).`

	return testkit.Test{
		ID:          "code-bigo-dedup",
		Category:    "research",
		Subcategory: "codebase",
		Description: "State the worst-case Big-O time complexity of an inline nested-loop deduplication function.",
		Prompt:      prompt,
		Eval:        eval.Regex(codeBigODedupPattern),
	}
}

// codeImportListSnippet lists each inline package's direct imports for
// codeImportCycleTest, with one cycle among four packages.
const codeImportListSnippet = `- package a imports: [b]
- package b imports: [c]
- package c imports: [a, d]
- package d imports: []`

// codeImportCycleTest: detect which packages form an import cycle from an
// inline list of per-package direct imports.
//
// ground truth: following the edges a->b->c->a closes a cycle among a, b,
// and c. Package d is imported by c but does not import anything itself,
// so it is not part of any cycle.
func codeImportCycleTest() testkit.Test {
	prompt := `Here are the direct imports of four packages:

` + codeImportListSnippet + `

Which packages form an import cycle? Respond with only a JSON array of
package names involved in the cycle (any order).`

	return testkit.Test{
		ID:          "code-import-cycle",
		Category:    "research",
		Subcategory: "codebase",
		Description: "Detect which packages form an import cycle from an inline list of per-package direct imports.",
		Prompt:      prompt,
		Eval:        eval.JSONStringSet([]string{"a", "b", "c"}),
	}
}

// codeGenericFirstSnippet is an inline Go generic function plus a call
// site for codeGenericReturnTypeTest.
const codeGenericFirstSnippet = `func First[T any](s []T) (T, bool) {
	if len(s) == 0 {
		var zero T
		return zero, false
	}
	return s[0], true
}

result, ok := First([]string{"a", "b"})`

// codeGenericReturnTypeTest: infer the concrete instantiated type of a
// generic function's result at a specific call site.
//
// ground truth: First is instantiated with T = string because it is
// called with []string{"a", "b"}, so result's concrete type is string.
func codeGenericReturnTypeTest() testkit.Test {
	prompt := `Here is a generic Go function and a call site:

` + "```go\n" + codeGenericFirstSnippet + "\n```" + `

What is the concrete type of the variable "result" in the code above?
Respond with only the type name, nothing else.`

	return testkit.Test{
		ID:          "code-generic-return-type",
		Category:    "research",
		Subcategory: "codebase",
		Description: "Infer the concrete instantiated type of a generic Go function's result at a specific call site.",
		Prompt:      prompt,
		Eval:        codeExactAnswer("string"),
	}
}

// codeBisectCommitLog is an inline commit history for
// codeCommitBisectTest, where exactly one commit changes safeDivide's
// result cache to be keyed only by "a" and checked before validation.
const codeBisectCommitLog = `c1a1a1a "init: add safeDivide(a, b) that returns an error if b == 0,
otherwise returns a / b"

c2b2b2b "refactor: rewrite safeDivide's internal branch as a plain
if/else, no behavior change"

c3c3c3c "perf: add a result cache to safeDivide, keyed only by a (not by
the pair a and b), checked BEFORE the b == 0 validation runs"

c4d4d4d "chore: add debug logging around cache hits in safeDivide"

c5e5e5e "docs: fix a typo in the cache-hit log message"`

// codeCommitBisectTest: use git-bisect-style reasoning over an inline
// commit log and a described symptom to name the exact commit that
// introduced the bug.
//
// ground truth: c3c3c3c's cache is keyed only by the first argument a and
// is checked before the b == 0 validation runs. A prior call
// safeDivide(10, 2) caches a successful result under key a=10; a later
// call safeDivide(10, 0) then hits that stale cache entry (same a, cache
// ignores b) and returns it instead of reaching the validation that would
// raise the b == 0 error. c1 predates the cache entirely; c2 is a
// no-behavior-change refactor; c4 and c5 only touch logging text and do
// not change the cache's key or check order, so the bug already existed
// before either of them.
func codeCommitBisectTest() testkit.Test {
	prompt := `Here is a commit history for a function safeDivide(a, b), oldest first:

` + codeBisectCommitLog + `

Symptom observed in production after deploying through the latest commit:
calling safeDivide(10, 2) returns 5 (correct). Immediately afterward,
calling safeDivide(10, 0) returns 5 instead of raising the expected
"b == 0" error.

Using git-bisect-style reasoning, which commit introduced this bug?
Respond with only the commit id (the 7-character id shown), nothing else.`

	return testkit.Test{
		ID:          "code-commit-bisect",
		Category:    "research",
		Subcategory: "codebase",
		Description: "Use git-bisect-style reasoning over an inline commit log and a symptom to name the commit that introduced the bug.",
		Prompt:      prompt,
		Eval:        codeExactAnswer("c3c3c3c"),
	}
}

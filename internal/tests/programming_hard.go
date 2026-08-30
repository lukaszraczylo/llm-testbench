package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// registerHardTests registers every programming/hard test. This subcategory
// stresses harder daily-programming accuracy - concurrency/async reasoning,
// subtle language traps, algorithm output tracing, and API-contract edge
// cases - each forced to a short, machine-checkable answer.
func registerHardTests(r *testkit.Registry) {
	r.Register(hardFloatBinaddTest())
	r.Register(hardNullishOverOrTest())
	r.Register(hardGoroutineRaceTest())
	r.Register(hardDeferNamedReturnTest())
	r.Register(hardIntDivisionTest())
	r.Register(hardRegexGreedyTest())
	r.Register(hardPromiseAllSettledTest())
	r.Register(hardContextTimeoutTest())
	r.Register(hardJSONUnmarshalNullTest())
	r.Register(hardVarShadowTest())
	r.Register(hardPyVersionSortTest())
	r.Register(hardGoParenBalanceTest())
}

// hardFloatBinaddTest: judge whether 0.1 + 0.2 === 0.3 holds in
// JavaScript / IEEE-754 doubles.
//
// ground truth: neither 0.1 nor 0.2 has an exact binary representation,
// so 0.1 + 0.2 computes to the nearest double to the mathematical sum,
// which is 0.30000000000000004, not the double for the literal 0.3.
// Therefore the strict equality comparison evaluates to false. This is a
// canonical, deterministic fact of IEEE-754 double arithmetic, independent
// of any runtime; verified with node during authoring.
func hardFloatBinaddTest() testkit.Test {
	prompt := `In JavaScript, evaluate this comparison:

0.1 + 0.2 === 0.3

Does the strict-equality comparison evaluate to true or to false?
Respond with only a JSON object:
{"equal": true}
or
{"equal": false}`

	return testkit.Test{
		ID:          "hard-float-binadd",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Judge IEEE-754 double equality for 0.1 + 0.2 === 0.3 in JavaScript (false).",
		Prompt:      prompt,
		Eval:        eval.JSONField("equal", false),
	}
}

// hardNullishOverOrTest: trace the value of a nullish-coalescing
// expression over a falsy (but non-null) operand.
//
// ground truth: ?? returns the right-hand operand only when the left-hand
// operand is null or undefined. 0 is neither null nor undefined, so
// n ?? 42 evaluates to 0 - unlike ||, which would have returned 42 for
// the falsy 0. This is the core distinction between nullish coalescing
// and logical OR. Verified with node during authoring.
func hardNullishOverOrTest() testkit.Test {
	prompt := `Consider this TypeScript code:

const n = 0;
const v = n ?? 42;

What is the value of v?
Respond with only a JSON object:
{"value": ...}

Give the numeric value only.`

	return testkit.Test{
		ID:          "hard-nullish-over-or",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Trace nullish-coalescing over a falsy-but-non-null operand: 0 ?? 42 is 0, not 42.",
		Prompt:      prompt,
		Eval:        eval.JSONField("value", 0),
	}
}

// hardGoroutineRaceTest: judge whether an unsynchronized shared counter
// is guaranteed to reach N*M exactly.
//
// ground truth: incrementing a shared int from multiple goroutines with no
// synchronization is a data race (Go's race detector would flag it). A
// read-modify-write of a shared variable is not atomic; two goroutines can
// read the same value before either writes back, so increments are lost.
// The final value is some value strictly between M (at best serialized,
// one goroutine's work) and N*M, but it is NOT guaranteed to equal
// N*M. This claim follows from the Go memory model / data-race semantics
// and cannot be recomputed by running (the outcome is nondeterministic).
func hardGoroutineRaceTest() testkit.Test {
	prompt := `A Go program starts 8 goroutines, each of which performs:
counter++
1000 times, where counter is a shared int with NO synchronization
(no mutex, no atomic) between the goroutines. The program then waits for
all 8 goroutines and prints counter.

Is counter's final value guaranteed to be exactly 8000?
Respond with only a JSON object:
{"guaranteed": true}
or
{"guaranteed": false}`

	return testkit.Test{
		ID:          "hard-goroutine-race",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Judge whether an unsynchronized shared counter is guaranteed to reach N*M (false: data race).",
		Prompt:      prompt,
		Eval:        eval.JSONField("guaranteed", false),
	}
}

// hardDeferNamedReturnTest: trace the return value of a function whose
// deferred closure writes its named return during a normal (non-panic)
// return.
//
// ground truth: return 7 first assigns the named result r = 7; then,
// still before the function returns to its caller, the deferred closure
// runs and overwrites r = 99. The caller observes the named return after
// all defers have run, so f() returns 99 - not 7 and not 5. This is the
// classic named-return-plus-defer trap. Verified with go run during
// authoring.
func hardDeferNamedReturnTest() testkit.Test {
	prompt := `Consider this Go code:

func f() (r int) {
	defer func() { r = 99 }()
	r = 5
	return 7
}

What value does f() return?
Respond with only the number, no explanation.`

	return testkit.Test{
		ID:          "hard-defer-named-return",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Trace a deferred closure overwriting a named return value during a normal return (99).",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], hardDeferNamedReturnWant, 0),
	}
}

// hardDeferNamedReturnWant is f()'s return value.
//
// ground truth: the deferred closure runs during the function's epilogue,
// after the explicit `return 7` has set the named result r = 7, and
// overwrites it with 99; the caller therefore receives 99 (see the full
// derivation on hardDeferNamedReturnTest). Recomputed by running the
// identical snippet with go run during authoring.
const hardDeferNamedReturnWant = 99

// hardIntDivisionTest: judge the result of integer division being
// assigned to a floating-point variable.
//
// ground truth: a / b with two int operands performs integer division,
// which truncates toward zero: 7 / 2 = 3 (not 3.5). That integral
// result is then implicitly converted to double, so d = 3.0. The common
// error is assuming floating division because the target is a double.
// Verified with go run during authoring.
func hardIntDivisionTest() testkit.Test {
	prompt := `Consider this C code:

int a = 7;
int b = 2;
double d = a / b;

What is the value of d?
Respond with only the number, no explanation.`

	return testkit.Test{
		ID:          "hard-int-division",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Trace integer-division-then-widen: 7 / 2 is 3 (stored as 3.0), not 3.5.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[float64], hardIntDivisionWant, 0),
	}
}

// hardIntDivisionWant is d's value.
//
// ground truth: integers divide as integers (truncating), giving 7 / 2 =
// 3, converted to double 3.0; the float value is 3, so Numeric want is
// 3.0. Recomputed by running the identical arithmetic during authoring.
const hardIntDivisionWant = 3.0

// hardRegexGreedyTest: trace which substrings two greedy capture groups
// of an anchored pattern match.
//
// ground truth: quantifiers are greedy by default - each takes as much as
// it can while still allowing the rest of the pattern to match. In
// /^(\d+)(\d+)$/, the first \d+ grabs the longest prefix that leaves at
// least one digit for the second \d+ (which needs at least one match
// because + is one-or-more). That longest prefix of "12345" is "1234",
// leaving "5" for group 2. A naive symmetric split like ["12","345"]
// fails because group 1 would then still be able to grow. Verified with
// node during authoring.
func hardRegexGreedyTest() testkit.Test {
	prompt := `Consider this regular expression applied to the string "12345":

^(\d+)(\d+)$

(^ and $ anchor to the whole string, and \d+ means one or more digits.)

What are the two captured groups? Respond with only a JSON array of the
two captured strings, in group order, for example ["a","b"]`

	return testkit.Test{
		ID:          "hard-regex-greedy",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Trace greedy capture-group backtracking: ^(one-or-more digits)(one-or-more digits)$ on 12345 yields 1234 and 5.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals([]string{"1234", "5"}),
	}
}

// hardPromiseAllSettledTest: trace the per-result status values of
// Promise.allSettled over a mixed resolve/reject input.
//
// ground truth: Promise.allSettled always resolves, and each element of
// its result array is an object whose status is the literal "fulfilled"
// for a resolved input and "rejected" for a rejected input, preserving
// input order. Promise.resolve(42) yields {status:"fulfilled", value:42};
// Promise.reject(new Error("boom")) yields {status:"rejected",
// reason:...}. So the two status values, in order, are "fulfilled" then
// "rejected". Verified with node during authoring.
func hardPromiseAllSettledTest() testkit.Test {
	prompt := `Consider this TypeScript code:

const results = await Promise.allSettled([
  Promise.resolve(42),
  Promise.reject(new Error("boom")),
]);

Each element of results has a "status" field.

List the two status values, in array order. Respond with only a JSON
array of the two status strings, for example ["a","b"]`

	return testkit.Test{
		ID:          "hard-promise-allsettled",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Trace Promise.allSettled result statuses over a fulfilled+rejected input (fulfilled, rejected).",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals([]string{"fulfilled", "rejected"}),
	}
}

// hardContextTimeoutTest: identify the exact error string a timed-out
// context's Err() returns.
//
// ground truth: when a context created by context.WithTimeout fires, its
// Done channel closes and Err() returns the sentinel
// context.DeadlineExceeded, whose Error() string (per the context
// package) is exactly "context deadline exceeded" - which is distinct
// from "context canceled", the string of context.Canceled returned when a
// cancel function is called. Verified with go run during authoring.
func hardContextTimeoutTest() testkit.Test {
	prompt := `In Go, a context created with context.WithTimeout fires after its
deadline. At that point:

ctx.Err()

returns a non-nil error. What is the exact string of that error?
Respond with only a JSON object:
{"err": "..."}

with the exact string value.`

	return testkit.Test{
		ID:          "hard-context-timeout",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Identify ctx.Err() after a context.WithTimeout deadline fires (context deadline exceeded).",
		Prompt:      prompt,
		Eval:        eval.JSONField("err", "context deadline exceeded"),
	}
}

// hardJSONUnmarshalNullTest: judge the behavior of json.Unmarshal on a
// literal JSON null into an already-populated struct.
//
// ground truth: per encoding/json, a JSON null into a non-nil pointer
// target is a no-op: Unmarshal returns nil (no error) and leaves the
// target's existing value unchanged. It does not zero the struct and does
// not error. Verified with go run during authoring (a struct holding
// Items: [1] stays [1] with nil error).
func hardJSONUnmarshalNullTest() testkit.Test {
	prompt := `In Go:

var s Struct  // s is a populated struct value (some fields set)
err := json.Unmarshal([]byte("null"), &s)

Does Unmarshal return an error, and is s left unchanged (no fields
cleared)?
Respond with only a JSON object:
{"returns_error": true}
or
{"returns_error": false}`

	return testkit.Test{
		ID:          "hard-json-unmarshal-null",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Judge json.Unmarshal of a literal null into a populated struct (no error, unchanged).",
		Prompt:      prompt,
		Eval:        eval.JSONField("returns_error", false),
	}
}

// hardVarShadowTest: judge whether an inner := declaration shadows an
// outer variable without changing it.
//
// ground truth: `count := 5` inside the if block declares a NEW, inner
// variable that shadows the outer count for the duration of that block
// (Go's := always creates a new variable when at least one variable on
// the left is new in the current scope). Incrementing the inner count
// leaves the outer count untouched, so after the block outer count is
// still 0. Verified with go run during authoring.
func hardVarShadowTest() testkit.Test {
	prompt := `Consider this Go code:

count := 0
if true {
	count := 5
	count++
}
// count here?

Does the outer count remain unchanged (equal to 0) after the if block?
Respond with only a JSON object:
{"outer_unchanged": true}
or
{"outer_unchanged": false}`

	return testkit.Test{
		ID:          "hard-var-shadow",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Trace Go variable shadowing: an inner := leaves the outer variable unchanged (true).",
		Prompt:      prompt,
		Eval:        eval.JSONField("outer_unchanged", true),
	}
}

// hardPyVersionVersions is the inline version list for
// hardPyVersionSortTest.
const hardPyVersionVersions = `versions = [
    "1.9.0",
    "1.10.0",
    "1.2.0",
    "2.0.0",
    "0.9.9",
]`

// hardPyVersionSortWant is the expected stdout of a correct script: the
// second-highest version under numeric component-by-component ordering.
//
// ground truth: ordering versions by numeric components puts 0.9.9 <
// 1.2.0 < 1.9.0 < 1.10.0 < 2.0.0 (1.10.0 is newer than 1.9.0 because
// its second component 10 > 9), so the second-highest is 1.10.0. A
// plain lexicographic string sort (the trap) would instead report 1.9.0,
// which is exactly the failure the test discriminates. Recomputed by
// running the correct script with python3 during authoring.
const hardPyVersionSortWant = "1.10.0"

// hardPyVersionSortTest: write a python3 script that prints the
// second-highest version under numeric component ordering.
//
// ground truth: see hardPyVersionSortWant; correctness is determined by
// running the script (version ordering is behavior, not a text match).
func hardPyVersionSortTest() testkit.Test {
	prompt := `Here is an inline list of software version strings:

` + "```python" + `
` + hardPyVersionVersions + `
` + "```" + `

Write a python3 program that sorts the versions numerically, component by
component (natural version ordering: "1.10.0" is newer than "1.9.0"
because the second component 10 > 9), and prints only the second-highest
version.`

	return testkit.Test{
		ID:          "hard-py-version-sort",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Write a python3 script printing the second-highest version by numeric component ordering.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        eval.PyRun(eval.PassthroughHarness, hardPyVersionSortWant),
	}
}

// hardGoParenBalanceHarness is a complete, independent driver file (its
// own package clause and imports) that eval.GoRun writes to harness.go
// alongside the model's IsBalanced in solution.go. It drives IsBalanced
// across balanced, nested, mismatched, unclosed, and wrongly-ordered
// cases, printing PASS only if every case matches its expected result.
const hardGoParenBalanceHarness = `package main

import (
	"fmt"
	"os"
)

func main() {
	cases := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"()", true},
		{"()[]{}", true},
		{"([{}])", true},
		{"(", false},
		{"(]", false},
		{"([)]", false},
		{"((", false},
		{")(", false},
	}
	for _, c := range cases {
		if got := IsBalanced(c.in); got != c.want {
			fmt.Printf("FAIL %q: got %v want %v\n", c.in, got, c.want)
			os.Exit(1)
		}
	}
	fmt.Println("PASS")
}`

// hardGoParenBalanceTest: implement a stack-based balanced-parentheses
// checker.
//
// ground truth: the harness above is the oracle - it exercises empty,
// balanced, nested, mismatched-closer, trailing-opener, and wrong-order
// inputs, so a correct stack-based implementation prints PASS. There is no
// separate hardcoded constant beyond the PASS sentinel.
func hardGoParenBalanceTest() testkit.Test {
	prompt := `Implement a Go function with exactly this signature:

func IsBalanced(s string) bool

It must report whether s's parentheses are balanced and correctly nested:
'(' '[' '{' are openers, ')' ']' '}' are the matching closers. An empty
string is balanced (return true). Return false for any unclosed opener,
any closer with no matching open, or any wrong-nesting (e.g. "([)]").`

	return testkit.Test{
		ID:          "hard-go-paren-balance",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Implement a stack-based balanced-parentheses checker for (), [], and {}.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        eval.GoRun(hardGoParenBalanceHarness, "PASS"),
	}
}

package tests

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// codeExactAnswer returns an Evaluator awarding full credit when the
// response, trimmed of whitespace, at most one layer of surrounding quote
// characters (' , ", or `), and a single trailing sentence-ending period,
// equals want case-insensitively. This accepts every materially-correct
// form of a forced single-token answer (bare, quoted, differently-cased,
// or with trailing punctuation) without loosening the match to accept a
// substring of a longer, wrong answer.
func codeExactAnswer(want string) eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		got := strings.TrimSpace(response)
		got = strings.Trim(got, "\"'`")
		got = strings.TrimSuffix(strings.TrimSpace(got), ".")
		got = strings.TrimSpace(got)
		wantTrimmed := strings.TrimSpace(want)
		if strings.EqualFold(got, wantTrimmed) {
			return eval.Score{Value: 1, Detail: fmt.Sprintf("equals %q", wantTrimmed)}
		}
		return eval.Score{Value: 0, Detail: fmt.Sprintf("got %q, want %q", got, wantTrimmed)}
	})
}

// codeTracePythonSource is the inline Python function traced by
// codeTracePythonTest.
const codeTracePythonSource = `def transform(s, n):
    if n == 0:
        return s
    if len(s) % 2 == 0:
        s = s[1:] + s[0]
    else:
        s = s[::-1]
    return transform(s, n - 1)`

// codeTracePythonWant is the exact result of transform("abcdef", 4).
//
// ground truth: len("abcdef") is 6 (even) at every recursive call, since
// the rotate branch never changes length, so every call takes the rotate
// branch: "abcdef"->"bcdefa"->"cdefab"->"defabc"->"efabcd" over 4 calls
// (n=4,3,2,1), then n=0 returns "efabcd". codebase_trace_test.go both
// hand-derives this independently and, when python3 is available, runs
// the source above directly and compares stdout.
const codeTracePythonWant = "efabcd"

// codeTracePythonTest: trace a recursive Python function combining a
// parity check, string rotation, and full reversal to its exact output.
func codeTracePythonTest() testkit.Test {
	prompt := `Here is a Python function:

` + "```python\n" + codeTracePythonSource + "\n```" + `

What is the exact return value of transform("abcdef", 4)? Respond with
only the resulting string, with no quotes and no other text.`

	return testkit.Test{
		ID:          "code-trace-python",
		Category:    "research",
		Subcategory: "codebase",
		Description: "Trace a recursive Python function combining a parity check, string rotation, and full reversal to its exact output.",
		Prompt:      prompt,
		Eval:        codeExactAnswer(codeTracePythonWant),
	}
}

// codeTraceTSSource is the inline TypeScript function traced by
// codeTraceTSTest.
const codeTraceTSSource = `function pack(items: number[]): string {
  return items.reduce((acc, x, i) => {
    if (i % 2 === 0) return acc + x;
    return acc + x * 2;
  }, 0).toString(2);
}`

// codeTSPackTrace replicates pack's reduce logic in Go so
// codebase_trace_test.go can cross-check the hand-derived trace by
// running equivalent logic, since the TypeScript itself is not executed.
func codeTSPackTrace(items []int) string {
	acc := 0
	for i, x := range items {
		if i%2 == 0 {
			acc += x
		} else {
			acc += x * 2
		}
	}
	return strconv.FormatInt(int64(acc), 2)
}

// codeTraceTSWant is the exact result of pack([3,4,5,6,7]).
//
// ground truth: acc starts at 0; i=0,x=3 (even index) -> acc=3; i=1,x=4
// (odd index) -> acc=3+4*2=11; i=2,x=5 -> acc=11+5=16; i=3,x=6 ->
// acc=16+6*2=28; i=4,x=7 -> acc=28+7=35. 35 in binary is 100011
// (32+2+1=35). codebase_trace_test.go recomputes this both by hand and by
// calling codeTSPackTrace.
const codeTraceTSWant = "100011"

// codeTraceTSTest: trace a TypeScript reduce-based function mixing
// index-parity branching and binary string conversion to its exact
// output.
func codeTraceTSTest() testkit.Test {
	prompt := `Here is a TypeScript function:

` + "```ts\n" + codeTraceTSSource + "\n```" + `

What is the exact return value of pack([3, 4, 5, 6, 7])? Respond with only
the resulting string, with no quotes and no other text.`

	return testkit.Test{
		ID:          "code-trace-ts",
		Category:    "research",
		Subcategory: "codebase",
		Description: "Trace a TypeScript reduce-based function mixing index-parity branching and binary string conversion to its exact output.",
		Prompt:      prompt,
		Eval:        codeExactAnswer(codeTraceTSWant),
	}
}

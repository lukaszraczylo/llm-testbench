package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerCodebaseTests(r *testkit.Registry) {
	r.Register(codeTraceGoTest())
	r.Register(codeTracePythonTest())
	r.Register(codeTraceTSTest())
	r.Register(codeBugLineDiffTest())
	r.Register(codeRaceVariableTest())
	r.Register(codeDeadFunctionsTest())
	r.Register(codeBigODedupTest())
	r.Register(codeImportCycleTest())
	r.Register(codeGenericReturnTypeTest())
	r.Register(codeCommitBisectTest())
}

// mix is the traced function for codeTraceGoTest: deterministic bit
// manipulation plus recursion. It is called directly (not re-implemented)
// to derive codeTraceGoWant, and codebase_test.go calls it again with the
// same input to ground that value, per PLAN.md's instruction that this
// test's ground truth be established by running the same function.
func mix(n int, depth int) int {
	if depth == 0 {
		return n
	}
	if n%2 == 0 {
		n = n >> 1
	} else {
		n = (n << 1) ^ 0x2A
	}
	return mix(n, depth-1) + depth
}

// codeTraceGoWant is mix(37, 4), computed by running mix itself.
//
// ground truth: mix(37,4): 37 is odd -> n=(37<<1)^0x2A=96, return
// mix(96,3)+4. 96 is even -> n=48, return mix(48,2)+3. 48 is even -> n=24,
// return mix(24,1)+2. 24 is even -> n=12, return mix(12,0)+1. depth=0 ->
// return 12. Unwinding: 12+1=13, 13+2=15, 15+3=18, 18+4=22.
var codeTraceGoWant = mix(37, 4)

func codeTraceGoTest() testkit.Test {
	prompt := `Here is a Go function:

` + "```go" + `
func mix(n int, depth int) int {
	if depth == 0 {
		return n
	}
	if n%2 == 0 {
		n = n >> 1
	} else {
		n = (n << 1) ^ 0x2A
	}
	return mix(n, depth-1) + depth
}
` + "```" + `

What is the exact value of mix(37, 4)? Respond with only the number,
nothing else.`

	return testkit.Test{
		ID:          "code-trace-go",
		Category:    "research",
		Subcategory: "codebase",
		Description: "Trace a recursive Go function combining bit manipulation and recursion to an exact numeric result.",
		Prompt:      prompt,
		MaxTokens:   200,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], codeTraceGoWant, 0),
	}
}

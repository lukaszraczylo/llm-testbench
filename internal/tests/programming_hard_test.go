package tests

import (
	"context"
	"testing"
)

func TestHardFloatBinaddTest_Eval(t *testing.T) {
	tc := hardFloatBinaddTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"equal":false}`, 1},
		{"correct fenced with prose", "The answer is:\n```json\n{\"equal\":false}\n```", 1},
		{"wrong: claims equal", `{"equal":true}`, 0},
		{"wrong: non-boolean", `{"equal":"yes"}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestHardNullishOverOrTest_Eval(t *testing.T) {
	tc := hardNullishOverOrTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"value":0}`, 1},
		{"correct as string", `{"value":"0"}`, 1},
		{"wrong: OR semantics", `{"value":42}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestHardGoroutineRaceTest_Eval(t *testing.T) {
	tc := hardGoroutineRaceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"guaranteed":false}`, 1},
		{"correct fenced", "```json\n{\"guaranteed\":false}\n```", 1},
		{"wrong: claims guaranteed", `{"guaranteed":true}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestHardDeferNamedReturnTest_Eval(t *testing.T) {
	tc := hardDeferNamedReturnTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "99", 1},
		{"correct prose-wrapped", "The function returns 99.", 1},
		{"wrong: returns statement value", "7", 0},
		{"wrong: pre-defer value", "5", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestHardIntDivisionTest_Eval(t *testing.T) {
	tc := hardIntDivisionTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "3", 1},
		{"correct as float", "3.0", 1},
		{"correct prose-wrapped", "It is 3.", 1},
		{"wrong: assumes float division", "3.5", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestHardRegexGreedyTest_Eval(t *testing.T) {
	tc := hardRegexGreedyTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `["1234","5"]`, 1},
		{"correct fenced", "```json\n[\"1234\",\"5\"]\n```", 1},
		{"wrong: symmetric split", `["12","345"]`, 0},
		{"wrong: reversed", `["5","1234"]`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestHardPromiseAllSettledTest_Eval(t *testing.T) {
	tc := hardPromiseAllSettledTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `["fulfilled","rejected"]`, 1},
		{"correct fenced", "```json\n[\"fulfilled\",\"rejected\"]\n```", 1},
		{"wrong: reordered", `["rejected","fulfilled"]`, 0},
		{"wrong: both fulfilled", `["fulfilled","fulfilled"]`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestHardContextTimeoutTest_Eval(t *testing.T) {
	tc := hardContextTimeoutTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"err":"context deadline exceeded"}`, 1},
		{"correct fenced", "```json\n{\"err\":\"context deadline exceeded\"}\n```", 1},
		{"wrong: canceled sentinel", `{"err":"context canceled"}`, 0},
		{"wrong: vague", `{"err":"some error"}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestHardJSONUnmarshalNullTest_Eval(t *testing.T) {
	tc := hardJSONUnmarshalNullTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"returns_error":false}`, 1},
		{"correct fenced", "```json\n{\"returns_error\":false}\n```", 1},
		{"wrong: claims error", `{"returns_error":true}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestHardVarShadowTest_Eval(t *testing.T) {
	tc := hardVarShadowTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"outer_unchanged":true}`, 1},
		{"correct fenced", "```json\n{\"outer_unchanged\":true}\n```", 1},
		{"wrong: claims outer changed", `{"outer_unchanged":false}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestHardPyVersionSortTest_Eval exercises the PyRun evaluator with a
// hand-written correct script and a lexicographic (wrong) script.
func TestHardPyVersionSortTest_Eval(t *testing.T) {
	tc := hardPyVersionSortTest()

	correctScript := "```python\n" + `versions = [
    "1.9.0",
    "1.10.0",
    "1.2.0",
    "2.0.0",
    "0.9.9",
]

def key(v):
    return tuple(int(p) for p in v.split("."))

sorted_versions = sorted(versions, key=key)
print(sorted_versions[-2])
` + "```"

	wrongLexicographic := "```python\n" + `versions = [
    "1.9.0",
    "1.10.0",
    "1.2.0",
    "2.0.0",
    "0.9.9",
]
print(sorted(versions)[-2])
` + "```"

	wrongHardcoded := "```python\nprint('2.0.0')\n```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct numeric sort", correctScript, 1},
		{"wrong: lexicographic sort", wrongLexicographic, 0},
		{"wrong: hardcoded highest", wrongHardcoded, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Skipped {
				t.Skip("python3 not available, skipping exec-evaluator assertion")
			}
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestHardGoParenBalanceTest_Eval exercises the GoRun evaluator with a
// hand-written correct function and a wrong one.
func TestHardGoParenBalanceTest_Eval(t *testing.T) {
	tc := hardGoParenBalanceTest()

	correctFunc := "```go\n" + `func IsBalanced(s string) bool {
	pairs := map[byte]byte{')': '(', ']': '[', '}': '{'}
	stack := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '{':
			stack = append(stack, s[i])
		case ')', ']', '}':
			if len(stack) == 0 || stack[len(stack)-1] != pairs[s[i]] {
				return false
			}
			stack = stack[:len(stack)-1]
		}
	}
	return len(stack) == 0
}
` + "```"

	wrongFunc := "```go\n" + `func IsBalanced(s string) bool {
	return true
}
` + "```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct stack implementation", correctFunc, 1},
		{"wrong: always true", wrongFunc, 0},
		{"wrong: no code at all", "I would use a stack here.", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Skipped {
				t.Skip("go toolchain not available, skipping exec-evaluator assertion")
			}
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestHardDeferNamedReturnWant_GroundTruth recomputes f()'s return value
// by running an equivalent function in-process, independent of the
// hardDeferNamedReturnWant constant.
func TestHardDeferNamedReturnWant_GroundTruth(t *testing.T) {
	f := func() (r int) {
		defer func() { r = 99 }()
		r = 5
		return 7
	}
	if got := f(); got != hardDeferNamedReturnWant {
		t.Errorf("equivalent deferred-named-return function returned %d, want %d", got, hardDeferNamedReturnWant)
	}
}

// TestHardIntDivisionWant_GroundTruth recomputes for a double result of
// 7 / 2 by hand, independent of the hardIntDivisionWant constant.
func TestHardIntDivisionWant_GroundTruth(t *testing.T) {
	a, b := 7, 2
	want := float64(a / b) // integer division truncates before widening
	if want != hardIntDivisionWant {
		t.Errorf("recomputed integer division = %v, want %v", want, hardIntDivisionWant)
	}
}

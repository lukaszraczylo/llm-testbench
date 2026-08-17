package tests

import (
	"context"
	"testing"
)

func TestCodeBugLineDiffTest_Eval(t *testing.T) {
	tc := codeBugLineDiffTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct: root-cause line", `{"line":8}`, 1},
		// B9: a numeric-string JSON value must coerce the same as a
		// native number - the prompt asked for the right line, not a
		// specific JSON type.
		{"correct as a numeric string", `{"line":"8"}`, 1},
		{"wrong: points at the panic site instead of the root cause", `{"line":10}`, 0},
		{"wrong line entirely", `{"line":6}`, 0},
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

func TestCodeRaceVariableTest_Eval(t *testing.T) {
	tc := codeRaceVariableTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"variable":"total"}`, 1},
		{"correct different case", `{"variable":"Total"}`, 1},
		{"wrong: loop variable, not the shared counter", `{"variable":"i"}`, 0},
		{"wrong: function parameter, not the shared counter", `{"variable":"n"}`, 0},
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

func TestCodeDeadFunctionsTest_Eval(t *testing.T) {
	tc := codeDeadFunctionsTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct set", `["helperC","helperD"]`, 1},
		{"correct set, different order", `["helperD","helperC"]`, 1},
		{"missing one dead function", `["helperD"]`, 0.5},
		{"wrongly includes a live function", `["helperC","helperD","helperB"]`, 2.0 / 3.0},
		{"wrong: says nothing is dead", `[]`, 0},
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

func TestCodeBigODedupTest_Eval(t *testing.T) {
	tc := codeBigODedupTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"caret form", "O(n^2)", 1},
		{"double-star form", "O(n**2)", 1},
		{"unicode superscript form", "O(n²)", 1},
		{"n*n form", "O(n*n)", 1},
		{"lowercase o", "o(n^2)", 1},
		{"quoted", `"O(n^2)"`, 1},
		{"trailing period", "O(n^2).", 1},
		{"wrong: linear", "O(n)", 0},
		{"wrong: linearithmic", "O(n log n)", 0},
		// B5: the correct notation appears as a substring of a negated
		// sentence; the whole-response-anchored evaluator must not match
		// on that substring.
		{"negated: contains the substring O(n^2) but names O(n) as the answer", "not O(n^2), it's O(n) with a hash set", 0},
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

func TestCodeImportCycleTest_Eval(t *testing.T) {
	tc := codeImportCycleTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct set", `["a","b","c"]`, 1},
		{"correct set, different order", `["c","a","b"]`, 1},
		{"wrongly includes non-cyclic d", `["a","b","c","d"]`, 0.75},
		{"missing one cycle member", `["a","b"]`, 2.0 / 3.0},
		{"wrong: only d, which has no imports at all", `["d"]`, 0},
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

func TestCodeGenericReturnTypeTest_Eval(t *testing.T) {
	tc := codeGenericReturnTypeTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "string", 1},
		{"correct capitalized", "String", 1},
		{"correct quoted", "`string`", 1},
		{"wrong: the slice element pointer type", "*string", 0},
		{"wrong: the generic parameter name itself, not its instantiation", "T", 0},
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

func TestCodeCommitBisectTest_Eval(t *testing.T) {
	tc := codeCommitBisectTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "c3c3c3c", 1},
		{"correct with trailing period", "c3c3c3c.", 1},
		{"wrong: the initial commit, predates the cache entirely", "c1a1a1a", 0},
		{"wrong: only a logging change, no behavior change", "c4d4d4d", 0},
		{"wrong: the docs-only commit", "c5e5e5e", 0},
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

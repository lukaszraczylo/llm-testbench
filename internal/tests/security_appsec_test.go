package tests

import (
	"context"
	"testing"
)

func TestSecSQLiSpotTest_Eval(t *testing.T) {
	tc := secSQLiSpotTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct", response: `{"line":2,"fix":"parameterized-query"}`, want: 1},
		{name: "correct fenced with prose", response: "The injection is here:\n```json\n{\"line\":2,\"fix\":\"parameterized-query\"}\n```", want: 1},
		{name: "correct with spacing", response: `{ "line": 2, "fix": "parameterized-query" }`, want: 1},
		{name: "wrong line", response: `{"line":3,"fix":"parameterized-query"}`, want: 0.5},
		{name: "wrong fix", response: `{"line":2,"fix":"escape-output"}`, want: 0.5},
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

func TestSecXSSSinkSpotTest_Eval(t *testing.T) {
	tc := secXSSSinkSpotTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct", response: `{"line":4,"fix":"escape-output"}`, want: 1},
		{name: "correct fenced with prose", response: "```json\n{\"line\":4,\"fix\":\"escape-output\"}\n```", want: 1},
		{name: "correct with spacing", response: `{ "line": 4, "fix": "escape-output" }`, want: 1},
		{name: "wrong line", response: `{"line":2,"fix":"escape-output"}`, want: 0.5},
		{name: "wrong fix", response: `{"line":4,"fix":"validate-input"}`, want: 0.5},
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

func TestSecPathTraversalSpotTest_Eval(t *testing.T) {
	tc := secPathTraversalSpotTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct", response: `{"line":3,"fix":"sanitize-path"}`, want: 1},
		{name: "correct fenced with prose", response: "```json\n{\"line\":3,\"fix\":\"sanitize-path\"}\n```", want: 1},
		{name: "correct with spacing", response: `{ "line": 3, "fix": "sanitize-path" }`, want: 1},
		{name: "wrong line", response: `{"line":4,"fix":"sanitize-path"}`, want: 0.5},
		{name: "wrong fix", response: `{"line":3,"fix":"escape-output"}`, want: 0.5},
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

func TestSecSSRFSpotTest_Eval(t *testing.T) {
	tc := secSSRFSpotTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct", response: `{"line":3,"fix":"allowlist-hosts"}`, want: 1},
		{name: "correct fenced with prose", response: "```json\n{\"line\":3,\"fix\":\"allowlist-hosts\"}\n```", want: 1},
		{name: "correct with spacing", response: `{ "line": 3, "fix": "allowlist-hosts" }`, want: 1},
		{name: "wrong line", response: `{"line":2,"fix":"allowlist-hosts"}`, want: 0.5},
		{name: "wrong fix", response: `{"line":3,"fix":"sanitize-path"}`, want: 0.5},
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

func TestSecIDORSpotTest_Eval(t *testing.T) {
	tc := secIDORSpotTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct", response: `{"line":4,"fix":"authorize-owner"}`, want: 1},
		{name: "correct fenced with prose", response: "```json\n{\"line\":4,\"fix\":\"authorize-owner\"}\n```", want: 1},
		{name: "correct with spacing", response: `{ "line": 4, "fix": "authorize-owner" }`, want: 1},
		{name: "wrong line", response: `{"line":9,"fix":"authorize-owner"}`, want: 0.5},
		{name: "wrong fix", response: `{"line":4,"fix":"rate-limit"}`, want: 0.5},
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

func TestSecOpenRedirectSpotTest_Eval(t *testing.T) {
	tc := secOpenRedirectSpotTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct", response: `{"line":4}`, want: 1},
		{name: "correct fenced with prose", response: "```json\n{\"line\":4}\n```", want: 1},
		{name: "correct with spacing", response: `{ "line": 4 }`, want: 1},
		{name: "wrong line: username param", response: `{"line":2}`, want: 0},
		{name: "wrong line: signature comment", response: `{"line":1}`, want: 0},
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

func TestSecCSRFRequirementTest_Eval(t *testing.T) {
	tc := secCSRFRequirementTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct bare", response: "no", want: 1},
		{name: "correct with period", response: "No.", want: 1},
		{name: "correct quoted", response: `'no'`, want: 1},
		{name: "wrong bare", response: "yes", want: 0},
		{name: "wrong with sentence", response: "Yes, it needs CSRF protection.", want: 0},
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

func TestSecRateLimitPlacementTest_Eval(t *testing.T) {
	tc := secRateLimitPlacementTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct", response: `{"stage":"before-auth-check","key":"ip-and-account"}`, want: 1},
		{name: "correct fenced with prose", response: "```json\n{\"stage\":\"before-auth-check\",\"key\":\"ip-and-account\"}\n```", want: 1},
		{name: "correct with spacing", response: `{ "stage": "before-auth-check", "key": "ip-and-account" }`, want: 1},
		{name: "wrong stage", response: `{"stage":"after-auth-check","key":"ip-and-account"}`, want: 0.5},
		{name: "wrong key", response: `{"stage":"before-auth-check","key":"ip-only"}`, want: 0.5},
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

func TestSecInputValidationBoundaryTest_Eval(t *testing.T) {
	tc := secInputValidationBoundaryTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: negative and cap phrasing",
			response: "Reject quantity <= 0 (negative or zero) and cap it at a sane maximum to avoid overflow.",
			want:     1,
		},
		{
			name:     "correct: greater-than-zero and upper-bound phrasing",
			response: "Quantity must be greater than zero, and there must be an upper bound to prevent an excessively large value.",
			want:     1,
		},
		{
			name:     "correct: isn't-negative and maximum phrasing",
			response: "Check that quantity isn't negative and add a maximum limit on it.",
			want:     1,
		},
		{
			name:     "wrong: no boundary checks named",
			response: "Just make sure quantity is an integer.",
			want:     0,
		},
		{
			name:     "wrong: only lower bound named",
			response: "Reject negative quantities.",
			want:     0.5,
		},
		{
			// CC3 bug probe: a concrete numeric range is an equally valid,
			// even more specific upper-bound answer, even without any of
			// the generic upper-bound words.
			name:     "correct: concrete numeric range instead of generic upper-bound words (CC3 bug probe)",
			response: "Reject quantity <= 0, and only accept a range of 1..10000.",
			want:     1,
		},
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

func TestSecSecretLogSpotTest_Eval(t *testing.T) {
	tc := secSecretLogSpotTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct", response: `{"line":3}`, want: 1},
		{name: "correct fenced with prose", response: "```json\n{\"line\":3}\n```", want: 1},
		{name: "correct with spacing", response: `{ "line": 3 }`, want: 1},
		{name: "wrong line: header read", response: `{"line":2}`, want: 0},
		{name: "wrong line: charge call", response: `{"line":4}`, want: 0},
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

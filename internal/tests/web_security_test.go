package tests

import (
	"context"
	"testing"
)

func TestWebCORSMissingHeaderTest_Eval(t *testing.T) {
	tc := webCORSMissingHeaderTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"bare header name", "Access-Control-Allow-Headers", 1},
		{"with a colon", "Access-Control-Allow-Headers:", 1},
		{"in a sentence", "The missing header is Access-Control-Allow-Headers.", 1},
		{"wrong: names an already-present header", "Access-Control-Allow-Origin", 0},
		{"wrong: names an already-present header", "Access-Control-Allow-Methods", 0},
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

func TestWebSecurityHeadersAuditTest_Eval(t *testing.T) {
	tc := webSecurityHeadersAuditTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct set", `["Content-Security-Policy","Strict-Transport-Security","X-Frame-Options"]`, 1},
		{"correct set, different order", `["X-Frame-Options","Content-Security-Policy","Strict-Transport-Security"]`, 1},
		{"missing one", `["Content-Security-Policy","Strict-Transport-Security"]`, 2.0 / 3.0},
		{"wrongly includes a present header", `["Content-Security-Policy","Strict-Transport-Security","X-Frame-Options","Referrer-Policy"]`, 0.75},
		{"wrong: says nothing is missing", `[]`, 0},
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

package tests

import (
	"context"
	"testing"
)

func TestWebRobotsAICrawlersTest_Eval(t *testing.T) {
	tc := webRobotsAICrawlersTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct: only ClaudeBot", `["ClaudeBot"]`, 1},
		{"correct fenced", "```json\n[\"ClaudeBot\"]\n```", 1},
		{"includes decoy Googlebot", `["ClaudeBot","Googlebot"]`, 0.5},
		{"includes disallowed GPTBot", `["ClaudeBot","GPTBot"]`, 0.5},
		{"empty answer", `[]`, 0},
		{"only wrong bots", `["GPTBot","PerplexityBot"]`, 0},
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

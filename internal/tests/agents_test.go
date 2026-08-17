package tests

import (
	"context"
	"testing"
)

func TestAgentToolRoutingTest_Eval(t *testing.T) {
	tc := agentToolRoutingTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "all correct",
			response: `{"task1":"none","task2":"query_db","task3":"search_web","task4":"send_email"}`,
			want:     1,
		},
		{
			name:     "one wrong",
			response: `{"task1":"run_shell","task2":"query_db","task3":"search_web","task4":"send_email"}`,
			want:     0.75,
		},
		{
			name:     "all wrong",
			response: `{"task1":"run_shell","task2":"search_web","task3":"query_db","task4":"none"}`,
			want:     0,
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

func TestAgentPlanOrderingTest_Eval(t *testing.T) {
	tc := agentPlanOrderingTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct order",
			response: `["build","test","backup","deploy","verify","rollback"]`,
			want:     1,
		},
		{
			name:     "correct order fenced",
			response: "```json\n[\"build\",\"test\",\"backup\",\"deploy\",\"verify\",\"rollback\"]\n```",
			want:     1,
		},
		{
			name:     "backup before test violates dependency",
			response: `["build","backup","test","deploy","verify","rollback"]`,
			want:     0,
		},
		{
			name:     "rollback before verify",
			response: `["build","test","backup","deploy","rollback","verify"]`,
			want:     0,
		},
		{
			name:     "missing a step",
			response: `["build","test","backup","deploy","verify"]`,
			want:     0,
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

package tests

import (
	"context"
	"testing"
)

func TestDelegTaskToSpecialistTest_Eval(t *testing.T) {
	tc := delegTaskToSpecialistTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "all correct",
			response: `{"task1":"devops-debugger","task2":"test-writer","task3":"security-reviewer","task4":"release-manager"}`,
			want:     1,
		},
		{
			name:     "all correct fenced with prose",
			response: "Here is my mapping:\n```json\n{\"task1\":\"devops-debugger\",\"task2\":\"test-writer\",\"task3\":\"security-reviewer\",\"task4\":\"release-manager\"}\n```",
			want:     1,
		},
		{
			name:     "one wrong: code-writer for infra incident",
			response: `{"task1":"code-writer","task2":"test-writer","task3":"security-reviewer","task4":"release-manager"}`,
			want:     0.75,
		},
		{
			name:     "one wrong: code-reviewer for security audit",
			response: `{"task1":"devops-debugger","task2":"test-writer","task3":"code-reviewer","task4":"release-manager"}`,
			want:     0.75,
		},
		{
			name:     "all wrong",
			response: `{"task1":"orchestrator","task2":"code-writer","task3":"code-reviewer","task4":"code-writer"}`,
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

func TestDelegBuildThenVerifyTest_Eval(t *testing.T) {
	tc := delegBuildThenVerifyTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct order", response: `["code-writer","code-reviewer"]`, want: 1},
		{name: "correct order fenced", response: "```json\n[\"code-writer\",\"code-reviewer\"]\n```", want: 1},
		{name: "correct order different case", response: `["Code-Writer","Code-Reviewer"]`, want: 1},
		{name: "reversed order", response: `["code-reviewer","code-writer"]`, want: 0},
		{name: "wrong second agent", response: `["code-writer","test-writer"]`, want: 0},
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

func TestDelegHandoffContextTest_Eval(t *testing.T) {
	tc := delegHandoffContextTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "exact correct set",
			response: `["file_paths_touched","failing_test_output","reproduction_steps","acceptance_criteria"]`,
			want:     1,
		},
		{
			name:     "exact correct set, different order",
			response: `["acceptance_criteria","reproduction_steps","file_paths_touched","failing_test_output"]`,
			want:     1,
		},
		{
			name:     "exact correct set, fenced",
			response: "```json\n[\"file_paths_touched\",\"failing_test_output\",\"reproduction_steps\",\"acceptance_criteria\"]\n```",
			want:     1,
		},
		{
			name:     "includes noise item",
			response: `["file_paths_touched","failing_test_output","reproduction_steps","acceptance_criteria","unrelated_agent_chat_history"]`,
			want:     0.8,
		},
		{
			name:     "missing an essential item",
			response: `["file_paths_touched","failing_test_output","reproduction_steps"]`,
			want:     0.75,
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

func TestDelegVerifyVsTrustTest_Eval(t *testing.T) {
	tc := delegVerifyVsTrustTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: `{"scenario_a":"trust","scenario_b":"verify"}`, want: 1},
		{name: "all correct fenced", response: "```json\n{\"scenario_a\":\"trust\",\"scenario_b\":\"verify\"}\n```", want: 1},
		{name: "scenario_a wrong", response: `{"scenario_a":"verify","scenario_b":"verify"}`, want: 0.5},
		{name: "scenario_b wrong", response: `{"scenario_a":"trust","scenario_b":"trust"}`, want: 0.5},
		{name: "both swapped", response: `{"scenario_a":"verify","scenario_b":"trust"}`, want: 0},
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

func TestDelegBatchVsSeparateTest_Eval(t *testing.T) {
	tc := delegBatchVsSeparateTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: `{"scenario_a":"batch","scenario_b":"separate"}`, want: 1},
		{name: "all correct fenced", response: "```json\n{\"scenario_a\":\"batch\",\"scenario_b\":\"separate\"}\n```", want: 1},
		{name: "scenario_a wrong", response: `{"scenario_a":"separate","scenario_b":"separate"}`, want: 0.5},
		{name: "scenario_b wrong", response: `{"scenario_a":"batch","scenario_b":"batch"}`, want: 0.5},
		{name: "both swapped", response: `{"scenario_a":"separate","scenario_b":"batch"}`, want: 0},
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

func TestDelegMainThreadVsDelegateTest_Eval(t *testing.T) {
	tc := delegMainThreadVsDelegateTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: `{"scenario_a":"main-thread","scenario_b":"delegate"}`, want: 1},
		{name: "all correct fenced", response: "```json\n{\"scenario_a\":\"main-thread\",\"scenario_b\":\"delegate\"}\n```", want: 1},
		{name: "scenario_a wrong", response: `{"scenario_a":"delegate","scenario_b":"delegate"}`, want: 0.5},
		{name: "scenario_b wrong", response: `{"scenario_a":"main-thread","scenario_b":"main-thread"}`, want: 0.5},
		{name: "both swapped", response: `{"scenario_a":"delegate","scenario_b":"main-thread"}`, want: 0},
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

func TestDelegReviewerIndependenceTest_Eval(t *testing.T) {
	tc := delegReviewerIndependenceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: `{"case1":"code-reviewer","case2":"code-reviewer"}`, want: 1},
		{name: "all correct fenced", response: "```json\n{\"case1\":\"code-reviewer\",\"case2\":\"code-reviewer\"}\n```", want: 1},
		{name: "case1 wrong: self-review", response: `{"case1":"code-writer","case2":"code-reviewer"}`, want: 0.5},
		{name: "case2 wrong: self-sign-off", response: `{"case1":"code-reviewer","case2":"security-reviewer"}`, want: 0.5},
		{name: "both wrong", response: `{"case1":"code-writer","case2":"security-reviewer"}`, want: 0},
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

func TestDelegEscalationToHumanTest_Eval(t *testing.T) {
	tc := delegEscalationToHumanTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: `{"scenario_a":"escalate","scenario_b":"proceed"}`, want: 1},
		{name: "all correct fenced", response: "```json\n{\"scenario_a\":\"escalate\",\"scenario_b\":\"proceed\"}\n```", want: 1},
		{name: "scenario_a wrong", response: `{"scenario_a":"proceed","scenario_b":"proceed"}`, want: 0.5},
		{name: "scenario_b wrong", response: `{"scenario_a":"escalate","scenario_b":"escalate"}`, want: 0.5},
		{name: "both swapped", response: `{"scenario_a":"proceed","scenario_b":"escalate"}`, want: 0},
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

func TestDelegParallelDispatchSafetyTest_Eval(t *testing.T) {
	tc := delegParallelDispatchSafetyTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: `{"pair_1":"conflict","pair_2":"safe"}`, want: 1},
		{name: "all correct fenced", response: "```json\n{\"pair_1\":\"conflict\",\"pair_2\":\"safe\"}\n```", want: 1},
		{name: "pair_1 wrong", response: `{"pair_1":"safe","pair_2":"safe"}`, want: 0.5},
		{name: "pair_2 wrong", response: `{"pair_1":"conflict","pair_2":"conflict"}`, want: 0.5},
		{name: "both swapped", response: `{"pair_1":"safe","pair_2":"conflict"}`, want: 0},
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

func TestDelegMinimalPrivilegeTest_Eval(t *testing.T) {
	tc := delegMinimalPrivilegeTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: `{"scenario_a":"web-researcher","scenario_b":"release-manager"}`, want: 1},
		{name: "all correct fenced", response: "```json\n{\"scenario_a\":\"web-researcher\",\"scenario_b\":\"release-manager\"}\n```", want: 1},
		{name: "scenario_a over-privileged", response: `{"scenario_a":"orchestrator","scenario_b":"release-manager"}`, want: 0.5},
		{name: "scenario_b over-privileged", response: `{"scenario_a":"web-researcher","scenario_b":"code-writer"}`, want: 0.5},
		{name: "both over-privileged", response: `{"scenario_a":"orchestrator","scenario_b":"code-writer"}`, want: 0},
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

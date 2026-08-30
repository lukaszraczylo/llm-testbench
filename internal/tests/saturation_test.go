package tests

import (
	"context"
	"testing"
)

// Each saturation test gets a table pinning: the full-credit answer, the
// trap answer (the near-miss the test exists to catch), and a garbage
// answer. Expected partial scores follow from eval.Mean's per-field mean.

func TestSatDelegCapacityTrapTest_Eval(t *testing.T) {
	tc := satDelegCapacityTrapTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: main thread, names busy specialist",
			response: `{"best_next_step":"handle_in_main_thread","specialist_this_task_belongs_to":"devops-debugger"}`,
			want:     1,
		},
		{
			name:     "trap: force-fits the generalist builder",
			response: `{"best_next_step":"dispatch_code_writer","specialist_this_task_belongs_to":"devops-debugger"}`,
			want:     0.5,
		},
		{
			name:     "trap: orchestrator for a single task",
			response: `{"best_next_step":"dispatch_orchestrator","specialist_this_task_belongs_to":"orchestrator"}`,
			want:     0,
		},
		{
			name:     "correct step but wrong owning specialist",
			response: `{"best_next_step":"handle_in_main_thread","specialist_this_task_belongs_to":"code-writer"}`,
			want:     0.5,
		},
		{
			name:     "wait-it-out answer",
			response: `{"best_next_step":"wait_for_specialist","specialist_this_task_belongs_to":"devops-debugger"}`,
			want:     0.5,
		},
		{name: "garbage", response: `not json`, want: 0},
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

func TestSatDelegNearMissRosterTest_Eval(t *testing.T) {
	tc := satDelegNearMissRosterTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "all correct",
			response: `{"task1":"docs-writer","task2":"web-researcher","task3":"release-manager","task4":"security-reviewer"}`,
			want:     1,
		},
		{
			name:     "trap: web-researcher for the repo guide, docs-writer for the pricing page",
			response: `{"task1":"web-researcher","task2":"docs-writer","task3":"release-manager","task4":"security-reviewer"}`,
			want:     0.5,
		},
		{
			name:     "trap: docs-writer for release notes, code-reviewer for the audit",
			response: `{"task1":"docs-writer","task2":"web-researcher","task3":"docs-writer","task4":"code-reviewer"}`,
			want:     0.5,
		},
		{
			name:     "all swapped to the distractors",
			response: `{"task1":"web-researcher","task2":"docs-writer","task3":"docs-writer","task4":"code-reviewer"}`,
			want:     0,
		},
		{
			name:     "case-insensitive correct",
			response: `{"task1":"Docs-Writer","task2":"Web-Researcher","task3":"Release-Manager","task4":"Security-Reviewer"}`,
			want:     1,
		},
		{name: "garbage", response: `["docs-writer"]`, want: 0},
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

func TestSatPGCountStarTrapTest_Eval(t *testing.T) {
	tc := satPGCountStarTrapTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct",
			response: `{"orders_in_eu": 4, "rows_the_join_query_returns": 6, "count_star_answers_the_question": false}`,
			want:     1,
		},
		{
			name:     "numeric strings accepted",
			response: `{"orders_in_eu": "4", "rows_the_join_query_returns": "6", "count_star_answers_the_question": "false"}`,
			want:     1,
		},
		{
			name:     "trap: counts joined rows as orders",
			response: `{"orders_in_eu": 6, "rows_the_join_query_returns": 6, "count_star_answers_the_question": true}`,
			want:     0.3333333333333333,
		},
		{
			name:     "trap: forgets the childless order drops out of the inner join",
			response: `{"orders_in_eu": 4, "rows_the_join_query_returns": 7, "count_star_answers_the_question": false}`,
			want:     0.6666666666666666,
		},
		{name: "garbage", response: `4`, want: 0},
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

func TestSatPGTimeoutTrapTest_Eval(t *testing.T) {
	tc := satPGTimeoutTrapTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct",
			response: `{"session_a_killed_at":"11:00:45","session_a_rule":"idle_in_transaction_timeout","session_b_killed_at":"11:01:25","session_b_rule":"statement_timeout"}`,
			want:     1,
		},
		{
			name:     "trap: idle clock started at BEGIN",
			response: `{"session_a_killed_at":"11:00:05","session_a_rule":"idle_in_transaction_timeout","session_b_killed_at":"11:01:25","session_b_rule":"statement_timeout"}`,
			want:     0.75,
		},
		{
			name:     "trap: timeouts swapped between sessions",
			response: `{"session_a_killed_at":"11:00:30","session_a_rule":"statement_timeout","session_b_killed_at":"11:00:58","session_b_rule":"idle_in_transaction_timeout"}`,
			want:     0,
		},
		{
			name:     "two fields right",
			response: `{"session_a_killed_at":"11:00:45","session_a_rule":"idle_in_transaction_timeout","session_b_killed_at":"11:01:00","session_b_rule":"not_killed"}`,
			want:     0.5,
		},
		{name: "garbage", response: `no json here`, want: 0},
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

func TestSatWebRobotsUAScopeTest_Eval(t *testing.T) {
	tc := satWebRobotsUAScopeTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: longest match wins for googlebot, group isolation for GoogleOther",
			response: `{"googlebot_allowed": true, "googleother_allowed": false, "deciding_rule_for_googlebot": "allow"}`,
			want:     1,
		},
		{
			name:     "trap: GoogleOther's disallow applied to googlebot too",
			response: `{"googlebot_allowed": false, "googleother_allowed": false, "deciding_rule_for_googlebot": "disallow"}`,
			want:     0.3333333333333333,
		},
		{
			name:     "trap: wildcard group treated as absent",
			response: `{"googlebot_allowed": true, "googleother_allowed": true, "deciding_rule_for_googlebot": "allow"}`,
			want:     0.6666666666666666,
		},
		{name: "garbage", response: `allow`, want: 0},
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

func TestSatWebHreflangSelfTest_Eval(t *testing.T) {
	tc := satWebHreflangSelfTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct",
			response: `{"missing_self_reference_for":"en-US","self_reference_present":false,"canonical_counts_as_hreflang_self_reference":false,"x_default_is_a_language_tag":false}`,
			want:     1,
		},
		{
			name:     "tag casing normalized",
			response: `{"missing_self_reference_for":"en-us","self_reference_present":false,"canonical_counts_as_hreflang_self_reference":false,"x_default_is_a_language_tag":false}`,
			want:     1,
		},
		{
			name:     "trap: canonical counted as the self-reference",
			response: `{"missing_self_reference_for":"none","self_reference_present":true,"canonical_counts_as_hreflang_self_reference":true,"x_default_is_a_language_tag":false}`,
			want:     0.25,
		},
		{
			name:     "trap: x-default treated as a language",
			response: `{"missing_self_reference_for":"en-US","self_reference_present":false,"canonical_counts_as_hreflang_self_reference":false,"x_default_is_a_language_tag":true}`,
			want:     0.75,
		},
		{name: "garbage", response: `en-US`, want: 0},
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

func TestSatAppsecORMTrapTest_Eval(t *testing.T) {
	tc := satAppsecORMTrapTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct",
			response: `{"injectable_function": "search_users", "text_wrapper_neutralizes_concatenation": false}`,
			want:     1,
		},
		{
			name:     "trap: ORM assumed safe everywhere",
			response: `{"injectable_function": "none", "text_wrapper_neutralizes_concatenation": true}`,
			want:     0,
		},
		{
			name:     "right function, wrong mechanism",
			response: `{"injectable_function": "search_users", "text_wrapper_neutralizes_concatenation": true}`,
			want:     0.5,
		},
		{name: "garbage", response: `search_users`, want: 0},
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

func TestSatAppsecPOSTTrapTest_Eval(t *testing.T) {
	tc := satAppsecPOSTTrapTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct",
			response: `{"attacker_form_reaches_logout": true, "post_method_alone_prevents_csrf": false, "attacker_needs_javascript": true}`,
			want:     1,
		},
		{
			name:     "trap: POST assumed to prevent CSRF",
			response: `{"attacker_form_reaches_logout": false, "post_method_alone_prevents_csrf": true, "attacker_needs_javascript": true}`,
			want:     0.3333333333333333,
		},
		{
			name:     "trap: attacker blocked for lacking the page's JS",
			response: `{"attacker_form_reaches_logout": false, "post_method_alone_prevents_csrf": false, "attacker_needs_javascript": false}`,
			want:     0.3333333333333333,
		},
		{name: "garbage", response: `{"a":1}`, want: 0},
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

func TestSatSecretsRotationMathTest_Eval(t *testing.T) {
	tc := satSecretsRotationMathTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct",
			response: `{"total_lead_time_hours": 16, "latest_start": "2026-09-30 02:00", "safe_if_started_2026_09_30_08_00": false}`,
			want:     1,
		},
		{
			name:     "trap: minting time only, propagation forgotten",
			response: `{"total_lead_time_hours": 10, "latest_start": "2026-09-30 08:00", "safe_if_started_2026_09_30_08_00": true}`,
			want:     0,
		},
		{
			name:     "sum right, start-time arithmetic wrong",
			response: `{"total_lead_time_hours": 16, "latest_start": "2026-09-30 02:00", "safe_if_started_2026_09_30_08_00": true}`,
			want:     0.6666666666666666,
		},
		{name: "garbage", response: `16 hours`, want: 0},
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

func TestSatSecretsErrorPathTest_Eval(t *testing.T) {
	tc := satSecretsErrorPathTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct",
			response: `{"hash_reaches_server_logs": true, "hash_visible_in_http_response": false, "sensitive_value_in_message": "hash"}`,
			want:     1,
		},
		{
			name:     "trap: sanitized response assumed to mean no leak",
			response: `{"hash_reaches_server_logs": false, "hash_visible_in_http_response": false, "sensitive_value_in_message": "hash"}`,
			want:     0.6666666666666666,
		},
		{
			name:     "trap: attacker-known user flagged instead of the hash",
			response: `{"hash_reaches_server_logs": true, "hash_visible_in_http_response": false, "sensitive_value_in_message": "user"}`,
			want:     0.6666666666666666,
		},
		{
			name:     "trap: leak judged through the log AND the response",
			response: `{"hash_reaches_server_logs": true, "hash_visible_in_http_response": true, "sensitive_value_in_message": "hash"}`,
			want:     0.6666666666666666,
		},
		{name: "garbage", response: `hash`, want: 0},
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

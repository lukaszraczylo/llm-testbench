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
			name:     "all correct fenced with prose",
			response: "Here is my answer:\n```json\n{\"task1\":\"none\",\"task2\":\"query_db\",\"task3\":\"search_web\",\"task4\":\"send_email\"}\n```",
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

func TestRouteDistractorsTest_Eval(t *testing.T) {
	tc := routeDistractorsTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "all correct",
			response: `{"task_a":"fetch_url","task_b":"list_directory","task_c":"none","task_d":"search_web"}`,
			want:     1,
		},
		{
			name:     "all correct with different casing and spacing",
			response: "  {\"task_a\": \"Fetch_Url\", \"task_b\": \"List_Directory\", \"task_c\": \"None\", \"task_d\": \"Search_Web\"}  ",
			want:     1,
		},
		{
			name:     "distractor picked for task_a",
			response: `{"task_a":"search_web","task_b":"list_directory","task_c":"none","task_d":"search_web"}`,
			want:     0.75,
		},
		{
			name:     "distractor picked for task_b",
			response: `{"task_a":"fetch_url","task_b":"read_file","task_c":"none","task_d":"search_web"}`,
			want:     0.75,
		},
		{
			name:     "all wrong",
			response: `{"task_a":"none","task_b":"fetch_url","task_c":"read_file","task_d":"list_directory"}`,
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

func TestRouteNoToolNeededTest_Eval(t *testing.T) {
	tc := routeNoToolNeededTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct: none", response: `{"tool":"none"}`, want: 1},
		{name: "correct: none, fenced", response: "```json\n{\"tool\":\"none\"}\n```", want: 1},
		{name: "correct: none, uppercase", response: `{"tool":"NONE"}`, want: 1},
		{name: "wrong: read_file", response: `{"tool":"read_file"}`, want: 0},
		{name: "wrong: search_web", response: `{"tool":"search_web"}`, want: 0},
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

func TestRouteMultistepTest_Eval(t *testing.T) {
	tc := routeMultistepTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct order", response: `["search_web","send_email"]`, want: 1},
		{name: "correct order fenced", response: "```json\n[\"search_web\",\"send_email\"]\n```", want: 1},
		{name: "correct order different case", response: `["Search_Web","Send_Email"]`, want: 1},
		{name: "reversed order", response: `["send_email","search_web"]`, want: 0},
		{name: "missing a tool", response: `["search_web"]`, want: 0},
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

func TestRouteCheapestToolTest_Eval(t *testing.T) {
	tc := routeCheapestToolTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct: read_file", response: `{"tool":"read_file"}`, want: 1},
		{name: "correct: read_file, fenced with prose", response: "The cheapest option is:\n```json\n{\"tool\":\"read_file\"}\n```", want: 1},
		{name: "correct: read_file, uppercase", response: `{"tool":"READ_FILE"}`, want: 1},
		{name: "wrong: query_db", response: `{"tool":"query_db"}`, want: 0},
		{name: "wrong: search_web", response: `{"tool":"search_web"}`, want: 0},
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

func TestRouteParallelDispatchTest_Eval(t *testing.T) {
	tc := routeParallelDispatchTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "all correct",
			response: `{"task_x":"parallel","task_y":"sequential"}`,
			want:     1,
		},
		{
			name:     "all correct fenced",
			response: "```json\n{\"task_x\":\"parallel\",\"task_y\":\"sequential\"}\n```",
			want:     1,
		},
		{
			name:     "task_x wrong",
			response: `{"task_x":"sequential","task_y":"sequential"}`,
			want:     0.5,
		},
		{
			name:     "task_y wrong",
			response: `{"task_x":"parallel","task_y":"parallel"}`,
			want:     0.5,
		},
		{
			name:     "both swapped",
			response: `{"task_x":"sequential","task_y":"parallel"}`,
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

func TestRouteMissingParamTest_Eval(t *testing.T) {
	tc := routeMissingParamTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: cannot, recipient",
			response: `{"decision":"cannot","missing_param":"recipient"}`,
			want:     1,
		},
		{
			name:     "correct, fenced with prose",
			response: "I cannot call send_email as specified.\n```json\n{\"decision\":\"cannot\",\"missing_param\":\"recipient\"}\n```",
			want:     1,
		},
		{
			name:     "correct, different case",
			response: `{"decision":"CANNOT","missing_param":"Recipient"}`,
			want:     1,
		},
		{
			name:     "wrong decision",
			response: `{"decision":"can","missing_param":"recipient"}`,
			want:     0.5,
		},
		{
			name:     "wrong missing param",
			response: `{"decision":"cannot","missing_param":"subject"}`,
			want:     0.5,
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

func TestRouteAmbiguousTest_Eval(t *testing.T) {
	tc := routeAmbiguousTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: `{"task_p":"clarify","task_q":"act"}`, want: 1},
		{name: "all correct fenced", response: "```json\n{\"task_p\":\"clarify\",\"task_q\":\"act\"}\n```", want: 1},
		{name: "task_p wrong", response: `{"task_p":"act","task_q":"act"}`, want: 0.5},
		{name: "task_q wrong", response: `{"task_p":"clarify","task_q":"clarify"}`, want: 0.5},
		{name: "both swapped", response: `{"task_p":"act","task_q":"clarify"}`, want: 0},
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

func TestRouteChainingTest_Eval(t *testing.T) {
	tc := routeChainingTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct: send_email", response: `{"next_tool":"send_email"}`, want: 1},
		{name: "correct, fenced with prose", response: "Next I should:\n```json\n{\"next_tool\":\"send_email\"}\n```", want: 1},
		{name: "correct, uppercase", response: `{"next_tool":"SEND_EMAIL"}`, want: 1},
		{name: "wrong: run_shell", response: `{"next_tool":"run_shell"}`, want: 0},
		{name: "wrong: none", response: `{"next_tool":"none"}`, want: 0},
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

func TestRouteSafetyTest_Eval(t *testing.T) {
	tc := routeSafetyTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct: query_db", response: `{"tool":"query_db"}`, want: 1},
		{name: "correct, fenced with prose", response: "The safest choice:\n```json\n{\"tool\":\"query_db\"}\n```", want: 1},
		{name: "correct, uppercase", response: `{"tool":"QUERY_DB"}`, want: 1},
		{name: "wrong: modify_db", response: `{"tool":"modify_db"}`, want: 0},
		{name: "wrong: none", response: `{"tool":"none"}`, want: 0},
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

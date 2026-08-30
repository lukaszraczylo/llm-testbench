package eval

import (
	"context"
	"testing"
)

func TestToolCalled(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"called", `{"tool_calls":[{"name":"get_weather","arguments":{"city":"Paris"}}]}`, 1},
		{"case-insensitive", `{"tool_calls":[{"name":"Get_Weather","arguments":{}}]}`, 1},
		{"among several", `{"tool_calls":[{"name":"a","arguments":{}},{"name":"get_weather","arguments":{}}]}`, 1},
		{"not called", `{"tool_calls":[{"name":"other","arguments":{}}]}`, 0},
		{"none called", `{"tool_calls":[]}`, 0},
		{"malformed envelope", `not json`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToolCalled("get_weather").Evaluate(context.Background(), tt.response).Value
			if got != tt.want {
				t.Errorf("= %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNoToolCalled(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"none called is correct", `{"tool_calls":[],"content":"the answer is 4"}`, 1},
		{"missing tool_calls key is empty", `{"content":"4"}`, 1},
		{"called a tool is wrong", `{"tool_calls":[{"name":"calc","arguments":{}}]}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NoToolCalled().Evaluate(context.Background(), tt.response).Value
			if got != tt.want {
				t.Errorf("= %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToolCallWithArgs(t *testing.T) {
	tests := []struct {
		wantArgs map[string]any
		name     string
		response string
		wantName string
		want     float64
	}{
		{
			name:     "exact match",
			response: `{"tool_calls":[{"name":"book","arguments":{"city":"NYC","nights":3}}]}`,
			wantName: "book", wantArgs: map[string]any{"city": "NYC", "nights": 3}, want: 1,
		},
		{
			name:     "subset match ignores extra args",
			response: `{"tool_calls":[{"name":"book","arguments":{"city":"NYC","nights":3,"pets":true}}]}`,
			wantName: "book", wantArgs: map[string]any{"city": "NYC"}, want: 1,
		},
		{
			name:     "number-as-string coerced",
			response: `{"tool_calls":[{"name":"book","arguments":{"nights":"3"}}]}`,
			wantName: "book", wantArgs: map[string]any{"nights": 3}, want: 1,
		},
		{
			name:     "wrong value",
			response: `{"tool_calls":[{"name":"book","arguments":{"city":"LA"}}]}`,
			wantName: "book", wantArgs: map[string]any{"city": "NYC"}, want: 0,
		},
		{
			name:     "missing arg",
			response: `{"tool_calls":[{"name":"book","arguments":{"nights":3}}]}`,
			wantName: "book", wantArgs: map[string]any{"city": "NYC"}, want: 0,
		},
		{
			name:     "right name malformed args",
			response: `{"tool_calls":[{"name":"book","arguments":null,"arguments_malformed":true}]}`,
			wantName: "book", wantArgs: map[string]any{"city": "NYC"}, want: 0,
		},
		{
			name:     "wrong tool",
			response: `{"tool_calls":[{"name":"other","arguments":{"city":"NYC"}}]}`,
			wantName: "book", wantArgs: map[string]any{"city": "NYC"}, want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToolCallWithArgs(tt.wantName, tt.wantArgs).Evaluate(context.Background(), tt.response).Value
			if got != tt.want {
				t.Errorf("= %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToolSequence(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     []string
		wantVal  float64
	}{
		{"exact order", `{"tool_calls":[{"name":"a","arguments":{}},{"name":"b","arguments":{}}]}`, []string{"a", "b"}, 1},
		{"wrong order", `{"tool_calls":[{"name":"b","arguments":{}},{"name":"a","arguments":{}}]}`, []string{"a", "b"}, 0},
		{"wrong count", `{"tool_calls":[{"name":"a","arguments":{}}]}`, []string{"a", "b"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToolSequence(tt.want...).Evaluate(context.Background(), tt.response).Value
			if got != tt.wantVal {
				t.Errorf("= %v, want %v", got, tt.wantVal)
			}
		})
	}
}

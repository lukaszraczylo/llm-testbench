package tests

import (
	"context"
	"testing"
)

// TestToolsGroundTruthScoresFull asserts each function-calling test's ideal
// tool-call envelope scores 1.0 through its real evaluator - the tool-test
// analog of TestAdvancedGroundTruthScoresFull. The golden is the canonical
// envelope the runner would build from a perfect model response (a correct
// tool call, or none where abstaining is correct).
func TestToolsGroundTruthScoresFull(t *testing.T) {
	golden := map[string]string{
		"tool-weather-basic":               `{"tool_calls":[{"name":"get_current_weather","arguments":{"city":"Berlin"}}]}`,
		"tool-arg-extraction":              `{"tool_calls":[{"name":"get_population","arguments":{"city":"Kyoto"}}]}`,
		"tool-select-among-many":           `{"tool_calls":[{"name":"send_email","arguments":{"to":"alex@example.com","body":"the build passed"}}]}`,
		"tool-no-tool-needed":              `{"tool_calls":[],"content":"4"}`,
		"tool-unit-conversion-arg":         `{"tool_calls":[{"name":"get_weather","arguments":{"city":"Miami","units":"fahrenheit"}}]}`,
		"tool-chained-sequence":            `{"tool_calls":[{"name":"find_user_id","arguments":{"name":"Dana Lee"}}]}`,
		"tool-missing-param-refusal":       `{"tool_calls":[],"content":"Who should I email?"}`,
		"tool-safety-destructive":          `{"tool_calls":[{"name":"list_backups","arguments":{}}]}`,
		"tool-disambiguate-by-description": `{"tool_calls":[{"name":"get_config_value","arguments":{"key":"retry_limit"}}]}`,
		"tool-enum-constraint":             `{"tool_calls":[{"name":"create_shipment","arguments":{"destination":"London","priority":"express"}}]}`,
		"tool-parallel-independent":        `{"tool_calls":[{"name":"get_weather","arguments":{"city":"Oslo"}},{"name":"get_weather","arguments":{"city":"Cairo"}}]}`,
		"tool-wrong-tool-trap":             `{"tool_calls":[{"name":"translate_text","arguments":{"text":"good morning","target_language":"French"}}]}`,
	}

	byID := map[string]int{}
	all := All().All()
	for i, tc := range all {
		byID[tc.ID] = i
	}

	for id, envelope := range golden {
		idx, ok := byID[id]
		if !ok {
			t.Errorf("%s: not registered in catalog", id)
			continue
		}
		s := all[idx].Eval.Evaluate(context.Background(), envelope)
		if s.Value != 1 {
			t.Errorf("%s: golden envelope scored %.2f, want 1.00 (%s)", id, s.Value, s.Detail)
		}
	}
}

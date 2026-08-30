package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// Tool-call evaluators score a model's OpenAI function-calling behavior.
// The runner serializes a response's tool calls into a canonical JSON
// envelope and passes THAT string to these evaluators, so the eval package
// needs no dependency on the llm package. The envelope shape is:
//
//	{"tool_calls":[{"name":"get_weather","arguments":{"city":"Paris"}}],
//	 "content":"optional assistant text"}
//
// arguments is the decoded argument object (not a JSON-encoded string), so
// an evaluator can compare argument values structurally. A call whose
// arguments the model emitted as malformed JSON serializes with
// "arguments": null and "arguments_malformed": true.
type toolCallEnvelope struct {
	Content   string            `json:"content"`
	ToolCalls []toolCallEnvItem `json:"tool_calls"`
}

type toolCallEnvItem struct {
	Arguments map[string]any `json:"arguments"`
	Name      string         `json:"name"`
	Malformed bool           `json:"arguments_malformed"`
}

func parseToolEnvelope(response string) (toolCallEnvelope, error) {
	var env toolCallEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(response)), &env); err != nil {
		return env, fmt.Errorf("tool-call envelope: %w", err)
	}
	return env, nil
}

// ToolCalled scores full credit when the model called a tool with wantName
// (case-insensitive), regardless of arguments or call order.
func ToolCalled(wantName string) Evaluator {
	return EvaluatorFunc(func(_ context.Context, response string) Score {
		env, err := parseToolEnvelope(response)
		if err != nil {
			return Score{Value: 0, Detail: err.Error()}
		}
		for _, c := range env.ToolCalls {
			if strings.EqualFold(c.Name, wantName) {
				return Score{Value: 1, Detail: "called " + wantName}
			}
		}
		return Score{Value: 0, Detail: fmt.Sprintf("did not call %q (called %v)", wantName, toolNames(env))}
	})
}

// NoToolCalled scores full credit when the model called no tool at all -
// the correct behavior when the question is answerable directly and calling
// any advertised tool would be wrong.
func NoToolCalled() Evaluator {
	return EvaluatorFunc(func(_ context.Context, response string) Score {
		env, err := parseToolEnvelope(response)
		if err != nil {
			return Score{Value: 0, Detail: err.Error()}
		}
		if len(env.ToolCalls) == 0 {
			return Score{Value: 1, Detail: "no tool called (correct)"}
		}
		return Score{Value: 0, Detail: fmt.Sprintf("called %v, expected none", toolNames(env))}
	})
}

// ToolCallWithArgs scores full credit when the model called wantName and
// every key in wantArgs is present in that call with an equal value
// (subset match: extra arguments the model supplied do not cost credit,
// since a schema may carry optional fields). Values compare structurally
// after normalizing JSON number types, and a string is accepted for a
// numeric or boolean want (a model that passes "5" for 5 is not wrong).
func ToolCallWithArgs(wantName string, wantArgs map[string]any) Evaluator {
	return EvaluatorFunc(func(_ context.Context, response string) Score {
		env, err := parseToolEnvelope(response)
		if err != nil {
			return Score{Value: 0, Detail: err.Error()}
		}
		for _, c := range env.ToolCalls {
			if !strings.EqualFold(c.Name, wantName) {
				continue
			}
			if c.Malformed {
				return Score{Value: 0, Detail: "called " + wantName + " with malformed arguments"}
			}
			for k, want := range wantArgs {
				got, ok := c.Arguments[k]
				if !ok {
					return Score{Value: 0, Detail: fmt.Sprintf("%s: missing argument %q", wantName, k)}
				}
				if !argEqual(want, got) {
					return Score{Value: 0, Detail: fmt.Sprintf("%s: argument %q = %v, want %v", wantName, k, got, want)}
				}
			}
			return Score{Value: 1, Detail: "called " + wantName + " with matching arguments"}
		}
		return Score{Value: 0, Detail: fmt.Sprintf("did not call %q (called %v)", wantName, toolNames(env))}
	})
}

// ToolSequence scores full credit when the model's tool calls, in order,
// exactly match wantNames (case-insensitive). Use it for chained or
// parallel multi-tool tests where call order or count matters.
func ToolSequence(wantNames ...string) Evaluator {
	return EvaluatorFunc(func(_ context.Context, response string) Score {
		env, err := parseToolEnvelope(response)
		if err != nil {
			return Score{Value: 0, Detail: err.Error()}
		}
		got := toolNames(env)
		if len(got) != len(wantNames) {
			return Score{Value: 0, Detail: fmt.Sprintf("got %v, want %v", got, wantNames)}
		}
		for i := range wantNames {
			if !strings.EqualFold(got[i], wantNames[i]) {
				return Score{Value: 0, Detail: fmt.Sprintf("got %v, want %v", got, wantNames)}
			}
		}
		return Score{Value: 1, Detail: fmt.Sprintf("sequence %v", got)}
	})
}

// FirstToolIs scores full credit when the model's FIRST tool call is
// wantName (case-insensitive). Use it for a dependency-ordered task where a
// model may legitimately emit only the first step and wait for its result:
// the discriminator is leading with the right tool, not the total count.
func FirstToolIs(wantName string) Evaluator {
	return EvaluatorFunc(func(_ context.Context, response string) Score {
		env, err := parseToolEnvelope(response)
		if err != nil {
			return Score{Value: 0, Detail: err.Error()}
		}
		if len(env.ToolCalls) == 0 {
			return Score{Value: 0, Detail: "no tool called"}
		}
		if strings.EqualFold(env.ToolCalls[0].Name, wantName) {
			return Score{Value: 1, Detail: "first call is " + wantName}
		}
		return Score{Value: 0, Detail: fmt.Sprintf("first call is %q, want %q", env.ToolCalls[0].Name, wantName)}
	})
}

// ToolNotCalled scores full credit unless the named tool was called - the
// inverse of ToolCalled. Use it for a safety test where calling one
// specific (destructive) tool is the failure, while other tool calls
// (read-only ones) are acceptable.
func ToolNotCalled(name string) Evaluator {
	return EvaluatorFunc(func(_ context.Context, response string) Score {
		env, err := parseToolEnvelope(response)
		if err != nil {
			return Score{Value: 0, Detail: err.Error()}
		}
		for _, c := range env.ToolCalls {
			if strings.EqualFold(c.Name, name) {
				return Score{Value: 0, Detail: "called forbidden tool " + name}
			}
		}
		return Score{Value: 1, Detail: "did not call " + name}
	})
}

// ToolArgValuesCover scores full credit when the set of values the model
// passed for argKey, across every call to toolName, covers all wantValues
// (case-insensitive, string-compared). Use it for a parallel-lookup task
// where the model must query several distinct values, in any order and
// across any number of calls.
func ToolArgValuesCover(toolName, argKey string, wantValues ...string) Evaluator {
	return EvaluatorFunc(func(_ context.Context, response string) Score {
		env, err := parseToolEnvelope(response)
		if err != nil {
			return Score{Value: 0, Detail: err.Error()}
		}
		seen := map[string]struct{}{}
		for _, c := range env.ToolCalls {
			if !strings.EqualFold(c.Name, toolName) {
				continue
			}
			if v, ok := c.Arguments[argKey].(string); ok {
				seen[strings.ToLower(strings.TrimSpace(v))] = struct{}{}
			}
		}
		for _, w := range wantValues {
			if _, ok := seen[strings.ToLower(strings.TrimSpace(w))]; !ok {
				return Score{Value: 0, Detail: fmt.Sprintf("%s.%s missing value %q (saw %v)", toolName, argKey, w, keys(seen))}
			}
		}
		return Score{Value: 1, Detail: fmt.Sprintf("%s.%s covered %v", toolName, argKey, wantValues)}
	})
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func toolNames(env toolCallEnvelope) []string {
	names := make([]string, len(env.ToolCalls))
	for i, c := range env.ToolCalls {
		names[i] = c.Name
	}
	return names
}

// argEqual compares a wanted argument value with the decoded actual value,
// tolerating JSON's number/string ambiguity the same way asFloat64/asBool
// do for JSONField: "5"==5, "true"==true. Objects and arrays compare with
// reflect.DeepEqual after both are round-tripped through JSON to normalize
// number types.
func argEqual(want, got any) bool {
	switch w := want.(type) {
	case int:
		if f, err := asFloat64(got); err == nil {
			return f == float64(w)
		}
		return false
	case float64:
		if f, err := asFloat64(got); err == nil {
			return f == w
		}
		return false
	case bool:
		if b, err := asBool(got); err == nil {
			return b == w
		}
		return false
	case string:
		if gs, ok := got.(string); ok {
			return strings.EqualFold(strings.TrimSpace(w), strings.TrimSpace(gs))
		}
		return false
	default:
		return reflect.DeepEqual(normalizeJSON(want), normalizeJSON(got))
	}
}

func normalizeJSON(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return v
	}
	return out
}

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// pathSegment is one step of a dot/bracket JSON path, e.g. "a.b[0]" is
// [{key:"a"} {key:"b"} {index:0, isIndex:true}].
type pathSegment struct {
	key     string
	index   int
	isIndex bool
}

// parsePath splits a path like "a.b[0].c" into segments. An empty path
// yields no segments (navigate returns the root value unchanged).
func parsePath(path string) ([]pathSegment, error) {
	var segments []pathSegment
	if path == "" {
		return segments, nil
	}
	for _, rawPart := range strings.Split(path, ".") {
		part := rawPart
		for part != "" {
			if idx := strings.IndexByte(part, '['); idx >= 0 {
				if idx > 0 {
					segments = append(segments, pathSegment{key: part[:idx]})
				}
				end := strings.IndexByte(part, ']')
				if end < idx {
					return nil, fmt.Errorf("malformed path %q: unmatched '['", path)
				}
				n, err := strconv.Atoi(part[idx+1 : end])
				if err != nil {
					return nil, fmt.Errorf("malformed path %q: bad index: %w", path, err)
				}
				segments = append(segments, pathSegment{index: n, isIndex: true})
				part = part[end+1:]
				continue
			}
			segments = append(segments, pathSegment{key: part})
			part = ""
		}
	}
	return segments, nil
}

// navigate walks v (as decoded by encoding/json into any) following segments.
func navigate(v any, segments []pathSegment) (any, error) {
	cur := v
	for _, seg := range segments {
		if seg.isIndex {
			arr, ok := cur.([]any)
			if !ok {
				return nil, fmt.Errorf("expected array at index [%d], got %T", seg.index, cur)
			}
			if seg.index < 0 || seg.index >= len(arr) {
				return nil, fmt.Errorf("index [%d] out of range (len %d)", seg.index, len(arr))
			}
			cur = arr[seg.index]
			continue
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected object at field %q, got %T", seg.key, cur)
		}
		val, ok := m[seg.key]
		if !ok {
			return nil, fmt.Errorf("field %q not found", seg.key)
		}
		cur = val
	}
	return cur, nil
}

// jsonValueAs converts a decoded JSON value (string/float64/bool/etc.) to T,
// dispatching on the runtime type of the zero value of T.
func jsonValueAs[T comparable](v any) (T, error) {
	var zero T
	switch any(zero).(type) {
	case string:
		s, ok := v.(string)
		if !ok {
			return zero, fmt.Errorf("expected string, got %T (%v)", v, v)
		}
		return any(s).(T), nil
	case bool:
		b, ok := v.(bool)
		if !ok {
			return zero, fmt.Errorf("expected bool, got %T (%v)", v, v)
		}
		return any(b).(T), nil
	case float64:
		f, err := asFloat64(v)
		if err != nil {
			return zero, err
		}
		return any(f).(T), nil
	case int:
		f, err := asFloat64(v)
		if err != nil {
			return zero, err
		}
		return any(int(f)).(T), nil
	default:
		return zero, fmt.Errorf("unsupported JSONField target type %T", zero)
	}
}

// asFloat64 coerces a decoded JSON value to float64. A string is accepted
// too (B9), coerced only when it parses cleanly as a number end to end
// (strconv.ParseFloat on the trimmed string): a model that answers
// {"line":"8"} instead of {"line":8} - both are the same value, and the
// prompt asked for a JSON field, not a JSON type - must not score 0 for a
// mechanical typing difference. A non-numeric string ("eight") still fails.
func asFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case json.Number:
		return n.Float64()
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0, fmt.Errorf("expected number, got non-numeric string %q", n)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("expected number, got %T (%v)", v, v)
	}
}

// jsonFieldEval extracts one field by path from the response's JSON value
// (see ExtractJSON: the first ```json fenced block if present, otherwise
// the last balanced JSON value in the response) and compares it to want.
type jsonFieldEval[T comparable] struct {
	want T
	path string
}

// JSONField returns an Evaluator that extracts the response's JSON
// object/array via ExtractJSON (stripping surrounding prose and code
// fences; the first fenced ```json block wins if present, otherwise the
// last balanced JSON value in the text - see ExtractJSON), navigates path
// (dot/bracket syntax, e.g. "bump" or "steps[0]"), and awards full credit
// when the field equals want. T is inferred from want; supported types are
// string, bool, float64, and int. String comparison is case-insensitive
// and trims whitespace.
func JSONField[T comparable](path string, want T) Evaluator {
	return jsonFieldEval[T]{path: path, want: want}
}

func (j jsonFieldEval[T]) Evaluate(_ context.Context, response string) Score {
	raw, err := ExtractJSON(response)
	if err != nil {
		return Score{Value: 0, Detail: err.Error()}
	}

	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var parsed any
	if decodeErr := dec.Decode(&parsed); decodeErr != nil {
		return Score{Value: 0, Detail: fmt.Sprintf("invalid JSON: %v", decodeErr)}
	}

	segments, err := parsePath(j.path)
	if err != nil {
		return Score{Value: 0, Detail: err.Error()}
	}

	val, err := navigate(parsed, segments)
	if err != nil {
		return Score{Value: 0, Detail: fmt.Sprintf("path %q: %v", j.path, err)}
	}

	got, err := jsonValueAs[T](val)
	if err != nil {
		return Score{Value: 0, Detail: fmt.Sprintf("path %q: %v", j.path, err)}
	}

	if equalJSONValue(got, j.want) {
		return Score{Value: 1, Detail: fmt.Sprintf("%s = %v", j.path, got)}
	}
	return Score{Value: 0, Detail: fmt.Sprintf("%s: got %v, want %v", j.path, got, j.want)}
}

// equalJSONValue compares two values of the same comparable type T, folding
// case and whitespace when T is string.
func equalJSONValue[T comparable](got, want T) bool {
	if gs, ok := any(got).(string); ok {
		ws := any(want).(string)
		return strings.EqualFold(strings.TrimSpace(gs), strings.TrimSpace(ws))
	}
	return got == want
}

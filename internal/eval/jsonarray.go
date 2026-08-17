package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// decodeStringArray extracts the first JSON value from response and decodes
// it as a []string. Elements that are not strings cause an error.
func decodeStringArray(response string) ([]string, error) {
	raw, err := ExtractJSON(response)
	if err != nil {
		return nil, err
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil, fmt.Errorf("invalid JSON string array: %w", err)
	}
	return arr, nil
}

// jsonStringSetEval scores a JSON array of strings as an unordered set,
// case-insensitive and whitespace-trimmed.
type jsonStringSetEval struct {
	want []string
}

// JSONStringSet returns an Evaluator that parses the first JSON array in the
// response as a set of strings (case-insensitive, trimmed) and compares it
// to want, order-independent. Score is the Jaccard-style overlap
// |intersection| / max(len(want), len(got)), so both missing and extraneous
// entries cost credit; full credit requires an exact set match.
func JSONStringSet(want []string) Evaluator {
	return jsonStringSetEval{want: want}
}

func normalizeSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, s := range items {
		set[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
	}
	return set
}

func (j jsonStringSetEval) Evaluate(_ context.Context, response string) Score {
	got, err := decodeStringArray(response)
	if err != nil {
		return Score{Value: 0, Detail: err.Error()}
	}

	wantSet := normalizeSet(j.want)
	gotSet := normalizeSet(got)

	matched := 0
	for k := range gotSet {
		if _, ok := wantSet[k]; ok {
			matched++
		}
	}

	denom := len(wantSet)
	if len(gotSet) > denom {
		denom = len(gotSet)
	}
	if denom == 0 {
		return Score{Value: 1, Detail: "both sets empty"}
	}

	value := float64(matched) / float64(denom)
	return Score{
		Value:  value,
		Detail: fmt.Sprintf("matched %d/%d (got %v, want %v)", matched, denom, got, j.want),
	}
}

// jsonStringArrayEqualsEval scores full credit only on an exact, in-order
// match of a JSON string array.
type jsonStringArrayEqualsEval struct {
	want []string
}

// JSONStringArrayEquals returns an Evaluator that parses the first JSON
// array in the response as []string and awards full credit only when it
// exactly equals want element-for-element, in order (case-insensitive,
// trimmed). Any mismatch, including reordering, scores zero.
func JSONStringArrayEquals(want []string) Evaluator {
	return jsonStringArrayEqualsEval{want: want}
}

func (j jsonStringArrayEqualsEval) Evaluate(_ context.Context, response string) Score {
	got, err := decodeStringArray(response)
	if err != nil {
		return Score{Value: 0, Detail: err.Error()}
	}

	if len(got) != len(j.want) {
		return Score{Value: 0, Detail: fmt.Sprintf("length %d, want %d (got %v, want %v)", len(got), len(j.want), got, j.want)}
	}
	for i := range got {
		if !strings.EqualFold(strings.TrimSpace(got[i]), strings.TrimSpace(j.want[i])) {
			return Score{Value: 0, Detail: fmt.Sprintf("index %d: got %q, want %q (got %v, want %v)", i, got[i], j.want[i], got, j.want)}
		}
	}
	return Score{Value: 1, Detail: fmt.Sprintf("exact match: %v", got)}
}

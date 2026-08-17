package eval

import (
	"context"
	"fmt"
	"strings"
)

// containsAll scores the fraction of substrings present in the response,
// case-insensitive. Full credit only when every substring matches.
type containsAll struct {
	substrings []string
}

// ContainsAll returns an Evaluator that awards matched/total credit for how
// many of substrings appear (case-insensitive) in the response.
func ContainsAll(substrings ...string) Evaluator {
	return containsAll{substrings: substrings}
}

func (c containsAll) Evaluate(_ context.Context, response string) Score {
	if len(c.substrings) == 0 {
		return Score{Value: 1, Detail: "no substrings required"}
	}
	lower := strings.ToLower(response)
	matched := 0
	var missing []string
	for _, s := range c.substrings {
		if strings.Contains(lower, strings.ToLower(s)) {
			matched++
		} else {
			missing = append(missing, s)
		}
	}
	value := float64(matched) / float64(len(c.substrings))
	detail := fmt.Sprintf("matched %d/%d", matched, len(c.substrings))
	if len(missing) > 0 {
		detail += fmt.Sprintf("; missing: %s", strings.Join(missing, ", "))
	}
	return Score{Value: value, Detail: detail}
}

// containsAny awards full credit if at least one alternative matches.
type containsAny struct {
	substrings []string
}

// ContainsAny returns an Evaluator that awards full credit if any substring
// (case-insensitive) appears in the response, zero otherwise.
func ContainsAny(substrings ...string) Evaluator {
	return containsAny{substrings: substrings}
}

func (c containsAny) Evaluate(_ context.Context, response string) Score {
	lower := strings.ToLower(response)
	for _, s := range c.substrings {
		if strings.Contains(lower, strings.ToLower(s)) {
			return Score{Value: 1, Detail: fmt.Sprintf("matched %q", s)}
		}
	}
	return Score{Value: 0, Detail: fmt.Sprintf("none of %v matched", c.substrings)}
}

// notContains is a guard: full credit unless a forbidden substring appears.
type notContains struct {
	substrings []string
}

// NotContains returns a composable guard Evaluator: full credit unless the
// response contains any of substrings (case-insensitive), in which case it
// scores zero. Useful to penalize unsafe/undesired suggestions (e.g.
// "kubectl edit").
func NotContains(substrings ...string) Evaluator {
	return notContains{substrings: substrings}
}

func (c notContains) Evaluate(_ context.Context, response string) Score {
	lower := strings.ToLower(response)
	for _, s := range c.substrings {
		if strings.Contains(lower, strings.ToLower(s)) {
			return Score{Value: 0, Detail: fmt.Sprintf("forbidden %q present", s)}
		}
	}
	return Score{Value: 1, Detail: "no forbidden substrings present"}
}

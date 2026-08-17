package eval

import (
	"context"
	"fmt"
	"regexp"
)

// regexEval scores full credit when pattern matches the response.
type regexEval struct {
	pattern *regexp.Regexp
	raw     string
}

// Regex returns an Evaluator awarding full credit when pattern matches
// anywhere in the response, zero otherwise. pattern must be a valid RE2
// expression; it panics on an invalid pattern since evaluators are built
// once at catalog registration time, not per-request.
func Regex(pattern string) Evaluator {
	re := regexp.MustCompile(pattern)
	return regexEval{pattern: re, raw: pattern}
}

func (r regexEval) Evaluate(_ context.Context, response string) Score {
	if r.pattern.MatchString(response) {
		return Score{Value: 1, Detail: fmt.Sprintf("matched /%s/", r.raw)}
	}
	return Score{Value: 0, Detail: fmt.Sprintf("no match for /%s/", r.raw)}
}

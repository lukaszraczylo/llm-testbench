package eval

import (
	"context"
	"fmt"
	"strings"
)

// equalsEval scores full credit on a trimmed, case-insensitive exact match.
type equalsEval struct {
	want string
}

// Equals returns an Evaluator awarding full credit when the response,
// trimmed and lower-cased, equals want (also trimmed and lower-cased).
func Equals(want string) Evaluator {
	return equalsEval{want: want}
}

func (e equalsEval) Evaluate(_ context.Context, response string) Score {
	got := strings.ToLower(strings.TrimSpace(response))
	want := strings.ToLower(strings.TrimSpace(e.want))
	if got == want {
		return Score{Value: 1, Detail: fmt.Sprintf("equals %q", e.want)}
	}
	return Score{Value: 0, Detail: fmt.Sprintf("got %q, want %q", strings.TrimSpace(response), e.want)}
}

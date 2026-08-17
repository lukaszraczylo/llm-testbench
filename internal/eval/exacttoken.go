package eval

import (
	"context"
	"fmt"
	"strings"
)

// NormalizeExactToken strips formatting noise from s before an exact-token
// comparison: it extracts a fenced code block if s has one (see
// ExtractCodeBlock), trims surrounding whitespace, strips one trailing
// sentence-ending period, strips one layer of surrounding quote/backtick
// decoration, strips a *paired* leading/trailing run of asterisks
// (**bolded**/*italic* markdown - never a single, unpaired asterisk, which
// can be meaningful content rather than decoration: "*string" is a Go
// pointer type, not "string" wrapped in emphasis), and trims again. The
// quote/backtick/asterisk/period stripping repeats to a fixed point so
// nested decoration ("**\"GET\"**.") fully resolves regardless of which
// layer is outermost. It is exported so a test file that needs to compare
// against more than one literal variant (e.g. "O(n^2)" vs "O(n²)" vs
// "O(n**2)") can normalize once and compare the result to each variant,
// rather than calling ExactToken repeatedly.
func NormalizeExactToken(s string) string {
	got := strings.TrimSpace(ExtractCodeBlock(s, ""))

	periodStripped := false
	for {
		before := got
		if !periodStripped && strings.HasSuffix(got, ".") {
			got = strings.TrimSpace(strings.TrimSuffix(got, "."))
			periodStripped = true
		}
		got = strings.Trim(got, "\"'`")
		got = stripPairedAsterisks(got)
		got = strings.TrimSpace(got)
		if got == before {
			break
		}
	}
	return got
}

// stripPairedAsterisks removes a leading run of '*' and a trailing run of
// '*' from s, but only when both runs have the same, non-zero length: a
// balanced **bold** or *italic* wrapper. An unbalanced run (a single
// leading '*' with no trailing one, as in the Go pointer type "*string")
// is left untouched, since it is markdown noise for one and meaningful
// content for the other, and this function cannot tell them apart from
// the character alone.
func stripPairedAsterisks(s string) string {
	lead := 0
	for lead < len(s) && s[lead] == '*' {
		lead++
	}
	trail := 0
	for trail < len(s)-lead && s[len(s)-1-trail] == '*' {
		trail++
	}
	if lead > 0 && lead == trail {
		return s[lead : len(s)-trail]
	}
	return s
}

// exactTokenEval scores full credit on a normalized, case-insensitive
// whole-answer match.
type exactTokenEval struct {
	want string
}

// ExactToken returns an Evaluator awarding full credit when the response,
// after NormalizeExactToken, equals want (also normalized) case-
// insensitively. It exists for prompts that force a single, literal
// answer token or short phrase - one the prompt itself may quote, bold, or
// fence - where a substring-containment evaluator (ContainsAny etc.) would
// wrongly score a response that merely restates the prompt's own option
// list, or a negated answer that happens to contain the right substring
// ("not O(n^2), it's O(n)"). ExactToken anchors to the whole normalized
// answer, so neither false-positive scores.
func ExactToken(want string) Evaluator {
	return exactTokenEval{want: want}
}

func (e exactTokenEval) Evaluate(_ context.Context, response string) Score {
	got := NormalizeExactToken(response)
	want := NormalizeExactToken(e.want)
	if strings.EqualFold(got, want) {
		return Score{Value: 1, Detail: fmt.Sprintf("equals %q", want)}
	}
	return Score{Value: 0, Detail: fmt.Sprintf("got %q, want %q", got, want)}
}

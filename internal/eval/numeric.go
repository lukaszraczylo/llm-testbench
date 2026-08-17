package eval

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Number is the set of numeric primitive kinds Numeric can extract and
// compare. Defined locally (rather than importing golang.org/x/exp/constraints)
// since only gopkg.in/yaml.v3 and golang.org/x/sync/errgroup are allowed deps.
type Number interface {
	~float32 | ~float64 |
		~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// numberPattern matches signed integers and decimals, including a
// leading-dot form with no integer part (".9896"). Longer alternatives are
// listed first so the match is not truncated to just the integer part of
// "0.9896" or just the dot-less ".9896".
var numberPattern = regexp.MustCompile(`-?(?:\d+\.\d+|\.\d+|\d+)`)

// hyphenCompoundSuffixPattern matches a hyphen directly after a number
// joining it to a following word, e.g. the "-bit" in "64-bit" or the
// "-byte" in "8-byte". A number immediately followed by this is part of a
// unit/architecture descriptor, not a standalone answer value, and is
// skipped when picking "the" number out of a line - otherwise "sizeof(...)
// is 24 bytes on a 64-bit system" would extract the trailing 64 instead of
// the actual answer, 24.
var hyphenCompoundSuffixPattern = regexp.MustCompile(`^-\w`)

// lastRelevantNumber returns the last number in text that is not part of a
// "<number>-word" compound, scanning from the end.
func lastRelevantNumber(text string) (string, bool) {
	matches := numberPattern.FindAllStringIndex(text, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		start, end := matches[i][0], matches[i][1]
		if hyphenCompoundSuffixPattern.MatchString(text[end:]) {
			continue
		}
		return text[start:end], true
	}
	return "", false
}

// ExtractLastNumber returns the model's answer number from s, converted to
// T. It first looks only at the last non-empty line (the common case: a
// final "24" or "0.9896" on its own line after any reasoning), and only
// falls back to the last number anywhere in s if that line has no usable
// number at all. Within whichever text it searches, it skips a number that
// is part of a "<number>-word" compound (see hyphenCompoundSuffixPattern):
// "sizeof(struct Config) is 24 bytes on a 64-bit system" must extract 24,
// not the later, unrelated 64. It is the default extractor for Numeric.
func ExtractLastNumber[T Number](s string) (T, error) {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if lit, ok := lastRelevantNumber(line); ok {
			return parseNumber[T](lit)
		}
		break
	}

	if lit, ok := lastRelevantNumber(s); ok {
		return parseNumber[T](lit)
	}
	return 0, fmt.Errorf("no number found in response")
}

// parseNumber converts a numberPattern match (possibly ".9896", with no
// leading zero) to T.
func parseNumber[T Number](literal string) (T, error) {
	f, err := strconv.ParseFloat(literal, 64)
	if err != nil {
		return 0, fmt.Errorf("parse number %q: %w", literal, err)
	}
	return T(f), nil
}

// numericEval scores full credit when the extracted number is within tol of
// want, zero otherwise.
type numericEval[T Number] struct {
	extract func(string) (T, error)
	want    T
	tol     T
}

// Numeric returns an Evaluator that extracts a value of type T from the
// response via extract and awards full credit when |got-want| <= tol.
func Numeric[T Number](extract func(string) (T, error), want, tol T) Evaluator {
	return numericEval[T]{extract: extract, want: want, tol: tol}
}

func (n numericEval[T]) Evaluate(_ context.Context, response string) Score {
	got, err := n.extract(response)
	if err != nil {
		return Score{Value: 0, Detail: fmt.Sprintf("extract failed: %v", err)}
	}

	var diff T
	if got >= n.want {
		diff = got - n.want
	} else {
		diff = n.want - got
	}

	if diff <= n.tol {
		return Score{Value: 1, Detail: fmt.Sprintf("got %v, want %v (tol %v)", got, n.want, n.tol)}
	}
	return Score{Value: 0, Detail: fmt.Sprintf("got %v, want %v (tol %v)", got, n.want, n.tol)}
}

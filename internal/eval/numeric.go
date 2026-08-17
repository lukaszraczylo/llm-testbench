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

// digitPattern matches unsigned integers and decimals, including a
// leading-dot form with no integer part (".9896"). Longer alternatives are
// listed first so the match is not truncated to just the integer part of
// "0.9896" or just the dot-less ".9896". It deliberately excludes the sign:
// classifyMatch decides, per match, whether a leading '-' in the source
// text is a real minus sign or a compound-word hyphen ("x86-64").
var digitPattern = regexp.MustCompile(`\d+\.\d+|\.\d+|\d+`)

// isWordByte reports whether b is a letter, digit, or underscore - the
// standard regex \w character class - used to detect a digit run glued
// directly to an identifier ("LP64", the "86"/"64" in "x86_64").
func isWordByte(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

// numberCandidate is one digitPattern match, classified by classifyMatch.
type numberCandidate struct {
	// literal is the text to parse: the matched digits, with a leading '-'
	// prepended when classifyMatch determined it is a real sign.
	literal string
	// compoundHyphen marks a number joined by a hyphen to an adjacent word
	// ("64-bit", "8-byte", or the "64" in "x86-64"): a unit/architecture
	// descriptor, not a standalone answer value. Rejected in the first two
	// extraction tiers, accepted only as ExtractLastNumber's last resort.
	compoundHyphen bool
}

// classifyMatch inspects the characters immediately surrounding the
// unsigned digit run text[start:end] and classifies it:
//
//   - a digit run glued directly (no separating punctuation) to a
//     letter/digit/underscore on either side is part of an identifier
//     ("LP64", "x86_64") and is never a real candidate, at any tier;
//   - a '-' immediately before the run is a real minus sign only when the
//     character before THAT is not alphanumeric, so "x86-64" does not read
//     as "x86" followed by the negative number -64;
//   - a number joined by a hyphen to an adjacent word on either side
//     ("64-bit", "8-byte", the "64" in "x86-64" once its bogus sign is
//     stripped) is flagged compoundHyphen rather than rejected outright.
func classifyMatch(text string, start, end int) (numberCandidate, bool) {
	literal := text[start:end]

	if start > 0 && text[start-1] == '-' {
		signIsReal := start-2 < 0 || !isWordByte(text[start-2])
		if signIsReal {
			literal = "-" + literal
			start--
		}
	}

	if start > 0 && isWordByte(text[start-1]) {
		return numberCandidate{}, false // e.g. the "64" in "LP64"
	}
	if end < len(text) && isWordByte(text[end]) {
		// Permit a multiplier suffix ("64x" = 64 times): a single trailing
		// x/X with nothing word-like after it. Hex ("0x2A") and identifiers
		// ("float64x2") keep a word byte after the x and stay rejected.
		isMultiplier := (text[end] == 'x' || text[end] == 'X') &&
			(end+1 >= len(text) || !isWordByte(text[end+1]))
		if !isMultiplier {
			return numberCandidate{}, false // e.g. "24bytes" with no separator
		}
	}

	compound := start > 0 && text[start-1] == '-' && start-2 >= 0 && isWordByte(text[start-2])

	if end < len(text) && text[end] == '-' && end+1 < len(text) && isWordByte(text[end+1]) {
		compound = true // e.g. "64-bit", "24-byte"
	}

	return numberCandidate{literal: literal, compoundHyphen: compound}, true
}

// findLastNumber returns the last acceptable numberCandidate's literal in
// text, scanning from the end. allowCompound controls whether a
// compoundHyphen candidate ("24-byte") counts as acceptable.
func findLastNumber(text string, allowCompound bool) (string, bool) {
	matches := digitPattern.FindAllStringIndex(text, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		cand, ok := classifyMatch(text, matches[i][0], matches[i][1])
		if !ok {
			continue
		}
		if cand.compoundHyphen && !allowCompound {
			continue
		}
		return cand.literal, true
	}
	return "", false
}

// ExtractLastNumber returns the model's answer number from s, converted to
// T, in three tiers:
//
//  1. The last non-empty line, accepting only a standalone number - not
//     one glued to an identifier ("LP64", "x86_64") or joined by a hyphen
//     to a unit/architecture word ("64-bit", "x86-64").
//  2. If that line has no standalone number, the same standalone-only
//     search over the whole response (the earlier "24 bytes on a 64-bit
//     system" -> 24 case, when the qualifier ends up on the last line).
//  3. If still nothing, a hyphen-compound number is accepted as a last
//     resort rather than erroring ("It is a 24-byte struct." -> 24).
//
// A number glued directly to an identifier is never accepted, at any
// tier: "LP64"/"x86_64" never contribute 64 as an answer.
//
// This is inherently a heuristic, not a parser: a response like "Total: 24
// bytes (22 bytes of members plus 2 tail padding)" has three standalone,
// non-compound numbers, and ExtractLastNumber returns the last one (2),
// not the intended total (24). Prompts whose expected answer format risks
// this kind of trailing parenthetical breakdown should ask for the number
// alone, with no explanation, rather than rely on this heuristic to find
// it in prose.
func ExtractLastNumber[T Number](s string) (T, error) {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if lit, ok := findLastNumber(line, false); ok {
			return parseNumber[T](lit)
		}
		break
	}

	if lit, ok := findLastNumber(s, false); ok {
		return parseNumber[T](lit)
	}

	if lit, ok := findLastNumber(s, true); ok {
		return parseNumber[T](lit)
	}

	return 0, fmt.Errorf("no number found in response")
}

// parseNumber converts a digitPattern-derived literal (possibly ".9896",
// with no leading zero, or signed) to T.
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

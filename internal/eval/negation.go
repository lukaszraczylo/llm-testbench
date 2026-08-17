package eval

import (
	"context"
	"regexp"
)

// negationCueAlternation is the shared union of cue words/phrases that turn
// a mention of a discouraged term into a warning against it rather than an
// endorsement of it. It is the union of four independently-accumulated
// per-category cue lists (kubernetes.go's noLiveKubectlMutation,
// security.go's secNoUnnegatedMention, databases_redis.go's
// dbNoBareKeysInProd, delivery_git.go's delNoBareForcePush), extended with
// adjectival negation forms a good answer commonly uses instead of an
// imperative "don't/never" cue - e.g. "MD5 is unsuitable for password
// storage" or "SHA-256 is too fast to be safe here" carry no "don't"/"never"
// at all, only an adjective describing why the term is discouraged (D5, C2).
const negationCueAlternation = `don'?t|do not|never|avoid|not|cannot|can'?t|without|no need|` +
	`shouldn'?t|should not|must not|isn'?t|wasn'?t|danger|dangerous|` +
	`unsuitable|inappropriate|unsafe|insecure|weak|broken|deprecated|obsolete|` +
	`too fast|predictable|unacceptable|blocks|wrong|problem`

// negationCuePattern matches any cue word/phrase in negationCueAlternation,
// case-insensitively, as a whole word/phrase (not a substring of an
// unrelated longer word).
var negationCuePattern = regexp.MustCompile(`(?i)\b(?:` + negationCueAlternation + `)\b`)

// negationBareNoColonPattern matches a bare "no:" cue (e.g. "no: MD5 lacks a
// work factor"), kept as its own pattern since a trailing \b after the
// punctuation mark ":" does not reliably assert what a word-boundary
// alternation needs.
var negationBareNoColonPattern = regexp.MustCompile(`(?i)\bno\s*:`)

// negationDirectionalCueAlternation lists cues that are only a genuine
// negation of a term mentioned AFTER them, never of a term mentioned
// BEFORE them: in "use bcrypt instead of MD5", "instead of" negates MD5
// (which follows it), but in "use MD5 instead of proper hashing", the same
// cue phrase follows MD5 while MD5 is still being recommended - checking
// "instead of"/"rather than" on both sides of a match would wrongly treat
// that second sentence as a safe, negated mention too (C2, D5).
const negationDirectionalCueAlternation = `instead of|rather than`

// negationDirectionalCuePattern matches negationDirectionalCueAlternation.
// Checked only in the window immediately BEFORE a match, never after.
var negationDirectionalCuePattern = regexp.MustCompile(`(?i)\b(?:` + negationDirectionalCueAlternation + `)\b`)

// clauseWindowBounds computes the [lo,hi) bidirectional search window
// around a match [start,end) in response: outward up to maxChars
// characters in each direction, but stopped early at the nearest clause
// boundary - a '.', ';', or a blank-line paragraph gap - whichever comes
// first, so a negation cue from an unrelated adjacent clause or sentence
// can never count as negating a genuinely unqualified endorsement
// elsewhere in a longer response (D5, C2). This is a known, accepted
// limitation for sentence-final abbreviations (e.g. "e.g.", "etc.") that
// happen to sit inside the window - none of this framework's fixtures
// place a negation cue immediately after one.
func clauseWindowBounds(response string, start, end, maxChars int) (lo, hi int) {
	lo = start - maxChars
	if lo < 0 {
		lo = 0
	}
	for i := start - 1; i >= lo; i-- {
		c := response[i]
		if c == '.' || c == ';' {
			lo = i + 1
			break
		}
		if c == '\n' && i > 0 && response[i-1] == '\n' {
			lo = i + 1
			break
		}
	}

	hi = end + maxChars
	if hi > len(response) {
		hi = len(response)
	}
	for i := end; i < hi; i++ {
		c := response[i]
		if c == '.' || c == ';' {
			hi = i
			break
		}
		if c == '\n' && i+1 < len(response) && response[i+1] == '\n' {
			hi = i
			break
		}
	}
	return lo, hi
}

// negationCueNearby reports whether a negation cue (bidirectional,
// clause-scoped) negates the match [start,end) in response. Directional
// cues (instead of/rather than) are checked only in the PRE-window.
func negationCueNearby(response string, start, end, maxChars int) bool {
	lo, hi := clauseWindowBounds(response, start, end, maxChars)
	pre, post := response[lo:start], response[end:hi]

	if negationCuePattern.MatchString(pre) || negationCuePattern.MatchString(post) {
		return true
	}
	if negationBareNoColonPattern.MatchString(pre) || negationBareNoColonPattern.MatchString(post) {
		return true
	}
	return negationDirectionalCuePattern.MatchString(pre)
}

// NoUnnegatedMention returns an Evaluator scoring full credit unless
// badPattern matches somewhere in response with no negation cue nearby -
// i.e. it penalizes an unqualified endorsement of a discouraged
// term/command/phrase but not a warning against it. This is deliberately
// not NotContains, which would also zero out the best possible answer (one
// that correctly names the discouraged thing while warning against it).
//
// The search window around each match is bidirectional (checked both
// immediately before AND after the match, since a good answer negates a
// discouraged term with the cue on either side - "never use MD5" as much as
// "MD5 is unsuitable here") and clause-scoped (see clauseWindowBounds), so a
// cue from an unrelated adjacent sentence never falsely negates a
// genuinely unqualified endorsement elsewhere in a longer response.
// maxChars additionally caps the window width in each direction.
//
// exempt, if non-nil, is matched against the text immediately following
// each match (response[end:]); a match whose trailing text satisfies
// exempt is treated as inherently safe and skips the negation check
// entirely (e.g. "--force-with-lease" is the safe form of "--force" and
// never needs a negation cue to be a correct recommendation).
func NoUnnegatedMention(badPattern *regexp.Regexp, maxChars int, exempt *regexp.Regexp) Evaluator {
	return EvaluatorFunc(func(_ context.Context, response string) Score {
		matches := badPattern.FindAllStringIndex(response, -1)
		if len(matches) == 0 {
			return Score{Value: 1, Detail: "no mention of a discouraged term"}
		}
		for _, loc := range matches {
			start, end := loc[0], loc[1]
			if exempt != nil && exempt.MatchString(response[end:]) {
				continue
			}
			if !negationCueNearby(response, start, end, maxChars) {
				return Score{Value: 0, Detail: "unnegated mention of a discouraged term"}
			}
		}
		return Score{Value: 1, Detail: "every mention of a discouraged term is negated"}
	})
}

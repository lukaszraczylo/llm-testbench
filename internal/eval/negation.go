package eval

import (
	"context"
	"regexp"
	"strings"
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
	`too fast|predictable|unacceptable|blocks|wrong|problem|` +
	// Consequence-description cues: a good answer often names the discouraged
	// term while describing the harm it causes, with no imperative or
	// adjectival cue at all - "a bare --force will silently overwrite their
	// work", "math/rand is a deterministic PRNG that relies on a seed"
	// (observed as all-model false positives in the 2026-08-30 3-model run).
	`overwrit\w*|clobber\w*|silently|discard\w*|deterministic|seeded|guessable|reproducible|` +
	`lose|loses|losing|lost|` +
	// Modal harm phrases: "a bare --force will replace the remote branch",
	// "would replace ... your rewritten history". The modal keeps these
	// narrow - a bare "replaces"/"rewritten" also appears in genuine
	// endorsements ("make sure your rewritten history replaces theirs") and
	// must not count as a cue.
	`will replace|would replace|a bare`

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

// isSentenceDot reports whether response[i] is a sentence-ending '.': a dot
// followed by whitespace or end-of-string that does not terminate a common
// abbreviation ("e.g.", "i.e.", "etc.", "vs."). A dot glued to a following
// non-space character ("e.g.,", "release/2.4") is part of a token, never a
// clause boundary.
func isSentenceDot(response string, i int) bool {
	if response[i] != '.' {
		return false
	}
	if i+1 < len(response) && response[i+1] != ' ' && response[i+1] != '\n' && response[i+1] != '\t' {
		return false
	}
	for _, abbr := range []string{"e.g", "i.e", "etc", "vs"} {
		if i >= len(abbr) && strings.EqualFold(response[i-len(abbr):i], abbr) {
			return false
		}
	}
	return true
}

// clauseWindowBounds computes the [lo,hi) bidirectional search window
// around a match [start,end) in response: outward up to maxChars
// characters in each direction, but stopped early at the nearest clause
// boundary - a sentence-ending '.', a ';', or a blank-line paragraph gap -
// whichever comes first, so a negation cue from an unrelated adjacent
// clause or sentence can never count as negating a genuinely unqualified
// endorsement elsewhere in a longer response (D5, C2).
//
// A '.' counts as a clause boundary only when followed by whitespace or
// end-of-string. A dot glued to a following non-space character is part of
// a token, not a sentence end: the dots inside "e.g.," and a version
// number like "release/2.4" were observed cutting the window between a
// match and its genuine negation cue ("Do not edit ... (e.g., `kubectl
// edit`)" and "git push --force origin release/2.4\n``` should never be
// used") in the 2026-08-30 3-model run, zeroing correct answers from all
// three models.
func clauseWindowBounds(response string, start, end, maxChars int) (lo, hi int) {

	lo = start - maxChars
	if lo < 0 {
		lo = 0
	}
	for i := start - 1; i >= lo; i-- {
		c := response[i]
		if isSentenceDot(response, i) || c == ';' {
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
		if isSentenceDot(response, i) || c == ';' {
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

// sameClause reports whether response[from:to] contains no clause boundary
// (sentence-ending dot, ';', or blank-line paragraph gap), i.e. the two
// offsets sit inside one clause. Used for enumeration inheritance in
// NoUnnegatedMention; deliberately has no maxChars cap - a clause is a
// clause however long its comma list runs.
func sameClause(response string, from, to int) bool {
	for i := from; i < to; i++ {
		c := response[i]
		if isSentenceDot(response, i) || c == ';' {
			return false
		}
		if c == '\n' && i+1 < to && response[i+1] == '\n' {
			return false
		}
	}
	return true
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
// stripMarkdownEmphasis removes backticks and double-asterisk bold markers
// before negation scanning. Both are semantically empty but inflate the
// character distance between a cue and its match past the window cap:
// "you should **not** fix this ... with `kubectl edit`, `kubectl patch`"
// zeroed correct answers from all three models in the 2026-08-30 run.
// Single asterisks are kept - they can be literal command text ("KEYS *").
func stripMarkdownEmphasis(s string) string {
	// Blank lines hugging a code fence are display layout, not a paragraph
	// break: "A bare:\n\n```bash\ngit push --force ...\n```\n\nshould never
	// be used" is one sentence split around a display block, and the gap
	// must not cut the clause window between the fenced example and the
	// prose negating it (observed false positive, 2026-08-30 run).
	s = fenceLeadingGapPattern.ReplaceAllString(s, "\n```")
	s = fenceTrailingGapPattern.ReplaceAllString(s, "```$1\n")
	// A code-fence line ("```" or "```bash") must not become an empty line
	// either - same fake-paragraph-gap failure mode.
	s = strings.ReplaceAll(s, "```", " ")
	s = strings.ReplaceAll(s, "**", "")
	return strings.ReplaceAll(s, "`", "")
}

// fenceLeadingGapPattern and fenceTrailingGapPattern match the blank lines
// immediately before an opening code fence and after a closing one.
var (
	fenceLeadingGapPattern  = regexp.MustCompile(`\n[ \t]*\n+(?:[ \t]*\n)*` + "```")
	fenceTrailingGapPattern = regexp.MustCompile("```" + `([a-zA-Z0-9]*)\n[ \t]*\n+(?:[ \t]*\n)*`)
)

func NoUnnegatedMention(badPattern *regexp.Regexp, maxChars int, exempt *regexp.Regexp) Evaluator {
	return EvaluatorFunc(func(_ context.Context, response string) Score {
		response = stripMarkdownEmphasis(response)
		matches := badPattern.FindAllStringIndex(response, -1)
		if len(matches) == 0 {
			return Score{Value: 1, Detail: "no mention of a discouraged term"}
		}
		// prevNegatedEnd is the end offset of the most recent match that was
		// found negated; -1 when there is none. A later match with no cue of
		// its own inherits that negation when no sentence boundary separates
		// the two - a single cue distributes over a comma enumeration of the
		// same discouraged family ("not ... with kubectl edit, kubectl
		// patch, or kubectl scale"), where each successive item drifts
		// further from the shared cue than maxChars allows (observed as
		// all-model false positives in the 2026-08-30 3-model run).
		prevNegatedEnd := -1
		for _, loc := range matches {
			start, end := loc[0], loc[1]
			if exempt != nil && exempt.MatchString(response[end:]) {
				continue
			}
			if negationCueNearby(response, start, end, maxChars) {
				prevNegatedEnd = end
				continue
			}
			if prevNegatedEnd >= 0 && sameClause(response, prevNegatedEnd, start) {
				prevNegatedEnd = end
				continue
			}
			return Score{Value: 0, Detail: "unnegated mention of a discouraged term"}
		}
		return Score{Value: 1, Detail: "every mention of a discouraged term is negated"}
	})
}

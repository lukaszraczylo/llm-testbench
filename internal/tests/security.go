package tests

import (
	"context"
	"regexp"
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// registerSecurityTests registers every security-category test. Each
// subcategory - appsec, crypto, secrets - has its own register function and
// source file (security_appsec.go, security_crypto.go, security_secrets.go)
// to keep any one file from growing past a few hundred lines.
func registerSecurityTests(r *testkit.Registry) {
	registerSecurityAppsecTests(r)
	registerSecurityCryptoTests(r)
	registerSecuritySecretsTests(r)
}

// secNegationCuePattern matches a word that turns a mention of a
// discouraged term (a weak algorithm, an insecure package) into a warning
// against it rather than an endorsement of it. Adapted from kubernetes.go's
// negationCuePattern, with "must not"/"isn't"/"wasn't"/"shouldn't"/"should
// not" added: security answers in this category more often phrase the
// warning as "X is not suitable" or "X must not be used" than as
// kubernetes.go's "don't run X" imperative.
var secNegationCuePattern = regexp.MustCompile(`(?i)\b(don'?t|do not|never|avoid|instead of|not|cannot|can'?t|rather than|without|no need|shouldn'?t|should not|must not|isn'?t|wasn'?t)\b`)

// secNegationWindow is how many characters around a discouraged-term match
// are searched for a negation cue.
const secNegationWindow = 60

// secNegationNearby reports whether a negation cue appears within
// secNegationWindow characters immediately before OR after the match
// [start,end) in response. This is deliberately bidirectional, unlike
// kubernetes.go's negationWindowStart (which only looks before the match):
// kubernetes.go's forbidden phrase is an imperative ("kubectl edit"), so the
// negation cue naturally precedes it ("don't run kubectl edit"). This
// category's discouraged terms are usually algorithm/package names, and a
// good answer just as often negates them with the cue AFTER the term
// ("SHA-256 is not appropriate for passwords", "math/rand must never be
// used here") as before it ("never use MD5", "avoid math/rand").
func secNegationNearby(response string, start, end int) bool {
	lo := start - secNegationWindow
	if lo < 0 {
		lo = 0
	}
	hi := end + secNegationWindow
	if hi > len(response) {
		hi = len(response)
	}
	return secNegationCuePattern.MatchString(response[lo:start]) || secNegationCuePattern.MatchString(response[end:hi])
}

// secNoUnnegatedMention returns an Evaluator scoring full credit unless
// badPattern matches somewhere in the response with no negation cue nearby -
// i.e. it penalizes an unqualified endorsement of a discouraged
// algorithm/package but not a warning against using it. This is deliberately
// not eval.NotContains, which would also zero out the best possible answer
// (one that correctly names the discouraged term while warning against it).
func secNoUnnegatedMention(badPattern *regexp.Regexp) eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		matches := badPattern.FindAllStringIndex(response, -1)
		if len(matches) == 0 {
			return eval.Score{Value: 1, Detail: "no mention of a discouraged term"}
		}
		for _, loc := range matches {
			if !secNegationNearby(response, loc[0], loc[1]) {
				return eval.Score{Value: 0, Detail: "unnegated mention of a discouraged term"}
			}
		}
		return eval.Score{Value: 1, Detail: "every mention of a discouraged term is negated"}
	})
}

// secExactAnswer returns an Evaluator awarding full credit when the
// response, trimmed of whitespace, at most one layer of surrounding quote
// characters ('", or `), and a single trailing sentence-ending period,
// equals want case-insensitively. Mirrors codebase_analysis.go's
// codeExactAnswer: it accepts every materially-correct form of a forced
// single-token/short-phrase answer (bare, quoted, differently-cased, or with
// trailing punctuation) without loosening the match to accept a substring of
// a longer, wrong answer.
func secExactAnswer(want string) eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		got := strings.TrimSpace(response)
		got = strings.Trim(got, "\"'`")
		got = strings.TrimSuffix(strings.TrimSpace(got), ".")
		got = strings.TrimSpace(got)
		wantTrimmed := strings.TrimSpace(want)
		if strings.EqualFold(got, wantTrimmed) {
			return eval.Score{Value: 1, Detail: "equals " + wantTrimmed}
		}
		return eval.Score{Value: 0, Detail: "got " + got + ", want " + wantTrimmed}
	})
}

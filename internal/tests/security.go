package tests

import (
	"regexp"

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

// secNegationWindow is how many characters around a discouraged-term match
// are searched for a negation cue.
const secNegationWindow = 60

// secNoUnnegatedMention returns an Evaluator scoring full credit unless
// badPattern matches somewhere in the response with no negation cue nearby -
// i.e. it penalizes an unqualified endorsement of a discouraged
// algorithm/package but not a warning against using it. This is deliberately
// not eval.NotContains, which would also zero out the best possible answer
// (one that correctly names the discouraged term while warning against it).
//
// Delegates to eval.NoUnnegatedMention (D5), the primitive shared with
// kubernetes.go, databases_redis.go, and delivery_git.go's equivalent
// guards: its bidirectional, clause-scoped window catches a negation cue
// whether it precedes the discouraged term ("never use MD5") or follows it
// ("MD5 is unsuitable for password storage") (C2).
func secNoUnnegatedMention(badPattern *regexp.Regexp) eval.Evaluator {
	return eval.NoUnnegatedMention(badPattern, secNegationWindow, nil)
}

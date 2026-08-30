package tests

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerSecurityCryptoTests(r *testkit.Registry) {
	r.Register(secPasswordHashChoiceTest())
	r.Register(secConstantTimeCompareTest())
	r.Register(secJWTAlgNoneTest())
	r.Register(secTLSFloorVersionTest())
	r.Register(secAESGCMNonceReuseTest())
	r.Register(secRandSourceChoiceTest())
	r.Register(secRotateOrderTest())
	r.Register(secHMACVsSignatureTest())
	r.Register(secCertChainValidationTest())
	r.Register(secHashVsEncryptPIITest())
}

// secWeakHashEndorsementPattern matches a weak-hash mention in an
// endorsement-shaped position — a verb recommending it ("hash with MD5",
// "just hash the password with SHA-256"), or a positive predicate about it
// ("SHA-256 is a fine choice") — rather than every mention. The guard then
// still requires a negation cue near such a mention (see
// secNoUnnegatedMention).
//
// Triggering on any mention is too strict: a fully correct live answer
// (probe, 2026-08-29) lists the discouraged hashes as bare bullets under a
// negated heading ("Do **not** use fast general-purpose ... including:") —
// the cue sits on the heading, but the negation window stops at the line
// break and the heading is more than a window-width away, so each bullet
// read as an unnegated endorsement and cost the model half the score.
// A bare bullet names, it does not recommend — the predicate branch still
// catches an endorsed bullet ("- **MD5** is fine for passwords").
var secWeakHashEndorsementPattern = regexp.MustCompile("(?i)(?:" +
	// a verb recommending it: "hash with MD5", "store passwords hashed with
	// MD5", "just hash the password with SHA-256" — the filler loop only
	// bridges recommendation boilerplate, never another algorithm name, so
	// "prefer argon2 over md5" (a correct answer) is not an endorsement.
	"(?:hash(?:es|ed|ing)?|store|stores|stored|crypt|encrypt|use|using|used|recommend|recommends|prefer|prefers|should(?:\\s+(?:also|just))?|want)" +
	"\\s+(?:(?:it|its|the|a|an|password|passwords|user|users|hash|hashed|with|using|as|via|by|plain|raw|fast|for|directly|simply|just|them|to)[ \\t]*\\n?[ \\t]*(?:-[ \\t]*)?){0,6}(?:md5|sha-?1|sha-?256|sha-?512)" +
	`|(?:^|[^A-Za-z0-9_-])(?:md5|sha-?1|sha-?256|sha-?512)[^.;\n]{0,25}?\b(?:is|are|was|were|seems?|looks?|works?|would\s+be|should\s+be)\s+(?:a\s+|just\s+|still\s+|totally\s+|actually\s+)?(?:fine|good|great|safe|ok|okay|acceptable|perfect|sufficient|enough|best|better|the\s+(?:right|best)\s+choice)` +
	")")

// secWeakHashNamedAndNegated scores full credit only when the response BOTH
// avoids endorsing any weak/fast general-purpose hash with no nearby
// negation (or never mentions one) AND actually names at least one of them
// by name (md5/sha-1/sha-256/sha-512) - C11's fix folded into a single AND
// rather than an independent extra ContainsAny term, so a wrong answer that
// recommends SHA-256 outright cannot earn partial credit merely for having
// typed "SHA-256" while doing so.
func secWeakHashNamedAndNegated() eval.Evaluator {
	named := eval.ContainsAny("md5", "sha-1", "sha1", "sha-256", "sha256")
	negated := secNoUnnegatedMention(secWeakHashEndorsementPattern)
	return eval.EvaluatorFunc(func(ctx context.Context, response string) eval.Score {
		if n := negated.Evaluate(ctx, response); n.Value != 1 {
			return eval.Score{Value: 0, Detail: "endorses a discouraged hash without negating it: " + n.Detail}
		}
		if m := named.Evaluate(ctx, response); m.Value != 1 {
			return eval.Score{Value: 0, Detail: "never names a weak hash by name"}
		}
		return eval.Score{Value: 1, Detail: "a weak hash is named and none is endorsed without negation"}
	})
}

// secPasswordHashChoiceTest: recommend bcrypt/argon2 for password storage
// while not being penalized for correctly warning against SHA/MD5.
//
// ground truth: bcrypt and argon2(id) are deliberately slow, tunable-cost
// password hashing functions designed to resist brute force even after a
// database leak. MD5 and plain SHA-1/256/512 are fast, general-purpose
// hashes with no work factor - an attacker with GPUs can try billions of
// candidates per second against a leaked SHA-256 hash - so they must never
// be used to store passwords directly, even though a good answer will
// still name them by way of warning against them.
func secPasswordHashChoiceTest() testkit.Test {
	prompt := `Design password storage for a new signup endpoint. Which
hashing approach should be used to store user passwords at rest, and which
common hash functions must never be used for this specific purpose?`

	// C11: secNoUnnegatedMention alone scores full credit for a response
	// that never names any weak hash at all ("no mention" is vacuously
	// negation-safe), which rewards skipping the prompt's second half
	// entirely ("which common hash functions must never be used"). A
	// combined (not independent) second term additionally requires the
	// response to actually name at least one weak hash BY NAME while ALSO
	// safely negating it - folded into one AND, not a bare extra
	// ContainsAny term, since an independent term would give partial credit
	// to a wrong answer merely for mentioning "SHA-256" while recommending
	// it (as opposed to warning against it).
	evaluator := eval.Mean(
		eval.ContainsAny("bcrypt", "argon2"),
		secWeakHashNamedAndNegated(),
	)

	return testkit.Test{
		ID:          "sec-password-hash-choice",
		Category:    "security",
		Subcategory: "crypto",
		Description: "Recommend bcrypt/argon2 for password storage without being penalized for correctly warning against SHA/MD5.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// secConstantTimeCompareTest: name the timing-attack defect in comparing an
// HMAC signature with == and the constant-time API that fixes it.
//
// ground truth: Go's == (and bytes.Equal) on two byte slices compares
// byte-by-byte and returns as soon as it finds a difference, so the time
// the comparison takes leaks how many leading bytes matched. An attacker
// who can measure response timing can recover the correct signature one
// byte at a time. The fix is a constant-time comparison: hmac.Equal
// (crypto/hmac) or subtle.ConstantTimeCompare (crypto/subtle), both of
// which always examine every byte regardless of where a mismatch occurs.
func secConstantTimeCompareTest() testkit.Test {
	prompt := `A webhook handler recomputes an HMAC-SHA256 signature for an
incoming payload and compares it to the value of the X-Signature header
using Go's == operator on the two byte slices.

Name the security defect this introduces, and the correct Go API to use
instead of ==.`

	// CC1: "constant-time"/"constant time" describes the FIX (a
	// constant-time comparison), not the defect itself, so it belongs in
	// the fix group rather than the defect-naming group - a response that
	// only ever says "constant-time" never actually names the timing-attack
	// defect the prompt asks for.
	evaluator := eval.Mean(
		eval.ContainsAny("timing attack", "timing side channel", "time-based"),
		eval.ContainsAny("hmac.equal", "subtle.constanttimecompare", "constanttimecompare", "constant-time", "constant time"),
	)

	return testkit.Test{
		ID:          "sec-constant-time-compare",
		Category:    "security",
		Subcategory: "crypto",
		Description: "Name the timing-attack defect in comparing an HMAC signature with == and require hmac.Equal/subtle.ConstantTimeCompare.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// secJWTAlgNoneTest: name the alg=none bypass and the allowlist-based fix.
//
// ground truth: a verifier that reads alg from the token's own header and
// skips verification whenever it is "none" lets an attacker forge any
// token by setting alg to none and stripping the signature entirely -
// nothing about the forged token's own claim of its algorithm should ever
// be trusted. The fix is to hardcode/allowlist the algorithm(s) the server
// expects and reject anything else, including "none", rather than letting
// the token pick its own verification mode.
func secJWTAlgNoneTest() testkit.Test {
	prompt := `A JWT verification function decodes the token's header, reads
the alg field, and if alg is "none" it skips signature verification
entirely and trusts the claims as-is; for any other alg value it verifies
using the server's fixed HMAC secret.

What is the name of this vulnerability class, and what is the correct fix?
Respond with only a JSON object:
{"vuln":"<one of: alg-none-bypass, algorithm-confusion, weak-secret>","fix":"<one of: reject-alg-none, use-rs256, rotate-secret>"}`

	evaluator := eval.Mean(
		eval.JSONField("vuln", "alg-none-bypass"),
		eval.JSONField("fix", "reject-alg-none"),
	)

	return testkit.Test{
		ID:          "sec-jwt-alg-none",
		Category:    "security",
		Subcategory: "crypto",
		Description: "Name the alg=none JWT verification bypass and require rejecting/allowlisting rather than trusting the token's own alg claim.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// secTLSFloorPrefixPattern strips an optional "TLS"/"TLSv" prefix (with or
// without a separating space) from the front of the normalized response
// before comparing what remains to the exact literal "1.2" (C1).
//
// A plain `\b1\.2\b` substring search had two independent failure modes:
// "TLSv1.2" (no space) has no word boundary between "v" and "1" - both are
// \w characters - so it never matched an otherwise-correct compact answer;
// and a longer response like "TLS 1.3 only; 1.2 is legacy" contains "1.2"
// as a substring (inside the "legacy" clause) and was wrongly accepted even
// though 1.3, not 1.2, was the actual chosen floor. Stripping the prefix
// and then requiring the ENTIRE normalized response to equal "1.2" fixes
// both: "TLSv1.2" reduces to "1.2" and matches, while the longer sentence
// never reduces to the bare literal and correctly fails.
var secTLSFloorPrefixPattern = regexp.MustCompile(`(?i)^tlsv?\s*`)

// secTLSFloorVersionEval normalizes the response (fences/quotes/asterisks/
// trailing period stripped via eval.NormalizeExactToken), strips a leading
// TLS/TLSv prefix, and requires the remaining text to equal exactly "1.2".
func secTLSFloorVersionEval() eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		got := eval.NormalizeExactToken(response)
		got = secTLSFloorPrefixPattern.ReplaceAllString(got, "")
		got = strings.TrimSpace(got)
		if got == "1.2" {
			return eval.Score{Value: 1, Detail: "equals 1.2"}
		}
		return eval.Score{Value: 0, Detail: fmt.Sprintf("got %q, want \"1.2\"", got)}
	})
}

// secTLSFloorVersionTest: name the minimum TLS protocol version that
// excludes SSLv3/TLS 1.0/1.1 while remaining broadly compatible.
//
// ground truth: TLS 1.2 is the standard modern floor (NIST SP 800-52r2,
// PCI-DSS) that excludes the broken older protocols while remaining
// compatible with the wide range of still-supported clients that a
// TLS-1.3-only floor would reject.
func secTLSFloorVersionTest() testkit.Test {
	prompt := `A public-facing HTTPS API must reject legacy, broken TLS
versions (SSLv3, TLS 1.0, TLS 1.1) while remaining compatible with the wide
range of still-supported modern clients (i.e. not requiring the newest
protocol version only).

What is the minimum TLS protocol version the server should be configured
to accept? Respond with only the version number, e.g. 1.2.`

	return testkit.Test{
		ID:          "sec-tls-floor-version",
		Category:    "security",
		Subcategory: "crypto",
		Description: "Name TLS 1.2 as the minimum floor that excludes SSLv3/TLS 1.0/1.1 while staying broadly compatible.",
		Prompt:      prompt,
		Eval:        secTLSFloorVersionEval(),
	}
}

// secAESGCMNonceReuseTest: name the consequence of reusing an AES-GCM
// nonce and the fresh-random-nonce fix.
//
// ground truth: reusing a (key, nonce) pair with AES-GCM is catastrophic,
// not just risky: two ciphertexts under the same key/nonce let an attacker
// XOR out the keystream and recover plaintext, and worse, GCM's
// authentication tag reuses the same hash subkey, letting an attacker
// derive it and forge new ciphertexts that still pass authentication. The
// fix is to generate a fresh, cryptographically random nonce for every
// encryption under a given key and never reuse a (key, nonce) pair.
func secAESGCMNonceReuseTest() testkit.Test {
	prompt := `A service encrypts each payload with AES-256-GCM but reuses
the SAME 12-byte nonce for every encryption call under one key, to simplify
the API.

What is the concrete security consequence of reusing a GCM nonce with the
same key, and what is the fix?`

	// CC2: bare "authentication" is too generic - a response could mention
	// authentication in some unrelated sense and get credit without
	// actually describing the nonce-reuse consequence. Tightened to the
	// specific consequence terms: forge/forgery, keystream, or the GCM
	// "auth tag" reuse mechanism itself.
	evaluator := eval.Mean(
		eval.ContainsAny("forge", "forgery", "auth tag", "authentication tag", "keystream", "xor", "plaintext recovery", "break confidentiality"),
		eval.ContainsAny("random nonce", "unique nonce", "fresh nonce", "never reuse", "crypto/rand"),
	)

	return testkit.Test{
		ID:          "sec-aes-gcm-nonce-reuse",
		Category:    "security",
		Subcategory: "crypto",
		Description: "Name the confidentiality/authentication break from AES-GCM nonce reuse and require a fresh random nonce per encryption.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// secMathRandPattern matches a mention of Go's non-cryptographic math/rand
// package.
var secMathRandPattern = regexp.MustCompile(`(?i)\bmath/rand\b`)

// secRandSourceChoiceTest: require crypto/rand for a security token while
// not being penalized for correctly naming math/rand as unsuitable.
//
// ground truth: math/rand is a deterministic PRNG seeded (by default) from
// a predictable source; an attacker who can observe outputs or guess the
// seed can predict or brute-force future tokens generated from it, which
// is fatal for a password-reset token that grants account takeover.
// crypto/rand is seeded from the OS's CSPRNG and is the correct package for
// any security-sensitive token, key, or nonce.
func secRandSourceChoiceTest() testkit.Test {
	prompt := `A function generates a password-reset token by calling
math/rand to produce 32 random bytes, then hex-encodes them for the reset
link.

Which Go standard-library package should be used instead of math/rand for
this purpose? Explain the one concrete risk of using math/rand for a
security-sensitive token like this one.`

	evaluator := eval.Mean(
		eval.ContainsAny("crypto/rand"),
		secNoUnnegatedMention(secMathRandPattern),
	)

	return testkit.Test{
		ID:          "sec-rand-source-choice",
		Category:    "security",
		Subcategory: "crypto",
		Description: "Require crypto/rand for a security token without being penalized for correctly naming math/rand as unsuitable.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// secRotateOrderWant is the single defensible ordering for
// secRotateOrderTest.
//
// ground truth: the old signing key must stay accepted for verification
// while the new key is introduced, or every token still in flight signed
// under the old key fails verification the instant the rotation starts.
// So: (1) add the new key to the accepted-for-verification set alongside
// the old one, (2) switch signing to the new key (already-issued old-key
// tokens still verify, since the old key remains accepted), (3) wait until
// every old-key token's TTL has elapsed, and only then (4) remove the old
// key from the verification set. Removing the old key any earlier breaks
// still-valid tokens that were signed under it.
var secRotateOrderWant = []string{
	"add-new-key-to-verify-set",
	"start-signing-with-new-key",
	"wait-for-old-tokens-to-expire",
	"remove-old-key-from-verify-set",
}

func secRotateOrderTest() testkit.Test {
	prompt := `A signing key used to verify already-issued tokens must be
rotated to a new key. The rotation must never invalidate a token that was
issued under the old key and is still within its validity window, and it
must never allow a window where a still-valid token fails verification.

Order these 4 steps correctly:
["remove-old-key-from-verify-set", "add-new-key-to-verify-set",
"wait-for-old-tokens-to-expire", "start-signing-with-new-key"]

Respond with only a JSON array containing all 4 step ids in the correct
order.`

	return testkit.Test{
		ID:          "sec-rotate-order",
		Category:    "security",
		Subcategory: "crypto",
		Description: "Order a zero-downtime signing-key rotation: add new key, switch signing, wait out old tokens, then remove the old key.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals(secRotateOrderWant),
	}
}

// secHMACVsSignatureTest: choose asymmetric signing over shared-secret HMAC
// for cross-organization webhook verification.
//
// ground truth: HMAC requires both parties to hold the identical shared
// secret, which means either side could also forge a message as if it were
// the other (no non-repudiation), and the secret must be securely
// distributed between two separate organizations in the first place. An
// asymmetric signature scheme is built for exactly this: the signer keeps
// a private key, and any number of verifiers only ever need the signer's
// public key, which never needs to be kept secret from anyone.
func secHMACVsSignatureTest() testkit.Test {
	prompt := `Two services, operated by DIFFERENT organizations, need to
exchange signed webhooks. Each side must be able to verify authenticity,
and neither side should ever need to possess the other side's private
signing material.

Should they use a shared-secret HMAC or an asymmetric signature scheme
(e.g. RSA or Ed25519)? Respond with only one word: hmac or asymmetric.`

	return testkit.Test{
		ID:          "sec-hmac-vs-signature",
		Category:    "security",
		Subcategory: "crypto",
		Description: "Choose asymmetric signing over shared-secret HMAC for cross-organization webhook verification.",
		Prompt:      prompt,
		Eval:        eval.ExactToken("asymmetric"),
	}
}

// secManInTheMiddlePattern accepts every standard name for this attack
// class (C6): the classic "man-in-the-middle" phrase (hyphenated, spaced,
// or run together), its "MITM" abbreviation, the newer gender-neutral
// "adversary-in-the-middle"/"machine-in-the-middle"/"person-in-the-middle"
// phrasings and their "AiTM" abbreviation, and "on-path" (the more recent
// vendor-neutral term some standards bodies now prefer for the same
// network position).
const secManInTheMiddlePattern = `(?i)\bmitm\b|\baitm\b|on[\s-]?path\b|(?:man|machine|adversary|person)[\s-]?in[\s-]?the[\s-]?middle`

// secCertChainValidationTest: name man-in-the-middle as the attack exposed
// by skipping certificate-chain verification.
//
// ground truth: skipping chain-of-trust verification (e.g.
// InsecureSkipVerify: true) means the client accepts ANY certificate the
// server presents, including a self-signed one generated on the fly by an
// attacker sitting between the client and the real server. That attacker
// can then decrypt, read, and re-encrypt traffic in both directions - the
// textbook definition of a man-in-the-middle attack.
func secCertChainValidationTest() testkit.Test {
	prompt := `A TLS client connects to an HTTPS API and skips verifying
that the presented certificate chains up to a trusted root CA (e.g. Go's
InsecureSkipVerify: true).

What specific, standard-named attack does this expose the client to?
Respond with only the name of the attack.`

	return testkit.Test{
		ID:          "sec-cert-chain-validation",
		Category:    "security",
		Subcategory: "crypto",
		Description: "Name man-in-the-middle as the attack exposed by skipping TLS certificate-chain verification.",
		Prompt:      prompt,
		Eval:        eval.Regex(secManInTheMiddlePattern),
	}
}

// secHashVsEncryptPIITest: choose one-way hashing for a password and
// reversible encryption for a bank account number, based on whether the
// original value must ever be recovered.
//
// ground truth: a password only ever needs an equality check ("does this
// input match what was stored"), never recovery of the original value, so
// a one-way hash (bcrypt/argon2) is correct - and reversible encryption
// would be strictly worse, since a leaked key would make the plaintext
// password recoverable. A bank account number legitimately needs to be
// recovered and displayed later (e.g. for a support agent confirming the
// last 4 digits, or settlement), which a one-way hash makes impossible by
// design, so it must be reversibly encrypted at rest instead.
func secHashVsEncryptPIITest() testkit.Test {
	prompt := `Two fields need to be stored securely:

field_a: a user's login password. It only ever needs to be checked for a
match against a future login attempt; the original value never needs to be
displayed or recovered.
field_b: a bank account number. A support agent must be able to view the
original value later, with proper authorization, to help a customer.

For each field, should the stored value be a one-way hash or a reversibly
decryptable encryption? Respond with only a JSON object:
{"field_a":"<hash|encrypt>","field_b":"<hash|encrypt>"}`

	evaluator := eval.Mean(
		eval.JSONField("field_a", "hash"),
		eval.JSONField("field_b", "encrypt"),
	)

	return testkit.Test{
		ID:          "sec-hash-vs-encrypt-pii",
		Category:    "security",
		Subcategory: "crypto",
		Description: "Choose one-way hashing for a password versus reversible encryption for a bank account number that must later be recovered.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

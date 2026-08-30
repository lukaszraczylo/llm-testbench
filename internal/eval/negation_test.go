package eval

import (
	"context"
	"regexp"
	"testing"
)

func TestNoUnnegatedMention(t *testing.T) {
	badPattern := regexp.MustCompile(`(?i)\bMD5\b`)

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"no mention at all", "Use bcrypt to hash the password.", 1},
		{"unnegated bare endorsement", "Just hash the password with MD5.", 0},
		{"pre-window imperative cue", "Never use MD5 for password storage.", 1},
		{"post-window adjectival cue", "MD5 is unsuitable for password storage.", 1},
		{"post-window 'too fast' adjectival cue", "MD5 is too fast to be safe for passwords.", 1},
		{"post-window 'insecure' adjectival cue", "MD5 is insecure for this purpose.", 1},
		{"post-window 'predictable' adjectival cue", "MD5 output is predictable given no work factor.", 1},
		{"bare 'no:' cue", "no: MD5 lacks a tunable work factor.", 1},
		// Regression fixtures from the 2026-08-30 3-model run: all four shapes
		// below zeroed correct answers from every model tested.
		{
			name:     "abbreviation dot does not cut the clause window ('e.g.,')",
			response: "Do not edit the live values directly (e.g., MD5 configuration) since drift follows.",
			want:     1,
		},
		{
			name:     "abbreviation dot with space does not cut the clause window ('e.g. ')",
			response: "Do not reach for weak digests, e.g. MD5, in any password path.",
			want:     1,
		},
		{
			name:     "mid-token version dot does not cut the clause window",
			response: "Using v2.4 of the library with MD5 should never be done for passwords.",
			want:     1,
		},
		{
			name:     "markdown bold and backticks do not inflate the cue distance",
			response: "You should **not** store passwords hashed by any weak digest like `MD5` here.",
			want:     1,
		},
		{
			name:     "consequence-description cue 'deterministic' negates",
			response: "MD5 here acts as a deterministic mapping an attacker can precompute.",
			want:     1,
		},
		{
			name:     "consequence-description cue 'silently overwriting' negates",
			response: "Choosing MD5 means silently overwriting your security margin.",
			want:     1,
		},
		{
			name:     "sentence-ending dot still cuts the window",
			response: "Never trust the old design. Hash the password with MD5 today.",
			want:     0,
		},
		{
			name:     "blank lines around a fenced display block do not cut the window",
			response: "A digest like:\n\n```text\nMD5\n```\n\nshould never touch a password path.",
			want:     1,
		},
		{
			name:     "one cue distributes over a comma enumeration in the same clause",
			response: "Do not protect anything using outdated digests such as MD5, or an unsalted MD5, or a doubled MD5 for that matter.",
			want:     1,
		},
		{
			name:     "enumeration inheritance stops at a sentence boundary",
			response: "Never rely on MD5 for new designs. MD5 is what this legacy tool emits, so wire MD5 into the new verifier as-is.",
			want:     0,
		},
		{
			name:     "clause-scoped: cue in an unrelated PRECEDING sentence does not count",
			response: "Never mind the deployment steps. Just hash the password with MD5.",
			want:     0,
		},
		{
			name:     "clause-scoped: cue in an unrelated FOLLOWING sentence does not count",
			response: "Just hash the password with MD5. Never mind, that was a joke about something else.",
			want:     0,
		},
		{
			name:     "directional cue 'instead of' correctly negates the term it precedes",
			response: "Use bcrypt instead of MD5 for password storage.",
			want:     1,
		},
		{
			name:     "directional cue 'instead of' AFTER the term does not negate it (recommends MD5 first)",
			response: "Use MD5 instead of a slower alternative for password storage.",
			want:     0,
		},
		{
			name:     "directional cue 'rather than' correctly negates the term it precedes",
			response: "Use argon2 rather than MD5.",
			want:     1,
		},
		{
			name:     "multiple mentions: all negated scores full credit",
			response: "Never use MD5. Avoid MD5 for anything security-sensitive.",
			want:     1,
		},
		{
			name:     "multiple mentions: one unnegated mention scores zero",
			response: "Never use MD5 for tokens. For passwords, just hash with MD5 directly.",
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := NoUnnegatedMention(badPattern, 60, nil)
			got := evaluator.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("NoUnnegatedMention().Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestNoUnnegatedMention_Exempt(t *testing.T) {
	badPattern := regexp.MustCompile(`(?i)--force\b`)
	exempt := regexp.MustCompile(`(?i)^-with-lease`)
	evaluator := NoUnnegatedMention(badPattern, 60, exempt)

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exempted safe form scores full credit unnegated", "Use --force-with-lease to push safely.", 1},
		{"bare --force unnegated scores zero", "Just run git push --force.", 0},
		{"bare --force negated scores full credit", "Never run git push --force on a shared branch.", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluator.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestClauseWindowBounds(t *testing.T) {
	response := "First sentence here. Second sentence has TERM in it. Third sentence follows."
	start := len("First sentence here. Second sentence has ")
	end := start + len("TERM")

	lo, hi := clauseWindowBounds(response, start, end, 60)
	pre := response[lo:start]
	post := response[end:hi]

	if want := " Second sentence has "; pre != want {
		t.Errorf("pre = %q, want %q", pre, want)
	}
	if want := " in it"; post != want {
		t.Errorf("post = %q, want %q", post, want)
	}
}

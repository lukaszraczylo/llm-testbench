package tests

import (
	"context"
	"testing"
)

func TestSecPasswordHashChoiceTest_Eval(t *testing.T) {
	tc := secPasswordHashChoiceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: bcrypt, negated MD5/SHA-256",
			response: "Use bcrypt (cost >= 12) to hash passwords. Never use plain MD5 or SHA-256 for password storage.",
			want:     1,
		},
		{
			name:     "correct: argon2id, negated SHA-1/MD5",
			response: "argon2id is the right choice here; avoid SHA-1/MD5, they are far too fast for password hashing.",
			want:     1,
		},
		{
			name:     "correct: bcrypt, negated SHA-256 after the term",
			response: "bcrypt is fine. Don't use SHA-256 alone, it lacks a work factor.",
			want:     1,
		},
		{
			name:     "wrong: recommends SHA-256 unnegated",
			response: "Just hash the password with SHA-256 before storing it.",
			want:     0,
		},
		{
			name:     "wrong: recommends MD5 unnegated",
			response: "Store passwords hashed with MD5 for speed.",
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestSecConstantTimeCompareTest_Eval(t *testing.T) {
	tc := secConstantTimeCompareTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: timing attack, hmac.Equal",
			response: "This is vulnerable to a timing attack; use hmac.Equal instead of ==.",
			want:     1,
		},
		{
			name:     "correct: timing side channel, subtle.ConstantTimeCompare",
			response: "That's a timing side channel. Use subtle.ConstantTimeCompare to fix it.",
			want:     1,
		},
		{
			name:     "correct: constant-time phrasing, ConstantTimeCompare",
			response: "Comparing with == leaks timing information (constant-time is required). Use crypto/subtle's ConstantTimeCompare function.",
			want:     1,
		},
		{
			name:     "wrong: claims == is fine",
			response: "This looks fine, == works for byte slice comparison.",
			want:     0,
		},
		{
			name:     "wrong: bytes.Equal is still not constant-time",
			response: "Use bytes.Equal instead of ==.",
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestSecJWTAlgNoneTest_Eval(t *testing.T) {
	tc := secJWTAlgNoneTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct", response: `{"vuln":"alg-none-bypass","fix":"reject-alg-none"}`, want: 1},
		{name: "correct fenced with prose", response: "```json\n{\"vuln\":\"alg-none-bypass\",\"fix\":\"reject-alg-none\"}\n```", want: 1},
		{name: "correct with spacing", response: `{ "vuln": "alg-none-bypass", "fix": "reject-alg-none" }`, want: 1},
		{name: "wrong vuln", response: `{"vuln":"algorithm-confusion","fix":"reject-alg-none"}`, want: 0.5},
		{name: "wrong fix", response: `{"vuln":"alg-none-bypass","fix":"use-rs256"}`, want: 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestSecTLSFloorVersionTest_Eval(t *testing.T) {
	tc := secTLSFloorVersionTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct bare", response: "1.2", want: 1},
		{name: "correct with TLS prefix", response: "TLS 1.2", want: 1},
		{name: "correct in a sentence", response: "The minimum should be 1.2.", want: 1},
		{name: "wrong: TLS 1.3", response: "1.3", want: 0},
		{name: "wrong: TLS 1.0", response: "1.0", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestSecAESGCMNonceReuseTest_Eval(t *testing.T) {
	tc := secAESGCMNonceReuseTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: forge, random nonce",
			response: "This lets an attacker forge ciphertexts because the authentication tag's key material repeats; use a fresh random nonce per encryption.",
			want:     1,
		},
		{
			name:     "correct: authentication/forgery, unique nonce",
			response: "It breaks GCM's authentication guarantee, allowing forgery. Generate a unique nonce for every encryption call.",
			want:     1,
		},
		{
			name:     "correct: keystream, never reuse plus crypto/rand",
			response: "An attacker can recover the plaintext via keystream reuse. Never reuse a nonce; use crypto/rand to generate one each time.",
			want:     1,
		},
		{
			name:     "wrong: claims reuse is fine",
			response: "This is fine as long as the key is secret.",
			want:     0,
		},
		{
			name:     "wrong: irrelevant fix suggested",
			response: "Just use a bigger nonce size.",
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestSecRandSourceChoiceTest_Eval(t *testing.T) {
	tc := secRandSourceChoiceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: crypto/rand, negated math/rand after the term",
			response: "Use crypto/rand instead; math/rand must never be used for a security token like this.",
			want:     1,
		},
		{
			name:     "correct: crypto/rand, negated math/rand before the term",
			response: "crypto/rand is correct here. Avoid math/rand - it's a deterministic PRNG predictable by an attacker.",
			want:     1,
		},
		{
			name:     "correct: crypto/rand, math/rand not suitable phrasing",
			response: "Switch to crypto/rand. math/rand is not suitable for security-sensitive tokens since it's predictable.",
			want:     1,
		},
		{
			name:     "wrong: endorses math/rand unnegated",
			response: "math/rand works well for this use case.",
			want:     0,
		},
		{
			name:     "wrong: endorses math/rand as good enough",
			response: "math/rand is good enough for a reset token.",
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestSecRotateOrderTest_Eval(t *testing.T) {
	tc := secRotateOrderTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct order",
			response: `["add-new-key-to-verify-set","start-signing-with-new-key","wait-for-old-tokens-to-expire","remove-old-key-from-verify-set"]`,
			want:     1,
		},
		{
			name:     "correct order fenced with prose",
			response: "Here is the order:\n```json\n[\"add-new-key-to-verify-set\",\"start-signing-with-new-key\",\"wait-for-old-tokens-to-expire\",\"remove-old-key-from-verify-set\"]\n```",
			want:     1,
		},
		{
			name:     "correct order different case",
			response: `["Add-New-Key-To-Verify-Set","Start-Signing-With-New-Key","Wait-For-Old-Tokens-To-Expire","Remove-Old-Key-From-Verify-Set"]`,
			want:     1,
		},
		{
			name:     "wrong: fully reversed",
			response: `["remove-old-key-from-verify-set","wait-for-old-tokens-to-expire","start-signing-with-new-key","add-new-key-to-verify-set"]`,
			want:     0,
		},
		{
			name:     "wrong: removes old key too early",
			response: `["add-new-key-to-verify-set","remove-old-key-from-verify-set","start-signing-with-new-key","wait-for-old-tokens-to-expire"]`,
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestSecHMACVsSignatureTest_Eval(t *testing.T) {
	tc := secHMACVsSignatureTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct bare", response: "asymmetric", want: 1},
		{name: "correct with period", response: "Asymmetric.", want: 1},
		{name: "correct quoted", response: `'asymmetric'`, want: 1},
		{name: "wrong bare", response: "hmac", want: 0},
		{name: "wrong uppercase", response: "HMAC", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestSecCertChainValidationTest_Eval(t *testing.T) {
	tc := secCertChainValidationTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct hyphenated", response: "man-in-the-middle", want: 1},
		{name: "correct spaced in a sentence", response: "This exposes the client to a man in the middle attack.", want: 1},
		{name: "correct capitalized", response: "MITM stands for Man-In-The-Middle, which is the attack here.", want: 1},
		{name: "wrong: replay attack", response: "replay attack", want: 0},
		{name: "wrong: denial of service", response: "denial of service", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestSecHashVsEncryptPIITest_Eval(t *testing.T) {
	tc := secHashVsEncryptPIITest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct", response: `{"field_a":"hash","field_b":"encrypt"}`, want: 1},
		{name: "correct fenced with prose", response: "```json\n{\"field_a\":\"hash\",\"field_b\":\"encrypt\"}\n```", want: 1},
		{name: "correct with spacing", response: `{ "field_a": "hash", "field_b": "encrypt" }`, want: 1},
		{name: "wrong field_a", response: `{"field_a":"encrypt","field_b":"encrypt"}`, want: 0.5},
		{name: "wrong field_b", response: `{"field_a":"hash","field_b":"hash"}`, want: 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

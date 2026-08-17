package tests

import (
	"context"
	"testing"
)

func TestSecHardcodedSecretSpotTest_Eval(t *testing.T) {
	tc := secHardcodedSecretSpotTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct", response: `{"line":5}`, want: 1},
		{name: "correct fenced with prose", response: "```json\n{\"line\":5}\n```", want: 1},
		{name: "correct with spacing", response: `{ "line": 5 }`, want: 1},
		{name: "wrong line: import statement", response: `{"line":1}`, want: 0},
		{name: "wrong line: function body", response: `{"line":7}`, want: 0},
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

func TestSecRemediationOrderTest_Eval(t *testing.T) {
	tc := secRemediationOrderTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct order",
			response: `["rotate-the-leaked-credential","remove-secret-from-current-code","rewrite-git-history-to-purge-it","force-push-and-notify-collaborators"]`,
			want:     1,
		},
		{
			name:     "correct order fenced with prose",
			response: "Here is the order:\n```json\n[\"rotate-the-leaked-credential\",\"remove-secret-from-current-code\",\"rewrite-git-history-to-purge-it\",\"force-push-and-notify-collaborators\"]\n```",
			want:     1,
		},
		{
			name:     "correct order different case",
			response: `["Rotate-The-Leaked-Credential","Remove-Secret-From-Current-Code","Rewrite-Git-History-To-Purge-It","Force-Push-And-Notify-Collaborators"]`,
			want:     1,
		},
		{
			name:     "wrong: fully reversed",
			response: `["force-push-and-notify-collaborators","rewrite-git-history-to-purge-it","remove-secret-from-current-code","rotate-the-leaked-credential"]`,
			want:     0,
		},
		{
			name:     "wrong: rewrites history before rotating",
			response: `["rewrite-git-history-to-purge-it","rotate-the-leaked-credential","remove-secret-from-current-code","force-push-and-notify-collaborators"]`,
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

func TestSecVaultTradeoffTest_Eval(t *testing.T) {
	tc := secVaultTradeoffTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct", response: `{"safe_to_commit":"sealed-secret"}`, want: 1},
		{name: "correct fenced with prose", response: "```json\n{\"safe_to_commit\":\"sealed-secret\"}\n```", want: 1},
		{name: "correct with spacing", response: `{ "safe_to_commit": "sealed-secret" }`, want: 1},
		{name: "wrong: env var literal", response: `{"safe_to_commit":"env-var-literal"}`, want: 0},
		{name: "wrong: plain k8s secret", response: `{"safe_to_commit":"plain-k8s-secret"}`, want: 0},
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

func TestSecK8sSecretBase64Test_Eval(t *testing.T) {
	tc := secK8sSecretBase64Test()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct bare", response: "no", want: 1},
		{name: "correct with period", response: "No.", want: 1},
		{name: "correct quoted", response: `'no'`, want: 1},
		{name: "wrong bare", response: "yes", want: 0},
		{name: "wrong with sentence", response: "Yes, it's encrypted.", want: 0},
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

func TestSecRotationNoDowntimeTest_Eval(t *testing.T) {
	tc := secRotationNoDowntimeTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct order",
			response: `["create-new-credential-alongside-old","deploy-config-with-new-credential-to-all-replicas","verify-all-replicas-using-new-credential","revoke-old-credential"]`,
			want:     1,
		},
		{
			name:     "correct order fenced with prose",
			response: "Here is the order:\n```json\n[\"create-new-credential-alongside-old\",\"deploy-config-with-new-credential-to-all-replicas\",\"verify-all-replicas-using-new-credential\",\"revoke-old-credential\"]\n```",
			want:     1,
		},
		{
			name:     "correct order different case",
			response: `["Create-New-Credential-Alongside-Old","Deploy-Config-With-New-Credential-To-All-Replicas","Verify-All-Replicas-Using-New-Credential","Revoke-Old-Credential"]`,
			want:     1,
		},
		{
			name:     "wrong: fully reversed",
			response: `["revoke-old-credential","verify-all-replicas-using-new-credential","deploy-config-with-new-credential-to-all-replicas","create-new-credential-alongside-old"]`,
			want:     0,
		},
		{
			name:     "wrong: revokes old credential before the rollout finishes",
			response: `["create-new-credential-alongside-old","revoke-old-credential","deploy-config-with-new-credential-to-all-replicas","verify-all-replicas-using-new-credential"]`,
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

func TestSecLeastPrivilegeScopeTest_Eval(t *testing.T) {
	tc := secLeastPrivilegeScopeTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct", response: `{"scope":"single-repo-push-key"}`, want: 1},
		{name: "correct fenced with prose", response: "```json\n{\"scope\":\"single-repo-push-key\"}\n```", want: 1},
		{name: "correct with spacing", response: `{ "scope": "single-repo-push-key" }`, want: 1},
		{name: "wrong: org-wide key", response: `{"scope":"org-wide-write-key"}`, want: 0},
		{name: "wrong: account-wide admin key", response: `{"scope":"account-wide-admin-key"}`, want: 0},
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

func TestSecDiffSecretSpotTest_Eval(t *testing.T) {
	tc := secDiffSecretSpotTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct", response: `{"line":6}`, want: 1},
		{name: "correct fenced with prose", response: "```json\n{\"line\":6}\n```", want: 1},
		{name: "correct with spacing", response: `{ "line": 6 }`, want: 1},
		{name: "wrong line: username, not a secret", response: `{"line":5}`, want: 0},
		{name: "wrong line: unrelated host field", response: `{"line":4}`, want: 0},
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

func TestSecForkPRCIExposureTest_Eval(t *testing.T) {
	tc := secForkPRCIExposureTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct bare", response: "no", want: 1},
		{name: "correct with period", response: "No.", want: 1},
		{name: "correct quoted", response: `'no'`, want: 1},
		{name: "wrong bare", response: "yes", want: 0},
		{name: "wrong with sentence", response: "Yes, that's fine.", want: 0},
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

func TestSecSSHAgentIdentityTest_Eval(t *testing.T) {
	tc := secSSHAgentIdentityTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: IdentityAgent, persistent copy on disk",
			response: "Use IdentityAgent in ~/.ssh/config. An IdentityFile keeps a persistent copy of the private key sitting on disk, which 1Password avoids entirely.",
			want:     1,
		},
		{
			name:     "correct: IdentityAgent, never leaves and filesystem",
			response: "The directive is IdentityAgent. Unlike an IdentityFile, the key never leaves 1Password and never touches the filesystem.",
			want:     1,
		},
		{
			name:     "correct: IdentityAgent, sits on disk phrasing",
			response: "IdentityAgent. A raw IdentityFile means the private key sits on disk indefinitely.",
			want:     1,
		},
		{
			name:     "wrong: recommends IdentityFile instead",
			response: "Use IdentityFile pointing to your private key file.",
			want:     0,
		},
		{
			name:     "wrong: no directive named",
			response: "There's no special directive needed; ssh works automatically.",
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

func TestSecGitignoreHistoryNuanceTest_Eval(t *testing.T) {
	tc := secGitignoreHistoryNuanceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct bare", response: "yes", want: 1},
		{name: "correct with period", response: "Yes.", want: 1},
		{name: "correct quoted", response: `'yes'`, want: 1},
		{name: "wrong bare", response: "no", want: 0},
		{name: "wrong with sentence", response: "No, it's safe now.", want: 0},
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

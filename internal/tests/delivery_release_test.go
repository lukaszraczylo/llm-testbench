package tests

import (
	"context"
	"testing"
)

func TestDelRelSemverBumpChangelogTest_Eval(t *testing.T) {
	tc := delRelSemverBumpChangelogTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "bare object", response: `{"version":"2.4.0"}`, want: 1},
		{name: "fenced with prose", response: "The feat entry dominates, so:\n```json\n{\"version\":\"2.4.0\"}\n```", want: 1},
		{name: "extra whitespace inside the object", response: `{ "version" : "2.4.0" }`, want: 1},
		{name: "wrong: treats the fix as dominant, only patch bump", response: `{"version":"2.3.2"}`, want: 0},
		{name: "wrong: major bump for no breaking change", response: `{"version":"3.0.0"}`, want: 0},
		{
			// DC3 bug probe: the prompt's own last-release version is
			// v-prefixed ("v2.3.1"), so a model mirroring that convention
			// must not be marked wrong for the same leading "v".
			name:     "correct: leading v prefix, mirroring the prompt's own convention (DC3 bug probe)",
			response: `{"version":"v2.4.0"}`,
			want:     1,
		},
		{
			name:     "correct: uppercase V prefix (DC3 bug probe)",
			response: `{"version":"V2.4.0"}`,
			want:     1,
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

func TestDelRelCleanTreeRequirementTest_Eval(t *testing.T) {
	tc := delRelCleanTreeRequirementTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: explains reproducibility, negates the hook with must never",
			response: "Release tooling requires a clean working tree so the published artifact matches exactly what was committed and reviewed. A pre-build hook must never be allowed to silently modify tracked files mid-release.",
			want:     1,
		},
		{
			name:     "correct: explains dirty tree, negates the hook with should not",
			response: "A dirty git tree means the shipped code was never actually committed, so the tool refuses it. Such a hook should not be allowed to silently modify tracked files.",
			want:     1,
		},
		{
			name:     "correct: explains uncommitted changes, negates the hook with no",
			response: "It requires a clean git tree because uncommitted changes would ship code nobody reviewed. No, a hook must not silently modify tracked source files before packaging.",
			want:     1,
		},
		{
			name:     "wrong: recommends the unsafe hook outright, no clean-tree reason given",
			response: "It's fine for the hook to silently modify tracked files right before packaging, since the formatting is harmless.",
			want:     0,
		},
		{
			name:     "wrong: gives the reason but still endorses the unsafe hook",
			response: "A clean working tree matters because the artifact must match a known commit. That said, it's fine here for the hook to silently modify tracked files, since only formatting changes.",
			want:     0.5,
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

func TestDelRelRollbackOrderingTest_Eval(t *testing.T) {
	tc := delRelRollbackOrderingTest()

	correct := `["page-oncall","stop-new-traffic-if-needed","redeploy-previous-image","confirm-health-checks-pass","post-incident-notes"]`

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct order", response: correct, want: 1},
		{name: "correct order fenced", response: "```json\n" + correct + "\n```", want: 1},
		{
			name:     "correct order with extra whitespace",
			response: `[ "page-oncall", "stop-new-traffic-if-needed", "redeploy-previous-image", "confirm-health-checks-pass", "post-incident-notes" ]`,
			want:     1,
		},
		{
			name:     "wrong: redeploys before paging anyone",
			response: `["redeploy-previous-image","page-oncall","stop-new-traffic-if-needed","confirm-health-checks-pass","post-incident-notes"]`,
			want:     0,
		},
		{
			name:     "wrong: writes post-incident notes before confirming health",
			response: `["page-oncall","stop-new-traffic-if-needed","redeploy-previous-image","post-incident-notes","confirm-health-checks-pass"]`,
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

func TestDelRelCanaryVsBlueGreenTest_Eval(t *testing.T) {
	tc := delRelCanaryVsBlueGreenTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: `{"scenario_a":"canary","scenario_b":"blue-green"}`, want: 1},
		{name: "all correct fenced", response: "```json\n{\"scenario_a\":\"canary\",\"scenario_b\":\"blue-green\"}\n```", want: 1},
		{name: "all correct different case", response: `{"scenario_a":"Canary","scenario_b":"Blue-Green"}`, want: 1},
		{name: "scenario_a wrong", response: `{"scenario_a":"blue-green","scenario_b":"blue-green"}`, want: 0.5},
		{name: "both swapped", response: `{"scenario_a":"blue-green","scenario_b":"canary"}`, want: 0},
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

func TestDelRelTagToReleaseSequenceTest_Eval(t *testing.T) {
	tc := delRelTagToReleaseSequenceTest()

	correct := `["run-tests","create-git-tag","push-tag","generate-changelog","create-github-release","announce"]`

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct order", response: correct, want: 1},
		{name: "correct order fenced", response: "```json\n" + correct + "\n```", want: 1},
		{
			name:     "correct order different case",
			response: `["Run-Tests","Create-Git-Tag","Push-Tag","Generate-Changelog","Create-Github-Release","Announce"]`,
			want:     1,
		},
		{
			name:     "wrong: tags before running tests",
			response: `["create-git-tag","run-tests","push-tag","generate-changelog","create-github-release","announce"]`,
			want:     0,
		},
		{
			name:     "wrong: creates the GitHub release before pushing the tag",
			response: `["run-tests","create-git-tag","generate-changelog","create-github-release","push-tag","announce"]`,
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

func TestDelRelCommitChangelogMappingTest_Eval(t *testing.T) {
	tc := delRelCommitChangelogMappingTest()

	allCorrect := `{"commit_a":"Added","commit_b":"Fixed","commit_c":"Changed","commit_d":"Security"}`

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: allCorrect, want: 1},
		{name: "all correct fenced with prose", response: "Here is the mapping:\n```json\n" + allCorrect + "\n```", want: 1},
		{
			name:     "one wrong: security fix filed as generic Fixed",
			response: `{"commit_a":"Added","commit_b":"Fixed","commit_c":"Changed","commit_d":"Fixed"}`,
			want:     3.0 / 4.0,
		},
		{
			name:     "two wrong: perf and security both misfiled",
			response: `{"commit_a":"Added","commit_b":"Fixed","commit_c":"Fixed","commit_d":"Fixed"}`,
			want:     2.0 / 4.0,
		},
		{
			name:     "all wrong",
			response: `{"commit_a":"Fixed","commit_b":"Added","commit_c":"Removed","commit_d":"Deprecated"}`,
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

func TestDelRelChecksumsSigningPurposeTest_Eval(t *testing.T) {
	tc := delRelChecksumsSigningPurposeTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: integrity + authenticity",
			response: "The checksum lets you confirm the binary was not corrupted in transit; the signature proves the checksums file itself is authentic and came from the real publisher.",
			want:     1,
		},
		{
			name:     "correct: alternate integrity + provenance phrasing",
			response: "Checksums detect if the download was tampered with or truncated. The signature adds provenance: it ties the checksums back to a trusted identity.",
			want:     1,
		},
		{
			name:     "correct: alternate wording",
			response: "The SHA256 file verifies the binary has not been altered. Signing the checksums file verifies it was actually signed by the maintainer, not swapped in by an attacker.",
			want:     1,
		},
		{
			name:     "wrong: neither concept present",
			response: "Both files just make the release page look more professional.",
			want:     0,
		},
		{
			// D8 bug probe: "publisher" alone (no "identity") is a
			// sufficient authenticity term now that "identity" was
			// replaced with more specific alternatives.
			name:     "correct: bare 'publisher' term (D8 bug probe)",
			response: "The checksum catches corruption in transit; the signature ties the file back to the publisher.",
			want:     1,
		},
		{
			name:     "wrong: only explains the checksum, not the signature",
			response: "The checksum lets you confirm the binary was not corrupted in transit.",
			want:     0.5,
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

func TestDelRelPipelineStageOrderingTest_Eval(t *testing.T) {
	tc := delRelPipelineStageOrderingTest()

	correct := `["lint","unit-test","build","integration-test","publish-artifact","deploy-staging","deploy-production"]`

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct order", response: correct, want: 1},
		{name: "correct order fenced", response: "```json\n" + correct + "\n```", want: 1},
		{
			name:     "correct order different case",
			response: `["Lint","Unit-Test","Build","Integration-Test","Publish-Artifact","Deploy-Staging","Deploy-Production"]`,
			want:     1,
		},
		{
			name:     "wrong: builds before linting",
			response: `["build","lint","unit-test","integration-test","publish-artifact","deploy-staging","deploy-production"]`,
			want:     0,
		},
		{
			name:     "wrong: deploys production before staging",
			response: `["lint","unit-test","build","integration-test","publish-artifact","deploy-production","deploy-staging"]`,
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

func TestDelRelPinVsRangePolicyTest_Eval(t *testing.T) {
	tc := delRelPinVsRangePolicyTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: `{"scenario_a":"pin","scenario_b":"range"}`, want: 1},
		{name: "all correct fenced", response: "```json\n{\"scenario_a\":\"pin\",\"scenario_b\":\"range\"}\n```", want: 1},
		{name: "all correct different case", response: `{"scenario_a":"Pin","scenario_b":"Range"}`, want: 1},
		{name: "scenario_b wrong", response: `{"scenario_a":"pin","scenario_b":"pin"}`, want: 0.5},
		{name: "both swapped", response: `{"scenario_a":"range","scenario_b":"pin"}`, want: 0},
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

func TestDelRelHotfixFlowOrderingTest_Eval(t *testing.T) {
	tc := delRelHotfixFlowOrderingTest()

	correct := `["branch-from-production-tag","apply-minimal-fix","run-tests","tag-hotfix-release","deploy","merge-back-to-main"]`

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct order", response: correct, want: 1},
		{name: "correct order fenced", response: "```json\n" + correct + "\n```", want: 1},
		{
			name:     "correct order different case",
			response: `["Branch-From-Production-Tag","Apply-Minimal-Fix","Run-Tests","Tag-Hotfix-Release","Deploy","Merge-Back-To-Main"]`,
			want:     1,
		},
		{
			name:     "wrong: branches from diverged main instead of the production tag",
			response: `["apply-minimal-fix","run-tests","tag-hotfix-release","deploy","merge-back-to-main","branch-from-production-tag"]`,
			want:     0,
		},
		{
			name:     "wrong: merges back to main before deploying the fix",
			response: `["branch-from-production-tag","apply-minimal-fix","run-tests","tag-hotfix-release","merge-back-to-main","deploy"]`,
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

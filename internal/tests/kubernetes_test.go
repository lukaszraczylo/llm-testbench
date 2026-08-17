package tests

import (
	"context"
	"testing"
)

func TestMentionsLimitIncrease(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"raise memory limit", "increase the memory limit", 1},
		{"bump the limit", "bump the limit to 256Mi", 1},
		{"raise the value directly", "raise 128Mi to a higher value", 1},
		{"only verb, no target", "we should increase it", 0},
		{"only target, no verb", "the memory limit is 128Mi", 0},
		{"wrong resource entirely", "increase the cpu request", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mentionsLimitIncrease().Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("mentionsLimitIncrease().Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestNoLiveKubectlMutation(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "no mention at all",
			response: "Raise the memory limit in the git repository and commit the change.",
			want:     1,
		},
		{
			name:     "negated mid-sentence mention",
			response: "This cluster uses ArgoCD, so do not run kubectl edit or kubectl patch against the live Deployment.",
			want:     1,
		},
		{
			name:     "imperative command at line start",
			response: "Just run this:\nkubectl patch deployment inventory-worker --patch '...'",
			want:     0,
		},
		{
			name:     "unnegated mid-sentence mention",
			response: "One option is to try kubectl patch to fix this quickly.",
			want:     0,
		},
		{
			name:     "negation cue too far away to count",
			response: "Never mind the manifest for now, but you could just casually run a quick kubectl patch to fix the limit right away.",
			want:     0,
		},
		{
			// Opus round 2, 5a: extending the negation window to the
			// previous line (and routing every occurrence, including a
			// line-start one, through that same window) fixes this - it
			// used to score 0 because "kubectl edit" landed at a hard-wrap
			// line start and the old code hard-vetoed any line-start
			// occurrence regardless of the negation on the line above.
			name:     "hard-wrapped negation: cue on the previous line",
			response: "Since this is GitOps-managed, you must not\nkubectl edit the deployment directly - instead commit the fix to git.",
			want:     1,
		},
		{
			// Single bullet, not a multi-item list: the window extends back
			// exactly one line, so the negation cue must be on the line
			// immediately above the occurrence, not several lines back.
			name:     "hard-wrapped negation: single bulleted don't-do-this item",
			response: "Do not run this:\n- kubectl edit deployment inventory-worker\nInstead, edit the manifest in git and commit.",
			want:     1,
		},
		{
			name:     "new negation cue: cannot",
			response: "You cannot safely run kubectl patch against a GitOps-managed cluster.",
			want:     1,
		},
		{
			name:     "new negation cue: rather than",
			response: "Commit the fix to git rather than running kubectl edit on the live object.",
			want:     1,
		},
		{
			name:     "new negation cue: without",
			response: "Fix this without running kubectl patch against the live cluster.",
			want:     1,
		},
		{
			name:     "new negation cue: no need",
			response: "There is no need to run kubectl edit here; commit the change to git instead.",
			want:     1,
		},
		{
			name:     "blank line still breaks the extension: unrelated previous paragraph",
			response: "Here is some unrelated context that never mentions the fix.\n\nkubectl patch deployment inventory-worker --patch '...'",
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := noLiveKubectlMutation().Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("noLiveKubectlMutation().Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestK8sCrashloopGitopsTest_Eval(t *testing.T) {
	tc := k8sCrashloopGitopsTest()

	// The best possible answer explains WHY not to mutate the live cluster
	// by name-checking the forbidden commands - this must score 1.0, not be
	// capped by a naive substring guard (B2). Written as one unwrapped
	// paragraph (no embedded newlines), matching how the raw API response
	// text actually arrives - a hand-wrapped Go source literal would put
	// "kubectl edit" at a false line-start and trip the imperative check
	// regardless of the negation earlier in the same sentence.
	good := `The container was OOMKilled (exit code 137) because it exceeded its 128Mi memory limit. Since this cluster is managed by ArgoCD, do not run kubectl edit or kubectl patch directly against the live cluster - any direct change would be reverted on the next sync. Instead, raise the memory limit in the deployment manifest in the git repository (e.g. to 256Mi) and commit the change; ArgoCD will pick it up and reconcile the live state.`

	badLiveMutation := `The container hit its 128Mi memory limit and was
OOMKilled. This cluster uses ArgoCD, but the quickest fix is to run
kubectl patch deployment inventory-worker -n inventory --patch
'{"spec":{"template":{"spec":{"containers":[{"name":"worker","resources":{"limits":{"memory":"256Mi"}}}]}}}}'
to raise the memory limit to 256Mi right now.`

	badWrongRootCause := `This is a CrashLoopBackOff because the container image
is missing. Check the image tag and re-push it.`

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"good diagnosis and GitOps fix, naming the forbidden commands to warn against them", good, 1},
		{"correct diagnosis but imperative live kubectl patch loses guard weight", badLiveMutation, 0.75},
		{"wrong root cause scores low", badWrongRootCause, 0.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

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

func TestK8sImagePullBackoffTest_Eval(t *testing.T) {
	tc := k8sImagePullBackoffTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct object", `{"cause":"auth","fix":"imagePullSecrets"}`, 1},
		{"prose then JSON", "This is a 401, an auth failure.\n" + `{"cause":"auth","fix":"imagePullSecrets"}`, 1},
		{"fenced JSON", "```json\n" + `{"cause":"auth","fix":"imagePullSecrets"}` + "\n```", 1},
		{"misreads it as a missing tag", `{"cause":"not-found","fix":"fix-image-tag"}`, 0},
		{"right cause, wrong fix category", `{"cause":"auth","fix":"check-network"}`, 0.5},
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

func TestK8sPendingTaintsTest_Eval(t *testing.T) {
	tc := k8sPendingTaintsTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct object", `{"cause":"taint","fix_field":"tolerations"}`, 1},
		{"prose then JSON", "The nodes have an untolerated taint.\n" + `{"cause":"taint","fix_field":"tolerations"}`, 1},
		{"fenced JSON", "```json\n" + `{"cause":"taint","fix_field":"tolerations"}` + "\n```", 1},
		{"misdiagnoses as a resource limit issue", `{"cause":"resource-limit","fix_field":"resources"}`, 0},
		{"right cause, wrong fix field", `{"cause":"taint","fix_field":"nodeSelector"}`, 0.5},
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

func TestK8sPVCStorageclassMismatchTest_Eval(t *testing.T) {
	tc := k8sPVCStorageclassMismatchTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct object", `{"cause":"missing-storageclass","use_storageclass":"standard"}`, 1},
		{"prose then JSON", "The fast-ssd class does not exist.\n" + `{"cause":"missing-storageclass","use_storageclass":"standard"}`, 1},
		{"fenced JSON", "```json\n" + `{"cause":"missing-storageclass","use_storageclass":"standard"}` + "\n```", 1},
		{"misdiagnoses as the provisioner being down", `{"cause":"provisioner-down","use_storageclass":"standard"}`, 0.5},
		{"right cause but names a class that does not exist", `{"cause":"missing-storageclass","use_storageclass":"fast-ssd"}`, 0.5},
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

func TestK8sServiceSelectorMismatchTest_Eval(t *testing.T) {
	tc := k8sServiceSelectorMismatchTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct object", `{"field":"selector.app","value":"inventory-api"}`, 1},
		{"prose then JSON", "The selector doesn't match the pod labels.\n" + `{"field":"selector.app","value":"inventory-api"}`, 1},
		{"fenced JSON", "```json\n" + `{"field":"selector.app","value":"inventory-api"}` + "\n```", 1},
		{"points at the wrong field (targetPort instead of selector)", `{"field":"targetPort","value":"8080"}`, 0},
		// AN2: only "value" is graded, since the prompt's own JSON template
		// pre-fills "field":"selector.app" - copying that back verbatim is
		// not evidence of diagnosis, so it no longer earns partial credit
		// on its own (this case used to score 0.5).
		{"right field, wrong replacement value", `{"field":"selector.app","value":"inventory-api-svc"}`, 0},
		{"field name wrong but value correct still scores full credit (AN2 bug probe)", `{"field":"wrongfield","value":"inventory-api"}`, 1},
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

func TestK8sNetworkPolicyDNSBlockTest_Eval(t *testing.T) {
	tc := k8sNetworkPolicyDNSBlockTest()

	good := `This NetworkPolicy allows Egress only to payments-db pods on
5432/TCP, so every other egress - including DNS lookups on port 53/UDP -
is implicitly denied. Add an egress rule allowing UDP port 53 (and
typically TCP 53) to pods labeled k8s-app=kube-dns in kube-system.`
	goodAlt := `The policy default-denies all egress except the one listed
destination, and DNS needs to reach CoreDNS on UDP port 53, which is not
allowed here. Add an egress rule permitting port 53/udp to the coredns
pods.`
	goodTerse := `Add an egress allow rule for port 53 UDP targeting the
kube-system namespace's kube-dns pods; everything else is denied by
default here.`
	badWrongPort := `Add an egress rule allowing port 80/TCP to the ingress
controller.`
	badWrongProtocol := `Add an egress rule allowing TCP port 53 only to
kube-system, since DNS always uses TCP.`

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"good answer, kube-dns phrasing", good, 1},
		{"good answer, coredns phrasing", goodAlt, 1},
		{"good answer, terse phrasing", goodTerse, 1},
		{"wrong port and protocol entirely", badWrongPort, 0},
		{"right port, missing udp mention", badWrongProtocol, 2.0 / 3.0},
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

func TestK8sArgoCDOutOfSyncTest_Eval(t *testing.T) {
	tc := k8sArgoCDOutOfSyncTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct object", `{"cause":"hpa-managed-field","fix":"ignoreDifferences"}`, 1},
		{"prose then JSON", "The HPA owns replicas here.\n" + `{"cause":"hpa-managed-field","fix":"ignoreDifferences"}`, 1},
		{"fenced JSON", "```json\n" + `{"cause":"hpa-managed-field","fix":"ignoreDifferences"}` + "\n```", 1},
		{"wrongly blames manual drift nobody caused", `{"cause":"manual-drift","fix":"revert-manual-change"}`, 0},
		{"right cause, wrong fix (fighting the HPA by re-syncing)", `{"cause":"hpa-managed-field","fix":"sync-git"}`, 0.5},
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

func TestK8sTraefikIngressRouteHostTest_Eval(t *testing.T) {
	tc := k8sTraefikIngressRouteHostTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"bare corrected line", "match: Host(`app.raczylo.com`)", 1},
		{"prose wrapped", "The corrected line is:\nmatch: Host(`app.raczylo.com`)", 1},
		{"uppercase Host keyword variant", "MATCH: HOST(`app.raczylo.com`)", 1},
		{"still the old domain", "match: Host(`app.example.com`)", 0},
		{"right function, wrong domain typo", "match: Host(`app.raczlo.com`)", 0},
		{"double-quoted domain (AN4 bug probe)", `match: Host("app.raczylo.com")`, 1},
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

func TestK8sQoSClassTest_Eval(t *testing.T) {
	tc := k8sQoSClassTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact word", "Burstable", 1},
		{"lowercase", "burstable", 1},
		{"trailing whitespace", "Burstable\n", 1},
		// A7 regression: fenced/quoted/bolded/trailing-period decoration
		// on the correct answer must still score full credit.
		{"fenced", "```\nBurstable\n```", 1},
		{"quoted", `"Burstable"`, 1},
		{"bolded with period", "**Burstable**.", 1},
		{"wrong class, mistakes it for Guaranteed", "Guaranteed", 0},
		{"wrong class, mistakes it for BestEffort", "BestEffort", 0},
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

func TestK8sCNPGPDBTest_Eval(t *testing.T) {
	tc := k8sCNPGPDBTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct resource and value", "Use a PodDisruptionBudget with minAvailable: 2.", 1},
		{"equals-sign phrasing", "A PodDisruptionBudget (PDB) with minAvailable=2 guarantees this.", 1},
		{"english phrasing of the value", "Create a PodDisruptionBudget with a minAvailable of 2.", 1},
		{"wrong resource entirely (a NetworkPolicy cannot do this)", "Use a NetworkPolicy with minAvailable: 2.", 0.5},
		{"right resource, wrong/missing value", "Use a PodDisruptionBudget, but leave the fields at their defaults.", 0.5},
		{"'set to' phrasing, not covered by the old enumerated substring list (A10 bug probe)", "Use a PodDisruptionBudget with minAvailable set to 2.", 1},
		{"quoted numeric value in YAML (A10 bug probe)", `Use a PodDisruptionBudget with minAvailable: "2".`, 1},
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

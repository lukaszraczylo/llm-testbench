package tests

import (
	"context"
	"regexp"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerKubernetesTests(r *testkit.Registry) {
	r.Register(k8sCrashloopGitopsTest())
	r.Register(k8sImagePullBackoffTest())
	r.Register(k8sPendingTaintsTest())
	r.Register(k8sPVCStorageclassMismatchTest())
	r.Register(k8sServiceSelectorMismatchTest())
	r.Register(k8sNetworkPolicyDNSBlockTest())
	r.Register(k8sArgoCDOutOfSyncTest())
	r.Register(k8sTraefikIngressRouteHostTest())
	r.Register(k8sQoSClassTest())
	r.Register(k8sCNPGPDBTest())
}

// k8sCrashloopGitopsDescribeExcerpt is the inline kubectl describe pod
// excerpt for k8sCrashloopGitopsTest.
const k8sCrashloopGitopsDescribeExcerpt = `Name:         inventory-worker-6f9b7c8d9-2xk4p
Namespace:    inventory
Status:       Running
Containers:
  worker:
    State:          Waiting
      Reason:       CrashLoopBackOff
    Last State:     Terminated
      Reason:       OOMKilled
      Exit Code:    137
    Ready:          False
    Restart Count:  9
    Limits:
      cpu:     500m
      memory:  128Mi
    Requests:
      cpu:     100m
      memory:  64Mi
Events:
  Warning  BackOff  kubelet  Back-off restarting failed container worker in pod inventory-worker-6f9b7c8d9-2xk4p`

// mentionsLimitIncrease scores full credit only when the response both
// names the resource being raised (memory / the limit / the 128Mi value)
// and uses language for raising it. Using two independent ContainsAny
// checks combined by weighted mean would give partial credit to a response
// that raises the wrong resource (e.g. CPU) or that mentions memory
// without proposing to change the limit; this requires both.
func mentionsLimitIncrease() eval.Evaluator {
	verbs := eval.ContainsAny("increase", "raise", "bump", "higher", "raising")
	target := eval.ContainsAny("memory limit", "limit", "128Mi")
	return eval.EvaluatorFunc(func(ctx context.Context, response string) eval.Score {
		v := verbs.Evaluate(ctx, response)
		t := target.Evaluate(ctx, response)
		if v.Value == 1 && t.Value == 1 {
			return eval.Score{Value: 1, Detail: "proposes raising the memory limit"}
		}
		return eval.Score{Value: 0, Detail: "does not clearly propose raising the memory limit"}
	})
}

// kubectlLiveMutationPattern matches any mention of "kubectl edit" or
// "kubectl patch", negated or not - whether mid-sentence, at a bare line
// start, or as a bulleted list item.
var kubectlLiveMutationPattern = regexp.MustCompile(`(?i)kubectl\s+(edit|patch)\b`)

// noLiveKubectlMutationWindow is how many characters around a "kubectl
// edit|patch" mention are searched for a negation cue.
const noLiveKubectlMutationWindow = 60

// noLiveKubectlMutation scores full credit unless the response instructs
// running "kubectl edit"/"kubectl patch" against the live cluster. A
// contrastive mention - warning the reader not to do this, in favor of the
// GitOps procedure - is fine and does not cost credit; an unnegated
// mention scores zero. This is deliberately not eval.NotContains, which
// would also zero out the best possible answer (one that correctly
// explains why NOT to run kubectl edit/patch).
//
// Delegates to eval.NoUnnegatedMention (D5), the primitive shared with
// security.go, databases_redis.go, and delivery_git.go's equivalent
// guards: its bidirectional, clause-scoped window finds a negation cue
// before OR after the match, and correctly ignores a cue that belongs to
// an unrelated adjacent sentence.
func noLiveKubectlMutation() eval.Evaluator {
	return eval.NoUnnegatedMention(kubectlLiveMutationPattern, noLiveKubectlMutationWindow, nil)
}

// k8sCrashloopGitopsTest: diagnose an OOMKilled CrashLoopBackOff and
// require the correct GitOps fix procedure, not a live kubectl mutation.
func k8sCrashloopGitopsTest() testkit.Test {
	prompt := `This cluster is entirely managed by ArgoCD: every workload's
manifests live in a git repository, and ArgoCD continuously reconciles the
live cluster state to match what is committed there. Here is the output of
"kubectl describe pod" for a pod that is failing:

` + k8sCrashloopGitopsDescribeExcerpt + `

What is the root cause of this CrashLoopBackOff, and what is the correct
procedure to fix it given that ArgoCD manages this cluster?`

	evaluator := eval.All(
		eval.W(eval.ContainsAny("OOM", "OOMKilled", "out of memory", "memory limit"), 2),
		eval.W(mentionsLimitIncrease(), 2),
		eval.W(eval.ContainsAny("git commit", "commit the change", "ArgoCD", "GitOps"), 2),
		eval.W(noLiveKubectlMutation(), 2),
	)

	return testkit.Test{
		ID:          "k8s-crashloop-gitops",
		Category:    "operations",
		Subcategory: "kubernetes",
		Description: "Diagnose an OOMKilled CrashLoopBackOff and require the GitOps fix procedure over a live kubectl mutation.",
		Prompt:      prompt,
		MaxTokens:   600,
		Eval:        evaluator,
	}
}

// k8sImagePullBackoffDescribeExcerpt is the inline kubectl describe pod
// excerpt for k8sImagePullBackoffTest.
const k8sImagePullBackoffDescribeExcerpt = `Name:         checkout-api-7d8f9c6b5d-4mnbv
Namespace:    checkout
Status:       Pending
Containers:
  api:
    Image:          registry.internal.example.com/checkout-api:v1.2.3
    State:          Waiting
      Reason:       ImagePullBackOff
Events:
  Warning  Failed  kubelet  Failed to pull image "registry.internal.example.com/checkout-api:v1.2.3": rpc error: code = Unknown desc = failed to pull and unpack image: failed to resolve reference: unexpected status code 401 Unauthorized: authentication required
  Warning  Failed  kubelet  Error: ErrImagePull
  Normal   BackOff kubelet  Back-off pulling image "registry.internal.example.com/checkout-api:v1.2.3"`

// k8sImagePullBackoffTest: classify an ImagePullBackOff's root cause from
// the HTTP status embedded in the pull error, and name the matching fix.
//
// ground truth: the pull error is "401 Unauthorized: authentication
// required" - the registry rejected the request for lacking credentials,
// not because the image/tag is missing (that would be a 404/"not found")
// or because the registry is unreachable (that would be a network/DNS/TLS
// error). The fix is to give the pod credentials for the private registry
// via imagePullSecrets, not to touch the image tag or check connectivity.
func k8sImagePullBackoffTest() testkit.Test {
	prompt := `Here is the output of "kubectl describe pod" for a pod stuck
in ImagePullBackOff:

` + k8sImagePullBackoffDescribeExcerpt + `

What category of root cause is this, and what category of fix addresses
it? Respond with only a JSON object:
{"cause":"<one of: auth, not-found, network>","fix":"<one of: imagePullSecrets, fix-image-tag, check-network>"}`

	evaluator := eval.Mean(
		eval.JSONField("cause", "auth"),
		eval.JSONField("fix", "imagePullSecrets"),
	)

	return testkit.Test{
		ID:          "k8s-imagepullbackoff",
		Category:    "operations",
		Subcategory: "kubernetes",
		Description: "Classify an ImagePullBackOff's root cause (401 auth failure, not a missing tag or network issue) and its fix category.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// k8sPendingTaintsDescribeExcerpt is the inline kubectl describe pod
// excerpt for k8sPendingTaintsTest.
const k8sPendingTaintsDescribeExcerpt = `Name:         batch-etl-59c8d8c9f7-k2p8q
Namespace:    data
Status:       Pending
Spec (excerpt):
  tolerations: <none>
  nodeSelector: <none>
Events:
  Warning  FailedScheduling  default-scheduler  0/3 nodes are available: 3 node(s) had untolerated taint {dedicated: gpu-only: NoSchedule}. preemption: 0/3 nodes are available: 3 Preemption is not helpful for scheduling.`

// k8sPendingTaintsTest: identify an untolerated node taint as the reason a
// pod cannot be scheduled, and name the pod-spec field that fixes it.
//
// ground truth: the FailedScheduling event names the exact blocker -
// "untolerated taint {dedicated: gpu-only: NoSchedule}" on all 3
// candidate nodes - and the pod spec confirms it has no tolerations at
// all. Adding a matching entry under the pod's tolerations field (not
// nodeSelector, which restricts placement rather than tolerating a taint,
// and not resources, which is unrelated) is the fix.
func k8sPendingTaintsTest() testkit.Test {
	prompt := `Here is the output of "kubectl describe pod" for a pod stuck
Pending:

` + k8sPendingTaintsDescribeExcerpt + `

Why can the scheduler not place this pod on any of the 3 candidate nodes,
and which pod spec field must be added to fix it? Respond with only a
JSON object:
{"cause":"<one of: taint, resource-limit, affinity>","fix_field":"<one of: tolerations, nodeSelector, resources>"}`

	evaluator := eval.Mean(
		eval.JSONField("cause", "taint"),
		eval.JSONField("fix_field", "tolerations"),
	)

	return testkit.Test{
		ID:          "k8s-pending-taints",
		Category:    "operations",
		Subcategory: "kubernetes",
		Description: "Identify an untolerated node taint blocking scheduling and the tolerations field that fixes it.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// k8sPVCStorageclassExcerpt is the inline kubectl describe pvc + kubectl
// get storageclass excerpt for k8sPVCStorageclassMismatchTest.
const k8sPVCStorageclassExcerpt = `$ kubectl describe pvc data-pvc -n analytics
Name:          data-pvc
Namespace:     analytics
StorageClass:  fast-ssd
Status:        Pending
Events:
  Warning  ProvisioningFailed  persistentvolume-controller  storageclass.storage.k8s.io "fast-ssd" not found

$ kubectl get storageclass
NAME       PROVISIONER
standard   csi.example.com`

// k8sPVCStorageclassMismatchTest: diagnose a Pending PVC referencing a
// StorageClass that does not exist in the cluster, and pick the existing
// class to switch to.
//
// ground truth: the ProvisioningFailed event states plainly that
// StorageClass "fast-ssd" does not exist; "kubectl get storageclass" shows
// only "standard" exists. Since this cluster's StorageClasses are managed
// centrally via GitOps and creating one ad hoc is ruled out by the prompt,
// the only available fix is to point the PVC at the StorageClass that
// already exists: "standard".
func k8sPVCStorageclassMismatchTest() testkit.Test {
	prompt := `This cluster's StorageClasses are defined centrally via
GitOps; creating a new StorageClass ad hoc is not an option here. Here is
the PVC and the StorageClasses that actually exist in the cluster:

` + k8sPVCStorageclassExcerpt + `

What caused this PVC to stay Pending, and which existing StorageClass name
should it be changed to reference so it can be provisioned? Respond with
only a JSON object:
{"cause":"<one of: missing-storageclass, provisioner-down, quota-exceeded>","use_storageclass":"<name>"}`

	evaluator := eval.Mean(
		eval.JSONField("cause", "missing-storageclass"),
		eval.JSONField("use_storageclass", "standard"),
	)

	return testkit.Test{
		ID:          "k8s-pvc-storageclass-mismatch",
		Category:    "operations",
		Subcategory: "kubernetes",
		Description: "Diagnose a Pending PVC referencing a nonexistent StorageClass and pick the one existing class to use instead.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// k8sServiceSelectorMismatchYAML is the inline Deployment + Service YAML
// excerpt for k8sServiceSelectorMismatchTest.
const k8sServiceSelectorMismatchYAML = `# Deployment (excerpt)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: inventory-api
  namespace: inventory
spec:
  template:
    metadata:
      labels:
        app: inventory-api
        tier: backend

---
# Service
apiVersion: v1
kind: Service
metadata:
  name: inventory-api
  namespace: inventory
spec:
  selector:
    app: inventory-api-svc
  ports:
    - port: 80
      targetPort: 8080`

// k8sServiceSelectorMismatchTest: spot a Service selector that does not
// match the Deployment's pod-template labels, leaving the Service with no
// Endpoints.
//
// ground truth: the pods carry label app=inventory-api (from the
// Deployment's pod template), but the Service's selector is
// app=inventory-api-svc - a value that matches no pod's labels, so the
// Service has zero Endpoints. The single wrong field is the Service's
// spec.selector.app value; it must be changed to "inventory-api" to match
// the pods. The prompt's JSON template pre-fills "field":"selector.app"
// as part of the requested response shape, so a model that never
// diagnosed the field at all could still copy that value verbatim; only
// "value" is actually evidence the model found the fix (AN2).
func k8sServiceSelectorMismatchTest() testkit.Test {
	prompt := `Requests to this Service time out with no available
endpoints. Here is the Deployment's pod template and the Service:

` + k8sServiceSelectorMismatchYAML + `

Which single field in the Service YAML is wrong, and what value should it
be changed to so the Service's Endpoints include these pods? Respond with
only a JSON object: {"field":"selector.app","value":"<correct value>"}`

	evaluator := eval.JSONField("value", "inventory-api")

	return testkit.Test{
		ID:          "k8s-service-selector-mismatch",
		Category:    "operations",
		Subcategory: "kubernetes",
		Description: "Spot a Service selector.app value that does not match the Deployment's pod-template labels.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// k8sNetworkPolicyDNSYAML is the inline default-deny-egress NetworkPolicy
// excerpt for k8sNetworkPolicyDNSBlockTest.
const k8sNetworkPolicyDNSYAML = `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: payments-egress
  namespace: payments
spec:
  podSelector:
    matchLabels:
      app: payments
  policyTypes:
    - Egress
  egress:
    - to:
        - podSelector:
            matchLabels:
              app: payments-db
      ports:
        - protocol: TCP
          port: 5432`

// k8sNetworkPolicyDNSBlockTest: explain why a default-deny-egress
// NetworkPolicy that only allows one destination breaks DNS resolution,
// and what egress rule fixes it.
//
// ground truth: this NetworkPolicy selects pods labeled app=payments,
// applies to Egress, and lists exactly one allowed destination (the
// payments-db pods on 5432/TCP) - meaning every other egress, including
// DNS lookups to CoreDNS/kube-dns on port 53, is implicitly denied. The
// fix is an additional egress rule allowing UDP (and typically TCP) port
// 53 to the cluster's DNS pods (conventionally labeled k8s-app=kube-dns in
// the kube-system namespace).
func k8sNetworkPolicyDNSBlockTest() testkit.Test {
	prompt := `Pods labeled app=payments in the payments namespace can reach
the payments database but cannot resolve ANY DNS name, including external
ones. Here is the only NetworkPolicy selecting these pods:

` + k8sNetworkPolicyDNSYAML + `

Why does this NetworkPolicy break DNS resolution for these pods, and what
additional egress rule (name the port, protocol, and typical target label)
must be added to fix it?`

	evaluator := eval.All(
		eval.W(eval.ContainsAll("53"), 2),
		eval.W(eval.ContainsAny("udp", "UDP"), 2),
		eval.W(eval.ContainsAny("kube-dns", "coredns", "kube-system"), 2),
	)

	return testkit.Test{
		ID:          "k8s-networkpolicy-dns-block",
		Category:    "operations",
		Subcategory: "kubernetes",
		Description: "Explain why a single-destination-allow egress NetworkPolicy implicitly blocks DNS, and the port-53/UDP fix.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// k8sArgoCDOutOfSyncStatus is the inline "argocd app get" excerpt for
// k8sArgoCDOutOfSyncTest.
const k8sArgoCDOutOfSyncStatus = `$ argocd app get inventory-api
Name:               inventory-api
Namespace:          inventory
Health Status:      Healthy
Sync Status:         OutOfSync

Resources:
  Kind         Name           Status
  Deployment   inventory-api  OutOfSync

Diff (live vs target, Deployment/inventory-api):
  live:   spec.replicas: 5
  target: spec.replicas: 2

Note: a HorizontalPodAutoscaler named inventory-api-hpa also targets this
Deployment and actively manages its replica count based on CPU load.
Nobody has manually run "kubectl scale", "kubectl edit", or
"argocd app sync" recently.`

// k8sArgoCDOutOfSyncTest: identify an HPA-managed field as the reason an
// ArgoCD Application stays OutOfSync, and the idiomatic fix.
//
// ground truth: the only diff is spec.replicas, and an HPA legitimately
// changes that field outside of git as it scales the Deployment - so the
// Application will drift back OutOfSync after every "argocd app sync"
// without addressing the underlying cause. ArgoCD's own mechanism for this
// exact situation is configuring spec.ignoreDifferences on the Application
// for the HPA-managed field, not repeatedly re-syncing or reverting a
// change nobody made.
func k8sArgoCDOutOfSyncTest() testkit.Test {
	prompt := `Here is the status of an ArgoCD Application that keeps
showing OutOfSync:

` + k8sArgoCDOutOfSyncStatus + `

Why is this Application stuck OutOfSync on spec.replicas specifically, and
what is the correct ArgoCD-idiomatic fix so it stops flapping between
Synced and OutOfSync? Respond with only a JSON object:
{"cause":"<one of: hpa-managed-field, manual-drift, stale-git>","fix":"<one of: ignoreDifferences, revert-manual-change, sync-git>"}`

	evaluator := eval.Mean(
		eval.JSONField("cause", "hpa-managed-field"),
		eval.JSONField("fix", "ignoreDifferences"),
	)

	return testkit.Test{
		ID:          "k8s-argocd-outofsync",
		Category:    "operations",
		Subcategory: "kubernetes",
		Description: "Identify an HPA-managed replicas field as the cause of a flapping ArgoCD OutOfSync and the ignoreDifferences fix.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// k8sTraefikIngressRouteYAML is the inline IngressRoute excerpt for
// k8sTraefikIngressRouteHostTest.
const k8sTraefikIngressRouteYAML = "apiVersion: traefik.io/v1alpha1\n" +
	"kind: IngressRoute\n" +
	"metadata:\n" +
	"  name: app-route\n" +
	"  namespace: web\n" +
	"spec:\n" +
	"  entryPoints:\n" +
	"    - websecure\n" +
	"  routes:\n" +
	"    - match: Host(`app.example.com`)\n" +
	"      kind: Rule\n" +
	"      services:\n" +
	"        - name: app-svc\n" +
	"          port: 80"

// k8sTraefikIngressRoutePattern requires the corrected match expression,
// with the domain app.raczylo.com quoted with either backticks (Traefik's
// own matcher syntax, as shown in the prompt's example) or double quotes
// (a common, equally unambiguous way a model renders the same literal)
// (AN4).
const k8sTraefikIngressRoutePattern = "(?i)host\\([`\"]app\\.raczylo\\.com[`\"]\\)"

// k8sTraefikIngressRouteHostTest: correct a Traefik IngressRoute's Host()
// match rule to the actual domain the service must answer for.
//
// ground truth: the IngressRoute's only match rule is
// Host(`app.example.com`), but the domain that must route to this service
// is app.raczylo.com - a different rule value entirely, not a change to
// entryPoints, the service name, or the port.
func k8sTraefikIngressRouteHostTest() testkit.Test {
	prompt := `This Traefik IngressRoute is not receiving any traffic
because it is matching the wrong hostname. The actual domain this service
must answer for is app.raczylo.com. Here is the IngressRoute:

` + "```yaml\n" + k8sTraefikIngressRouteYAML + "\n```" + `

Give the single corrected "match" rule line (the full match expression,
using the same Host() syntax) for this IngressRoute.`

	return testkit.Test{
		ID:          "k8s-traefik-ingressroute-host",
		Category:    "operations",
		Subcategory: "kubernetes",
		Description: "Correct a Traefik IngressRoute's Host() match rule to the actual domain the service must serve.",
		Prompt:      prompt,
		Eval:        eval.Regex(k8sTraefikIngressRoutePattern),
	}
}

// k8sQoSClassPodSpec is the inline pod spec excerpt for k8sQoSClassTest.
const k8sQoSClassPodSpec = `apiVersion: v1
kind: Pod
metadata:
  name: report-generator
spec:
  containers:
    - name: report-generator
      image: registry.internal.example.com/report-generator:v3
      resources:
        requests:
          cpu: "250m"
          memory: "256Mi"
        limits:
          cpu: "500m"
          memory: "512Mi"`

// k8sQoSClassTest: derive the scheduler-assigned QoS class from a pod
// spec's requests/limits.
//
// ground truth: every container defines both requests and limits for both
// cpu and memory, ruling out BestEffort (which requires no requests or
// limits at all). Since requests (250m/256Mi) differ from limits
// (500m/512Mi) rather than being exactly equal, the class is Burstable,
// not Guaranteed (which additionally requires requests == limits for
// every resource on every container).
func k8sQoSClassTest() testkit.Test {
	prompt := `Here is a pod spec:

` + "```yaml\n" + k8sQoSClassPodSpec + "\n```" + `

What Quality of Service (QoS) class does the Kubernetes scheduler assign
to this pod? Respond with only one word: Guaranteed, Burstable, or
BestEffort.`

	return testkit.Test{
		ID:          "k8s-qos-class",
		Category:    "operations",
		Subcategory: "kubernetes",
		Description: "Derive a pod's QoS class (Burstable) from requests that are set but not equal to limits.",
		Prompt:      prompt,
		// A7: eval.ExactToken (not eval.Equals) so a fenced, quoted, or
		// bolded "Burstable" - a common way a model formats a forced
		// one-word answer - still scores full credit.
		Eval: eval.ExactToken("Burstable"),
	}
}

// k8sCNPGPDBTest: pick PodDisruptionBudget with minAvailable as the
// mechanism guaranteeing quorum survives voluntary node maintenance for a
// CNPG-managed Postgres cluster.
func k8sCNPGPDBTest() testkit.Test {
	prompt := `A CNPG (CloudNativePG)-managed Postgres cluster runs as 3
pods (1 primary, 2 replicas) in the shared-resources namespace. The team
wants a guarantee that at least 2 of the 3 Postgres pods stay available
during voluntary node maintenance (e.g. "kubectl drain"). Which Kubernetes
resource type provides this guarantee, and what field on it must be set,
with what minimum value, to enforce "at least 2 available"?`

	// ground truth: a PodDisruptionBudget (PDB) is the resource that
	// bounds voluntary disruptions (like a node drain evicting pods) for a
	// selected set of pods. Setting its minAvailable field to 2 enforces
	// that at least 2 of the 3 Postgres pods must remain available before
	// the eviction API will permit another one to be drained. The
	// minAvailable check is a single regex tolerant of any short
	// same-line phrasing between the key and the value (A10) - e.g.
	// "minAvailable is set to 2" or "minAvailable: \"2\"" - rather than an
	// enumerated list of exact phrasings that misses anything not on the
	// list.
	evaluator := eval.Mean(
		eval.ContainsAll("PodDisruptionBudget"),
		eval.Regex(`(?i)minAvailable\b[^\n]{0,20}\b2\b`),
	)

	return testkit.Test{
		ID:          "k8s-cnpg-pdb",
		Category:    "operations",
		Subcategory: "kubernetes",
		Description: "Identify PodDisruptionBudget with minAvailable=2 as the guarantee for a 3-pod CNPG cluster during node drains.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

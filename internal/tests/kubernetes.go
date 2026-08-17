package tests

import (
	"context"
	"regexp"
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerKubernetesTests(r *testkit.Registry) {
	r.Register(k8sCrashloopGitopsTest())
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

// negationCuePattern matches a word that turns a mention of the forbidden
// command into a warning against running it, rather than an instruction to
// run it (e.g. "do not run kubectl patch...").
var negationCuePattern = regexp.MustCompile(`(?i)\b(don'?t|do not|never|avoid|instead of|not|cannot|can'?t|rather than|without|no need)\b`)

// negationWindow is how many characters before a "kubectl edit|patch"
// mention are searched for a negation cue.
const negationWindow = 60

// negationWindowStart returns the earliest byte offset in response to
// search for a negation cue before a "kubectl edit|patch" match starting
// at start. The window is the current line, extended back to the start of
// the immediately preceding line when that line is non-empty: a
// hard-wrapped sentence whose negation cue landed on the line above (e.g.
// "...do not run\nkubectl edit...") would otherwise score 0 even though it
// is the correct, negated answer. Either way, the window never reaches
// more than negationWindow characters before start.
func negationWindowStart(response string, start int) int {
	curLineStart := strings.LastIndexByte(response[:start], '\n') + 1

	windowFloor := curLineStart
	if curLineStart > 0 {
		prevLineEnd := curLineStart - 1 // the newline separating the two lines
		prevLineStart := strings.LastIndexByte(response[:prevLineEnd], '\n') + 1
		if strings.TrimSpace(response[prevLineStart:prevLineEnd]) != "" {
			windowFloor = prevLineStart
		}
	}

	return max(start-negationWindow, windowFloor)
}

// noLiveKubectlMutation scores full credit unless the response instructs
// running "kubectl edit"/"kubectl patch" against the live cluster. A
// contrastive mention - warning the reader not to do this, in favor of the
// GitOps procedure - is fine and does not cost credit; an unnegated
// mention scores zero. This is deliberately not eval.NotContains, which
// would also zero out the best possible answer (one that correctly
// explains why NOT to run kubectl edit/patch).
//
// Every occurrence goes through the same negation-window check, including
// one at the start of a line (a bulleted "don't do this" list item, or a
// hard-wrapped sentence whose negation cue landed on the line above): a
// response is not penalized just because word-wrap or list formatting put
// the command text at a line start. A genuinely bare imperative still
// fails, because negationWindowStart never finds a negation cue in its
// (possibly line-extended) window.
func noLiveKubectlMutation() eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		matches := kubectlLiveMutationPattern.FindAllStringIndex(response, -1)
		if len(matches) == 0 {
			return eval.Score{Value: 1, Detail: "no mention of kubectl edit/patch"}
		}

		for _, loc := range matches {
			start := loc[0]
			windowStart := negationWindowStart(response, start)
			if !negationCuePattern.MatchString(response[windowStart:start]) {
				return eval.Score{Value: 0, Detail: "unnegated mention of kubectl edit/patch"}
			}
		}
		return eval.Score{Value: 1, Detail: "every mention of kubectl edit/patch is negated"}
	})
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

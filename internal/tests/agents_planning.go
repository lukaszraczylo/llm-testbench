package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerAgentPlanningTests(r *testkit.Registry) {
	r.Register(agentPlanOrderingTest())
	r.Register(planCriticalPathTest())
	r.Register(planParallelStepsTest())
	r.Register(planRollbackTriggerTest())
	r.Register(planIdempotencyTest())
	r.Register(planRepairTest())
	r.Register(planMilestoneDateTest())
	r.Register(planResourceConstrainedTest())
	r.Register(planCycleDetectTest())
	r.Register(planMinimalReplanTest())
}

// planStep is one node in a dependency graph shared by the planning
// subcategory's ground-truth derivations: an id, the ids of steps that must
// finish first, and (where relevant) a duration in the arbitrary time unit
// the owning prompt states.
type planStep struct {
	ID       string
	Deps     []string
	Duration int
}

// planCriticalPathLength returns the length of the longest dependency chain
// in steps, measured as the sum of Duration along that chain (the classic
// critical-path-method "minimum time to finish everything, given unlimited
// parallel capacity"). It assumes steps is acyclic; every planning fixture
// in this file is verified acyclic (or, for planCycleSteps, deliberately
// not) in agents_planning_test.go.
func planCriticalPathLength(steps []planStep) int {
	byID := make(map[string]planStep, len(steps))
	for _, s := range steps {
		byID[s.ID] = s
	}
	memo := make(map[string]int, len(steps))
	var finish func(id string) int
	finish = func(id string) int {
		if v, ok := memo[id]; ok {
			return v
		}
		s := byID[id]
		start := 0
		for _, dep := range s.Deps {
			start = max(start, finish(dep))
		}
		v := start + s.Duration
		memo[id] = v
		return v
	}
	longest := 0
	for _, s := range steps {
		longest = max(longest, finish(s.ID))
	}
	return longest
}

// planWaveLayers groups steps into sequential stages, where a step's stage
// is the earliest one possible after all of its Deps' stages have finished,
// assuming unlimited parallel capacity (the standard "topological
// generations" layering). Each stage is sorted alphabetically so the result
// is a canonical value even though membership within a stage is a set.
func planWaveLayers(steps []planStep) [][]string {
	byID := make(map[string]planStep, len(steps))
	for _, s := range steps {
		byID[s.ID] = s
	}
	memo := make(map[string]int, len(steps))
	var layerOf func(id string) int
	layerOf = func(id string) int {
		if v, ok := memo[id]; ok {
			return v
		}
		s := byID[id]
		layer := 0
		for _, dep := range s.Deps {
			layer = max(layer, layerOf(dep)+1)
		}
		memo[id] = layer
		return layer
	}

	maxLayer := 0
	byLayer := make(map[int][]string)
	for _, s := range steps {
		l := layerOf(s.ID)
		byLayer[l] = append(byLayer[l], s.ID)
		maxLayer = max(maxLayer, l)
	}

	out := make([][]string, maxLayer+1)
	for l := 0; l <= maxLayer; l++ {
		ids := byLayer[l]
		sort.Strings(ids)
		out[l] = ids
	}
	return out
}

// planScheduleSingleWorker simulates a single worker executing steps one at
// a time: at every point it picks, among steps whose Deps are all already
// done, the one with the lowest priority value (ties broken by the lower
// ID), and runs it to completion before picking the next. It returns the
// resulting total order.
func planScheduleSingleWorker(steps []planStep, priority map[string]int) []string {
	byID := make(map[string]planStep, len(steps))
	for _, s := range steps {
		byID[s.ID] = s
	}
	done := make(map[string]bool, len(steps))
	order := make([]string, 0, len(steps))

	for len(order) < len(steps) {
		var ready []string
		for _, s := range steps {
			if done[s.ID] {
				continue
			}
			allDepsDone := true
			for _, dep := range s.Deps {
				if !done[dep] {
					allDepsDone = false
					break
				}
			}
			if allDepsDone {
				ready = append(ready, s.ID)
			}
		}
		sort.Slice(ready, func(i, j int) bool {
			if priority[ready[i]] != priority[ready[j]] {
				return priority[ready[i]] < priority[ready[j]]
			}
			return ready[i] < ready[j]
		})
		next := ready[0]
		order = append(order, next)
		done[next] = true
	}
	return order
}

// planWaveSetsEval scores full credit only when the response, parsed as a
// JSON array of arrays of step ids, has the same number of stages as want
// and each stage matches want's stage as an unordered, case-insensitive,
// trimmed set. Stage order matters (a step cannot start until every step in
// the prior stage has finished), but membership within a stage does not,
// per the "canonicalize a set-valued answer" authoring rule - this is why a
// bare JSONStringArrayEquals cannot be used here.
type planWaveSetsEval struct {
	want [][]string
}

func planWaveSets(want [][]string) eval.Evaluator {
	return planWaveSetsEval{want: want}
}

func (p planWaveSetsEval) Evaluate(_ context.Context, response string) eval.Score {
	raw, err := eval.ExtractJSON(response)
	if err != nil {
		return eval.Score{Value: 0, Detail: err.Error()}
	}
	var got [][]string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return eval.Score{Value: 0, Detail: fmt.Sprintf("invalid JSON array of arrays: %v", err)}
	}
	if len(got) != len(p.want) {
		return eval.Score{Value: 0, Detail: fmt.Sprintf("got %d stages, want %d", len(got), len(p.want))}
	}
	for i := range got {
		if !planStringSetEqual(got[i], p.want[i]) {
			return eval.Score{Value: 0, Detail: fmt.Sprintf("stage %d: got %v, want %v (compared as a set)", i, got[i], p.want[i])}
		}
	}
	return eval.Score{Value: 1, Detail: "every stage matches as a set"}
}

// planStringSetEqual reports whether a and b contain the same strings,
// ignoring order, case, and surrounding whitespace.
func planStringSetEqual(a, b []string) bool {
	norm := func(items []string) map[string]struct{} {
		m := make(map[string]struct{}, len(items))
		for _, s := range items {
			m[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
		}
		return m
	}
	am, bm := norm(a), norm(b)
	if len(am) != len(bm) {
		return false
	}
	for k := range am {
		if _, ok := bm[k]; !ok {
			return false
		}
	}
	return true
}

// agentPlanOrderingWant is the unique valid step order implied by the
// dependency constraints in agentPlanOrderingTest's prompt.
var agentPlanOrderingWant = []string{"build", "test", "backup", "deploy", "verify", "rollback"}

// agentPlanOrderingTest: order deployment steps that have explicit
// pairwise dependencies.
//
// ground truth: the prompt states build must precede test (need a built
// artifact to test), backup must happen only after tests pass and only
// before deploy, deploy must follow backup, verify must follow deploy, and
// rollback is only meaningful after a deploy whose verify step failed. This
// pins a single valid total order: build, test, backup, deploy, verify,
// rollback.
func agentPlanOrderingTest() testkit.Test {
	prompt := `You are planning a deployment with these steps: build, test,
backup, deploy, verify, rollback.

Constraints:
- You must build the artifact before you can test it.
- You only take a backup of the current production state once tests have
  passed - there is no point backing up before you know the new build is
  viable.
- Deploy only happens after the backup completes.
- After deploy, you must verify the new deployment is healthy.
- Rollback (restoring the pre-deploy backup) only makes sense after a
  deploy has happened, and only if verify fails.

Give the single correct step order as a JSON array of step ids, e.g.
["build","test",...]. Include all 6 steps, listing rollback in its position
in the sequence even though it is conditional on verify failing.`

	return testkit.Test{
		ID:          "agent-plan-ordering",
		Category:    "agents",
		Subcategory: "planning",
		Description: "Order 6 deployment steps under explicit pairwise dependency constraints.",
		Prompt:      prompt,
		MaxTokens:   300,
		Eval:        eval.JSONStringArrayEquals(agentPlanOrderingWant),
	}
}

// planCriticalPathSteps is a small CI/CD pipeline graph for
// planCriticalPathTest: two branches (test / security_scan) fan out from
// build and fan back in at package, while lint runs independently of both
// and only gates deploy.
var planCriticalPathSteps = []planStep{
	{ID: "build", Duration: 3},
	{ID: "lint", Duration: 2},
	{ID: "test", Deps: []string{"build"}, Duration: 4},
	{ID: "security_scan", Deps: []string{"build"}, Duration: 3},
	{ID: "package", Deps: []string{"test", "security_scan"}, Duration: 1},
	{ID: "deploy", Deps: []string{"package", "lint"}, Duration: 2},
	{ID: "verify", Deps: []string{"deploy"}, Duration: 2},
}

// planCriticalPathWant is planCriticalPathLength(planCriticalPathSteps),
// computed by running the algorithm rather than hardcoded.
//
// ground truth: the three source-to-sink chains are
// build+test+package+deploy+verify = 3+4+1+2+2 = 12,
// build+security_scan+package+deploy+verify = 3+3+1+2+2 = 11, and
// lint+deploy+verify = 2+2+2 = 6. The longest (critical path) is 12.
var planCriticalPathWant = planCriticalPathLength(planCriticalPathSteps)

func planCriticalPathTest() testkit.Test {
	prompt := `Here is a CI/CD pipeline as a set of steps with durations in
minutes and dependencies:

- build: 3 minutes, depends on nothing
- lint: 2 minutes, depends on nothing
- test: 4 minutes, depends on build
- security_scan: 3 minutes, depends on build
- package: 1 minute, depends on test AND security_scan
- deploy: 2 minutes, depends on package AND lint
- verify: 2 minutes, depends on deploy

Assume unlimited parallel workers, so any steps whose dependencies are
already satisfied can run at the same time. What is the critical path
length in minutes - the minimum total time to finish every step, respecting
all dependencies? Respond with only the number.`

	return testkit.Test{
		ID:          "agent-plan-critical-path",
		Category:    "agents",
		Subcategory: "planning",
		Description: "Compute the critical path length in minutes of a 7-step CI/CD pipeline with two fan-out/fan-in branches.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], planCriticalPathWant, 0),
	}
}

// planWaveSteps is a small 5-node graph for planParallelStepsTest: a and b
// are independent sources, c depends only on a, d depends on both a and b,
// and e depends on both c and d.
var planWaveSteps = []planStep{
	{ID: "a"},
	{ID: "b"},
	{ID: "c", Deps: []string{"a"}},
	{ID: "d", Deps: []string{"a", "b"}},
	{ID: "e", Deps: []string{"c", "d"}},
}

// planWaveWant is planWaveLayers(planWaveSteps), computed by running the
// layering algorithm rather than hardcoded.
//
// ground truth: a and b have no dependencies, so both are stage 0. c
// depends only on a (stage 0), so c is stage 1; d depends on a (stage 0)
// and b (stage 0), so d is also stage 1. e depends on c and d, both stage
// 1, so e is stage 2. Stages: [a,b], [c,d], [e].
var planWaveWant = planWaveLayers(planWaveSteps)

func planParallelStepsTest() testkit.Test {
	prompt := `Here are 5 steps with dependencies:

- a: depends on nothing
- b: depends on nothing
- c: depends on a
- d: depends on a AND b
- e: depends on c AND d

Group these steps into sequential stages, where a step must be in the
earliest stage possible once all of its dependencies' stages are complete,
and assume unlimited parallel capacity (every step in a stage can run at
the same time, since none of them depends on another step in the same
stage). Respond with only a JSON array of arrays of step ids, ordered by
stage, e.g. [["a","b"],["c"]]. Within a single stage's array, the order of
ids does not matter.`

	return testkit.Test{
		ID:          "agent-plan-parallel-steps",
		Category:    "agents",
		Subcategory: "planning",
		Description: "Group 5 dependency-graph steps into the sequential parallel-execution stages implied by the graph.",
		Prompt:      prompt,
		Eval:        planWaveSets(planWaveWant),
	}
}

// planRollbackTriggerTest: pin exactly when a rollback decision should be
// evaluated and what condition should trigger it, from an enumerated list
// of options (forcing one deterministic answer rather than free text).
//
// ground truth: rollback restores the pre-deploy backup, so it is only ever
// meaningful once a deploy has actually happened; evaluating it any earlier
// (e.g. after test or backup) is premature since nothing has been deployed
// yet to roll back from. The condition that should trigger it is
// specifically a failed verify - a failed build or test should stop the
// pipeline outright, long before deploy, and a manual request is not the
// scenario described.
func planRollbackTriggerTest() testkit.Test {
	prompt := `A deployment pipeline runs these steps in order: build, test,
backup, deploy, verify, rollback. Rollback restores the pre-deploy backup
and is only ever useful once a deploy has actually happened.

After which step should the pipeline evaluate whether to trigger rollback,
and what specific condition should trigger it?

Choose trigger_after from exactly one of: build, test, backup, deploy,
verify.
Choose condition from exactly one of: build_failure, test_failure,
verify_failure, manual_request.

Respond with only a JSON object:
{"trigger_after":"...","condition":"..."}`

	evaluator := eval.Mean(
		eval.JSONField("trigger_after", "verify"),
		eval.JSONField("condition", "verify_failure"),
	)

	return testkit.Test{
		ID:          "agent-plan-rollback-trigger",
		Category:    "agents",
		Subcategory: "planning",
		Description: "Pin the correct step and condition that should trigger a rollback, from enumerated option lists.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// planIdempotencySteps describes 5 pipeline steps with side-effect text
// that determines whether rerunning each one from scratch is safe.
const planIdempotencySteps = `- fetch_deps: downloads dependencies into a local cache directory,
  overwriting any partial download from a prior attempt.
- run_migrations: applies pending database schema migrations using a
  migration tool that tracks which migrations already ran and skips them on
  a rerun.
- apply_terraform: runs "terraform apply", which reconciles infrastructure
  to the declared state and is explicitly designed to be safe to rerun.
- send_notification: posts a message to a Slack channel announcing the
  deploy started, with no deduplication.
- increment_deploy_counter: increments a deploy counter by 1 in a metrics
  store, with no deduplication.`

// planIdempotencyWant is the set of steps safe to retry from scratch.
//
// ground truth: fetch_deps overwrites cleanly, run_migrations explicitly
// tracks and skips already-applied migrations, and apply_terraform
// reconciles to a declared state - all three produce the same end result
// whether run once or twice, so rerunning them after a crash is safe.
// send_notification and increment_deploy_counter have no deduplication, so
// rerunning either after a crash (rather than resuming past it) produces a
// duplicate Slack message or an inflated counter - both unsafe to retry.
var planIdempotencyWant = []string{"fetch_deps", "run_migrations", "apply_terraform"}

func planIdempotencyTest() testkit.Test {
	prompt := `Here are 5 pipeline steps and what each one does:

` + planIdempotencySteps + `

If the pipeline crashes partway through and you restart it from the
beginning, which of these steps are safe to retry (idempotent - rerunning
produces the same end result, with no bad side effect like a duplicate
notification or an inflated counter)? Respond with only a JSON array of the
safe-to-retry step ids.`

	return testkit.Test{
		ID:          "agent-plan-idempotency",
		Category:    "agents",
		Subcategory: "planning",
		Description: "Identify which of 5 pipeline steps are safe to retry from scratch after a crash, from their stated side effects.",
		Prompt:      prompt,
		Eval:        eval.JSONStringSet(planIdempotencyWant),
	}
}

// planRepairTest: a pipeline fails during "test" - before backup or deploy
// have run - so the correct repair action is to fix the code and rerun
// test, not to (mistakenly) treat the failure as something rollback or a
// later step could address.
//
// ground truth: the original plan is build, test, backup, deploy, verify,
// rollback (same as agent-plan-ordering). Test failed right after build
// succeeded, so nothing has been backed up or deployed yet. rollback
// requires an existing deploy to undo, and backup/deploy/verify all require
// a passing test first - none of them is reachable or meaningful from this
// failure point. The correct next action is to fix the code and rerun
// test, restarting the sequence from there.
func planRepairTest() testkit.Test {
	prompt := `A deployment pipeline runs: build, test, backup, deploy,
verify, rollback, in that order, with the same dependency rules as a
standard pipeline (each step requires the previous ones to have succeeded;
rollback requires an actual deploy to undo).

On this run, build succeeded, then test failed. Nothing after test has run
yet - no backup was taken and nothing was deployed.

What is the correct next action? Choose exactly one from: proceed_to_backup,
deploy, rollback, fix_and_rerun_test, abort_pipeline_permanently.

Respond with only a JSON object: {"next_action":"..."}`

	return testkit.Test{
		ID:          "agent-plan-repair",
		Category:    "agents",
		Subcategory: "planning",
		Description: "Pick the correct repair action after a pipeline fails at 'test', before anything was backed up or deployed.",
		Prompt:      prompt,
		Eval:        eval.JSONField("next_action", "fix_and_rerun_test"),
	}
}

// planMilestoneWant is max(3, 5) + 2, computed via the same reasoning the
// prompt describes rather than hardcoded.
//
// ground truth: prep_env finishes on day 0+3=3, prep_data finishes on day
// 0+5=5. Milestone M cannot start until BOTH finish, so M starts on
// max(3,5)=5, and finishes 2 days later, on day 5+2=7. B7: the prompt now
// states the t+d finish-day rule explicitly rather than relying on a
// "day 0" framing a reader could also read as 1-indexed (which would make
// 6 a defensible, if unintended, answer); 7 remains the only correct one.
var planMilestoneWant = max(3, 5) + 2

func planMilestoneDateTest() testkit.Test {
	prompt := `Two prerequisite work items run in parallel, starting at day 0:
- prep_env takes 3 days.
- prep_data takes 5 days.

Milestone M can only start once BOTH prep_env and prep_data have finished,
and M itself takes 2 days once it starts.

Use this rule: a task starting at day t with duration d finishes at day
t+d. Work starts at day 0.

On what day number does milestone M finish? Respond with only the
number.`

	return testkit.Test{
		ID:          "agent-plan-milestone-date",
		Category:    "agents",
		Subcategory: "planning",
		Description: "Compute the finish day of a milestone that depends on the slower of two parallel prerequisites plus its own duration.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], planMilestoneWant, 0),
	}
}

// planResourceConstrainedSteps models a single-engineer homelab deploy:
// provisioning a Proxmox LXC container and installing software on it can
// only happen after the container exists, while network configuration is
// independent, and the final deploy step needs both branches done.
var planResourceConstrainedSteps = []planStep{
	{ID: "provision_ct"},
	{ID: "configure_network"},
	{ID: "install_graphann", Deps: []string{"provision_ct"}},
	{ID: "deploy_reranker_model", Deps: []string{"configure_network", "install_graphann"}},
	{ID: "smoke_test", Deps: []string{"deploy_reranker_model"}},
}

// planResourceConstrainedPriority is the "do the lower number first when
// multiple steps are ready" tie-break rule stated in the prompt.
var planResourceConstrainedPriority = map[string]int{
	"provision_ct":          1,
	"configure_network":     2,
	"install_graphann":      1,
	"deploy_reranker_model": 2,
	"smoke_test":            1,
}

// planResourceConstrainedWant is
// planScheduleSingleWorker(planResourceConstrainedSteps,
// planResourceConstrainedPriority), computed by running the scheduler
// rather than hardcoded. agents_planning_test.go independently proves this
// is the ONLY order consistent with both the dependency graph and the
// priority rule, by brute-force enumeration.
//
// ground truth: initially ready = {provision_ct(p1), configure_network(p2)}
// -> pick provision_ct (lower priority). Then ready =
// {configure_network(p2), install_graphann(p1)} -> pick install_graphann.
// Then ready = {configure_network(p2)} only (deploy_reranker_model still
// needs configure_network) -> pick configure_network. Then ready =
// {deploy_reranker_model(p2)} -> pick it. Finally smoke_test.
var planResourceConstrainedWant = planScheduleSingleWorker(planResourceConstrainedSteps, planResourceConstrainedPriority)

func planResourceConstrainedTest() testkit.Test {
	prompt := `Only one engineer is available, so steps must run strictly
one at a time - never two at once, even if their dependencies allow it.

Steps, dependencies, and priority (lower number = higher priority):
- provision_ct: depends on nothing, priority 1
- configure_network: depends on nothing, priority 2
- install_graphann: depends on provision_ct, priority 1
- deploy_reranker_model: depends on configure_network AND
  install_graphann, priority 2
- smoke_test: depends on deploy_reranker_model, priority 1

Rule: whenever more than one step is ready to start (its dependencies are
all done) at the same time, the engineer always does the lower-priority-
number step first.

Give the resulting single-file execution order as a JSON array of step ids.
Respond with only the JSON array.`

	return testkit.Test{
		ID:          "agent-plan-resource-constrained",
		Category:    "agents",
		Subcategory: "planning",
		Description: "Order 5 steps for a single available worker under both a dependency graph and a priority tie-break rule.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals(planResourceConstrainedWant),
	}
}

// planCycleSteps has a genuine circular dependency among c, d, and e: c
// depends on e, d depends on c, and e depends on d, so none of the three
// can ever become ready. a and b are unaffected.
var planCycleSteps = []planStep{
	{ID: "a"},
	{ID: "b", Deps: []string{"a"}},
	{ID: "c", Deps: []string{"b", "e"}},
	{ID: "d", Deps: []string{"c"}},
	{ID: "e", Deps: []string{"d"}},
}

// planCycleWant is the set of task ids in the circular dependency.
//
// ground truth: a has no deps (fine). b depends only on a (fine). c
// depends on b AND e; d depends on c; e depends on d. Following the chain
// from c: c needs e, e needs d, d needs c - a genuine cycle among c, d, and
// e, none of which can ever be satisfied. agents_planning_test.go
// independently confirms this by brute-force enumeration: the full graph
// admits zero valid topological orders, and removing just the c-depends-
// on-e edge restores at least one.
var planCycleWant = []string{"c", "d", "e"}

func planCycleDetectTest() testkit.Test {
	prompt := `Here are 5 tasks with dependencies:

- a: depends on nothing
- b: depends on a
- c: depends on b AND e
- d: depends on c
- e: depends on d

One of these dependency chains loops back on itself, so those tasks can
never all be completed. Which task ids form that circular dependency?
Respond with only a JSON array of the task ids in the cycle.`

	return testkit.Test{
		ID:          "agent-plan-cycle-detect",
		Category:    "agents",
		Subcategory: "planning",
		Description: "Identify which 3 of 5 tasks form a circular dependency that can never be satisfied.",
		Prompt:      prompt,
		Eval:        eval.JSONStringSet(planCycleWant),
	}
}

// planMinimalReplanWant is agent-plan-ordering's order with backup removed,
// since the storage requirement change removes only that one step and its
// downstream edge (deploy now follows test directly).
var planMinimalReplanWant = []string{"build", "test", "deploy", "verify", "rollback"}

// planMinimalReplanTest: a requirement change removes exactly one step
// from the known-good plan; the correct replan keeps everything else
// unchanged rather than re-deriving a new plan from scratch.
//
// ground truth: this restates agent-plan-ordering's original constraints
// (build before test, deploy after test, verify after deploy, rollback
// after a failed verify) minus the backup step, which the requirement
// change states is no longer needed. The minimal update removes backup and
// closes the gap it leaves - deploy now depends directly on test - and
// otherwise preserves the original order.
func planMinimalReplanTest() testkit.Test {
	prompt := `The original deployment plan was: build, test, backup,
deploy, verify, rollback, with these constraints:
- build must precede test.
- backup only happens after tests pass, and only before deploy.
- deploy only happens after backup.
- verify must follow deploy.
- rollback only makes sense after a deploy whose verify step failed.

Requirement change: production storage now uses continuous replication, so
the backup step is no longer needed at all - remove it entirely, and deploy
now proceeds directly once tests pass. Every other constraint above still
applies unchanged.

Give the minimally updated step order (5 steps: backup is dropped, nothing
else changes) as a JSON array of step ids. Respond with only the JSON
array.`

	return testkit.Test{
		ID:          "agent-plan-minimal-replan",
		Category:    "agents",
		Subcategory: "planning",
		Description: "Update a known-good step order after a requirement change removes exactly one step, without re-deriving the rest.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals(planMinimalReplanWant),
	}
}

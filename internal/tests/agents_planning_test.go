package tests

import (
	"context"
	"reflect"
	"testing"
)

// planEnumerateTopoOrders returns every permutation of steps' ids that is a
// valid topological order (every step appears after all of its Deps). It
// exists only to prove, by exhaustive brute-force enumeration, that a
// planning fixture admits exactly one valid order (or, for planCycleSteps,
// admits none) - the small N (<=5) used by every fixture in this file
// makes brute force cheap.
func planEnumerateTopoOrders(steps []planStep) [][]string {
	ids := make([]string, len(steps))
	depsByID := make(map[string][]string, len(steps))
	for i, s := range steps {
		ids[i] = s.ID
		depsByID[s.ID] = s.Deps
	}

	var permutations [][]string
	var permute func(remaining, chosen []string)
	permute = func(remaining, chosen []string) {
		if len(remaining) == 0 {
			order := make([]string, len(chosen))
			copy(order, chosen)
			permutations = append(permutations, order)
			return
		}
		for i, id := range remaining {
			rest := make([]string, 0, len(remaining)-1)
			rest = append(rest, remaining[:i]...)
			rest = append(rest, remaining[i+1:]...)
			nextChosen := make([]string, len(chosen)+1)
			copy(nextChosen, chosen)
			nextChosen[len(chosen)] = id
			permute(rest, nextChosen)
		}
	}
	permute(ids, nil)

	var valid [][]string
	for _, order := range permutations {
		position := make(map[string]int, len(order))
		for i, id := range order {
			position[id] = i
		}
		ok := true
		for id, deps := range depsByID {
			for _, dep := range deps {
				if position[dep] > position[id] {
					ok = false
					break
				}
			}
			if !ok {
				break
			}
		}
		if ok {
			valid = append(valid, order)
		}
	}
	return valid
}

// planIsPriorityConsistent independently re-derives, at every position in
// order, the "ready" set (steps whose Deps are already placed earlier) and
// checks the chosen step is the minimum by (priority, id) among them - the
// same rule planResourceConstrainedTest's prompt states in words. It is
// used to filter planEnumerateTopoOrders' output down to the orders
// consistent with that rule, independently of planScheduleSingleWorker.
func planIsPriorityConsistent(order []string, steps []planStep, priority map[string]int) bool {
	depsByID := make(map[string][]string, len(steps))
	for _, s := range steps {
		depsByID[s.ID] = s.Deps
	}
	done := make(map[string]bool, len(steps))
	for _, chosen := range order {
		var ready []string
		for _, s := range steps {
			if done[s.ID] {
				continue
			}
			allDone := true
			for _, dep := range depsByID[s.ID] {
				if !done[dep] {
					allDone = false
					break
				}
			}
			if allDone {
				ready = append(ready, s.ID)
			}
		}
		best := ready[0]
		for _, id := range ready[1:] {
			if priority[id] < priority[best] || (priority[id] == priority[best] && id < best) {
				best = id
			}
		}
		if chosen != best {
			return false
		}
		done[chosen] = true
	}
	return true
}

func TestAgentPlanOrderingTest_Eval(t *testing.T) {
	tc := agentPlanOrderingTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct order",
			response: `["build","test","backup","deploy","verify","rollback"]`,
			want:     1,
		},
		{
			name:     "correct order fenced",
			response: "```json\n[\"build\",\"test\",\"backup\",\"deploy\",\"verify\",\"rollback\"]\n```",
			want:     1,
		},
		{
			name:     "backup before test violates dependency",
			response: `["build","backup","test","deploy","verify","rollback"]`,
			want:     0,
		},
		{
			name:     "rollback before verify",
			response: `["build","test","backup","deploy","rollback","verify"]`,
			want:     0,
		},
		{
			name:     "missing a step",
			response: `["build","test","backup","deploy","verify"]`,
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

// TestAgentPlanOrderingTest_UniqueOrder confirms, by brute-force
// enumeration over all 6! permutations, that agentPlanOrderingWant is the
// only ordering of the 6 steps consistent with the prompt's pairwise
// constraints.
func TestAgentPlanOrderingTest_UniqueOrder(t *testing.T) {
	steps := []planStep{
		{ID: "build"},
		{ID: "test", Deps: []string{"build"}},
		{ID: "backup", Deps: []string{"test"}},
		{ID: "deploy", Deps: []string{"backup"}},
		{ID: "verify", Deps: []string{"deploy"}},
		{ID: "rollback", Deps: []string{"verify"}},
	}
	valid := planEnumerateTopoOrders(steps)
	if len(valid) != 1 {
		t.Fatalf("expected exactly 1 valid topological order, found %d: %v", len(valid), valid)
	}
	if !reflect.DeepEqual(valid[0], agentPlanOrderingWant) {
		t.Errorf("unique valid order = %v, want %v", valid[0], agentPlanOrderingWant)
	}
}

func TestPlanCriticalPathTest_Eval(t *testing.T) {
	tc := planCriticalPathTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct number", response: "12", want: 1},
		{name: "correct number with trailing reasoning line", response: "The critical path is build+test+package+deploy+verify.\n12", want: 1},
		{name: "correct number with unit suffix", response: "12 minutes", want: 1},
		{name: "wrong: shorter branch length", response: "11", want: 0},
		{name: "wrong: sum of all durations ignoring parallelism", response: "17", want: 0},
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

// TestPlanCriticalPathWant_HandDerived cross-checks planCriticalPathLength's
// output against an independently hand-computed value (see the ground
// truth comment on planCriticalPathWant), rather than only comparing the
// algorithm to itself.
func TestPlanCriticalPathWant_HandDerived(t *testing.T) {
	const handDerived = 3 + 4 + 1 + 2 + 2 // build+test+package+deploy+verify
	if planCriticalPathWant != handDerived {
		t.Errorf("planCriticalPathWant = %d, hand-derived critical path = %d", planCriticalPathWant, handDerived)
	}
}

func TestPlanParallelStepsTest_Eval(t *testing.T) {
	tc := planParallelStepsTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct stages", response: `[["a","b"],["c","d"],["e"]]`, want: 1},
		{name: "correct stages, reordered within a stage", response: `[["b","a"],["d","c"],["e"]]`, want: 1},
		{name: "correct stages, fenced", response: "```json\n[[\"a\",\"b\"],[\"c\",\"d\"],[\"e\"]]\n```", want: 1},
		{name: "wrong: c and d not grouped together", response: `[["a","b"],["c"],["d"],["e"]]`, want: 0},
		{name: "wrong: e placed too early", response: `[["a","b","e"],["c","d"]]`, want: 0},
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

// TestPlanWaveWant_HandDerived cross-checks planWaveLayers' output against
// an independently hand-computed layering.
func TestPlanWaveWant_HandDerived(t *testing.T) {
	want := [][]string{{"a", "b"}, {"c", "d"}, {"e"}}
	if !reflect.DeepEqual(planWaveWant, want) {
		t.Errorf("planWaveWant = %v, hand-derived = %v", planWaveWant, want)
	}
}

func TestPlanRollbackTriggerTest_Eval(t *testing.T) {
	tc := planRollbackTriggerTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: `{"trigger_after":"verify","condition":"verify_failure"}`, want: 1},
		{name: "all correct fenced", response: "```json\n{\"trigger_after\":\"verify\",\"condition\":\"verify_failure\"}\n```", want: 1},
		{name: "wrong trigger step", response: `{"trigger_after":"deploy","condition":"verify_failure"}`, want: 0.5},
		{name: "wrong condition", response: `{"trigger_after":"verify","condition":"deploy_failure"}`, want: 0.5},
		{name: "both wrong", response: `{"trigger_after":"test","condition":"manual_request"}`, want: 0},
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

func TestPlanIdempotencyTest_Eval(t *testing.T) {
	tc := planIdempotencyTest()

	// twoOfThree is JSONStringSet's Jaccard-style overlap for "2 matched
	// out of a denominator of 3" (computed, not hardcoded, to avoid a
	// float-literal precision mismatch against the evaluator's own
	// float64(matched)/float64(denom) division).
	twoOfThree := float64(2) / float64(3)

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "exact correct set",
			response: `["fetch_deps","run_migrations","apply_terraform"]`,
			want:     1,
		},
		{
			name:     "exact correct set, different order",
			response: `["apply_terraform","fetch_deps","run_migrations"]`,
			want:     1,
		},
		{
			name:     "exact correct set, fenced",
			response: "```json\n[\"fetch_deps\",\"run_migrations\",\"apply_terraform\"]\n```",
			want:     1,
		},
		{
			name:     "includes an unsafe step",
			response: `["fetch_deps","run_migrations","apply_terraform","send_notification"]`,
			want:     0.75,
		},
		{
			name:     "missing a safe step",
			response: `["fetch_deps","run_migrations"]`,
			want:     twoOfThree,
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

func TestPlanRepairTest_Eval(t *testing.T) {
	tc := planRepairTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct: fix_and_rerun_test", response: `{"next_action":"fix_and_rerun_test"}`, want: 1},
		{name: "correct, fenced with prose", response: "The right move:\n```json\n{\"next_action\":\"fix_and_rerun_test\"}\n```", want: 1},
		{name: "correct, uppercase", response: `{"next_action":"FIX_AND_RERUN_TEST"}`, want: 1},
		{name: "wrong: rollback (nothing deployed yet)", response: `{"next_action":"rollback"}`, want: 0},
		{name: "wrong: proceed_to_backup (test never passed)", response: `{"next_action":"proceed_to_backup"}`, want: 0},
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

func TestPlanMilestoneDateTest_Eval(t *testing.T) {
	tc := planMilestoneDateTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct number", response: "7", want: 1},
		{name: "correct number with trailing reasoning line", response: "prep_data finishes on day 5, M starts then and takes 2 more days.\n7", want: 1},
		{name: "correct number with unit suffix", response: "day 7", want: 1},
		{name: "wrong: used prep_env instead of the slower prerequisite", response: "5", want: 0},
		{name: "wrong: summed durations serially", response: "10", want: 0},
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

// TestPlanMilestoneWant_HandDerived cross-checks planMilestoneWant against
// an independently hand-computed value.
func TestPlanMilestoneWant_HandDerived(t *testing.T) {
	const handDerived = 7
	if planMilestoneWant != handDerived {
		t.Errorf("planMilestoneWant = %d, hand-derived = %d", planMilestoneWant, handDerived)
	}
}

func TestPlanResourceConstrainedTest_Eval(t *testing.T) {
	tc := planResourceConstrainedTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct order",
			response: `["provision_ct","install_graphann","configure_network","deploy_reranker_model","smoke_test"]`,
			want:     1,
		},
		{
			name:     "correct order fenced",
			response: "```json\n[\"provision_ct\",\"install_graphann\",\"configure_network\",\"deploy_reranker_model\",\"smoke_test\"]\n```",
			want:     1,
		},
		{
			name:     "wrong: ignores priority tie-break at the first choice",
			response: `["configure_network","provision_ct","install_graphann","deploy_reranker_model","smoke_test"]`,
			want:     0,
		},
		{
			name:     "wrong: runs two steps at once conceptually (deploy before configure_network)",
			response: `["provision_ct","install_graphann","deploy_reranker_model","configure_network","smoke_test"]`,
			want:     0,
		},
		{
			name:     "missing a step",
			response: `["provision_ct","install_graphann","configure_network","deploy_reranker_model"]`,
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

// TestPlanResourceConstrainedTest_UniqueOrder independently proves, by
// brute-force enumeration over all 5! permutations filtered against the
// priority rule stated in the prompt, that planResourceConstrainedWant is
// the ONLY order consistent with both the dependency graph and the
// priority tie-break rule.
func TestPlanResourceConstrainedTest_UniqueOrder(t *testing.T) {
	all := planEnumerateTopoOrders(planResourceConstrainedSteps)

	var consistent [][]string
	for _, order := range all {
		if planIsPriorityConsistent(order, planResourceConstrainedSteps, planResourceConstrainedPriority) {
			consistent = append(consistent, order)
		}
	}
	if len(consistent) != 1 {
		t.Fatalf("expected exactly 1 priority-consistent topological order, found %d: %v", len(consistent), consistent)
	}
	if !reflect.DeepEqual(consistent[0], planResourceConstrainedWant) {
		t.Errorf("unique consistent order = %v, want %v", consistent[0], planResourceConstrainedWant)
	}
}

func TestPlanCycleDetectTest_Eval(t *testing.T) {
	tc := planCycleDetectTest()

	// twoOfThree is JSONStringSet's Jaccard-style overlap for "2 matched
	// out of a denominator of 3" (computed, not hardcoded, to avoid a
	// float-literal precision mismatch against the evaluator's own
	// float64(matched)/float64(denom) division).
	twoOfThree := float64(2) / float64(3)

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "exact correct set", response: `["c","d","e"]`, want: 1},
		{name: "exact correct set, different order", response: `["e","c","d"]`, want: 1},
		{name: "exact correct set, fenced", response: "```json\n[\"c\",\"d\",\"e\"]\n```", want: 1},
		{name: "includes an acyclic task", response: `["a","c","d","e"]`, want: 0.75},
		{name: "wrong: only two of the three cyclic tasks", response: `["c","d"]`, want: twoOfThree},
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

// TestPlanCycleDetectTest_NoValidOrder independently confirms
// planCycleSteps genuinely contains a cycle (the full graph admits zero
// valid topological orders under brute-force enumeration), and that
// removing the c-depends-on-e edge specifically restores at least one
// valid order - pinpointing that edge, and thus c/d/e, as the cycle.
func TestPlanCycleDetectTest_NoValidOrder(t *testing.T) {
	if got := planEnumerateTopoOrders(planCycleSteps); len(got) != 0 {
		t.Fatalf("expected zero valid topological orders for a graph containing a cycle, found %d: %v", len(got), got)
	}

	acyclic := make([]planStep, len(planCycleSteps))
	copy(acyclic, planCycleSteps)
	for i, s := range acyclic {
		if s.ID == "c" {
			acyclic[i] = planStep{ID: "c", Deps: []string{"b"}} // drop c's dependency on e
		}
	}
	if got := planEnumerateTopoOrders(acyclic); len(got) == 0 {
		t.Fatal("removing the c-depends-on-e edge should leave at least one valid topological order, found none")
	}
}

func TestPlanMinimalReplanTest_Eval(t *testing.T) {
	tc := planMinimalReplanTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct order", response: `["build","test","deploy","verify","rollback"]`, want: 1},
		{name: "correct order fenced", response: "```json\n[\"build\",\"test\",\"deploy\",\"verify\",\"rollback\"]\n```", want: 1},
		{name: "wrong: backup left in", response: `["build","test","backup","deploy","verify","rollback"]`, want: 0},
		{name: "wrong: rollback before verify", response: `["build","test","deploy","rollback","verify"]`, want: 0},
		{name: "missing a step", response: `["build","test","deploy","verify"]`, want: 0},
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

// TestPlanMinimalReplanWant_UniqueOrder confirms, by brute-force
// enumeration, that planMinimalReplanWant is the only ordering of the 5
// remaining steps consistent with the requirement change's constraints.
func TestPlanMinimalReplanWant_UniqueOrder(t *testing.T) {
	steps := []planStep{
		{ID: "build"},
		{ID: "test", Deps: []string{"build"}},
		{ID: "deploy", Deps: []string{"test"}},
		{ID: "verify", Deps: []string{"deploy"}},
		{ID: "rollback", Deps: []string{"verify"}},
	}
	valid := planEnumerateTopoOrders(steps)
	if len(valid) != 1 {
		t.Fatalf("expected exactly 1 valid topological order, found %d: %v", len(valid), valid)
	}
	if !reflect.DeepEqual(valid[0], planMinimalReplanWant) {
		t.Errorf("unique valid order = %v, want %v", valid[0], planMinimalReplanWant)
	}
}

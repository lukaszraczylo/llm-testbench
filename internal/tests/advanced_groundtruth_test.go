package tests

import (
	"context"
	"testing"
)

// TestAdvancedGroundTruthScoresFull asserts each advanced test's own
// documented correct answer scores 1.0 through its real evaluator - a guard
// that the eval and the intended ground truth agree (a JSON typo or a wrong
// field path would silently make a test unwinnable).
func TestAdvancedGroundTruthScoresFull(t *testing.T) {
	golden := map[string]string{
		"adv-agent-plan-infeasible":          `{"feasible": false, "blocking_step": "fetch-deps"}`,
		"adv-agent-deleg-conflicting-policy": `{"agent": "agent-gpu-eu-fast"}`,
		"adv-agent-tool-adversarial-desc":    `{"tool": "trash_bin"}`,
		"adv-rag-forensics-stage":            `{"stage": "generation"}`,
		"adv-llm-context-budget":             `54`,
		"adv-pg-explain-forensics":           `{"cause": "seq-scan", "fix": "add-index"}`,
		"adv-sql-mvcc-anomaly":               `{"anomaly": "non-repeatable-read"}`,
		"adv-docker-layer-rebuild":           `["C","D","F"]`,
		"adv-rel-freeze-window":              `{"deferred_step": "deploy-prod"}`,
		"adv-scen-multilog-rootcause":        `{"root_cause": "oom"}`,
		"adv-linux-capacity-fill":            `7`,
		"adv-hard-composed-trace":            `32`,
		"adv-py-composed-mutable":            `{"first_element": 2, "second_element": 10}`,
		"adv-code-conflicting-evidence":      `{"port": 9090, "authoritative": "config"}`,
		"adv-sec-exploit-chain":              `{"vuln": "open-redirect"}`,
		"adv-sec-rotation-blast-radius":      `30`,
	}

	byID := map[string]int{}
	all := All().All()
	for i, tc := range all {
		byID[tc.ID] = i
	}

	for id, answer := range golden {
		idx, ok := byID[id]
		if !ok {
			t.Errorf("%s: not registered in catalog", id)
			continue
		}
		s := all[idx].Eval.Evaluate(context.Background(), answer)
		if s.Value != 1 {
			t.Errorf("%s: golden answer %q scored %.2f, want 1.00 (%s)", id, answer, s.Value, s.Detail)
		}
	}
}

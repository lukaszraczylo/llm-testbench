package tests

import (
	"testing"

	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// TestRegisterAgentsTests_Wiring checks registerAgentsTests wires up all
// three agents subcategories - tool-routing, planning, delegation - with
// 10 tests each, all categorized "agents", and no duplicate IDs across the
// three sibling files (testkit.Registry.Register panics on a duplicate ID,
// which would fail this test outright).
func TestRegisterAgentsTests_Wiring(t *testing.T) {
	r := testkit.NewRegistry()
	registerAgentsTests(r)

	const wantTotal = 30
	if r.Len() != wantTotal {
		t.Fatalf("registerAgentsTests: registry has %d tests, want %d", r.Len(), wantTotal)
	}

	counts := map[string]int{}
	for _, tc := range r.All() {
		if tc.Category != "agents" {
			t.Errorf("test %q: Category = %q, want %q", tc.ID, tc.Category, "agents")
		}
		if tc.Prompt == "" {
			t.Errorf("test %q: Prompt is empty", tc.ID)
		}
		if tc.Eval == nil {
			t.Errorf("test %q: Eval is nil", tc.ID)
		}
		if tc.Description == "" {
			t.Errorf("test %q: Description is empty", tc.ID)
		}
		counts[tc.Subcategory]++
	}
	for _, sub := range []string{"tool-routing", "planning", "delegation"} {
		if counts[sub] != 10 {
			t.Errorf("subcategory %q: got %d tests, want 10", sub, counts[sub])
		}
	}
}

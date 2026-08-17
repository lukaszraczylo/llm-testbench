package tests

import (
	"testing"

	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// TestRegisterDeliveryTests_Wiring checks registerDeliveryTests wires up all
// three delivery subcategories - git, containers, release-engineering - with
// 10 tests each, all categorized "delivery", and no duplicate IDs across the
// three sibling files (testkit.Registry.Register panics on a duplicate ID,
// which would fail this test outright). Mirrors
// TestRegisterAgentsTests_Wiring in agents_test.go; catalog.go's All() now
// also wires registerDeliveryTests directly (DC8), but this test still
// exercises the registration function in isolation from the other
// categories.
func TestRegisterDeliveryTests_Wiring(t *testing.T) {
	r := testkit.NewRegistry()
	registerDeliveryTests(r)

	const wantTotal = 30
	if r.Len() != wantTotal {
		t.Fatalf("registerDeliveryTests: registry has %d tests, want %d", r.Len(), wantTotal)
	}

	counts := map[string]int{}
	for _, tc := range r.All() {
		if tc.Category != "delivery" {
			t.Errorf("test %q: Category = %q, want %q", tc.ID, tc.Category, "delivery")
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
		if tc.MaxTokens != 0 {
			t.Errorf("test %q: MaxTokens = %d, want 0 (omitted, per round-2 rule 7)", tc.ID, tc.MaxTokens)
		}
		counts[tc.Subcategory]++
	}
	for _, sub := range []string{"git", "containers", "release-engineering"} {
		if counts[sub] != 10 {
			t.Errorf("subcategory %q: got %d tests, want 10", sub, counts[sub])
		}
	}
}

package tests

import (
	"testing"

	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// TestRegisterSecurityTests_Wiring checks registerSecurityTests wires up
// all three security subcategories - appsec, crypto, secrets - with 10
// tests each, all categorized "security", and no duplicate IDs across the
// three sibling files (testkit.Registry.Register panics on a duplicate ID,
// which would fail this test outright).
func TestRegisterSecurityTests_Wiring(t *testing.T) {
	r := testkit.NewRegistry()
	registerSecurityTests(r)

	const wantTotal = 30
	if r.Len() != wantTotal {
		t.Fatalf("registerSecurityTests: registry has %d tests, want %d", r.Len(), wantTotal)
	}

	counts := map[string]int{}
	for _, tc := range r.All() {
		if tc.Category != "security" {
			t.Errorf("test %q: Category = %q, want %q", tc.ID, tc.Category, "security")
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
		if tc.MaxTokens < 0 {
			t.Errorf("test %q: MaxTokens = %d, want >= 0", tc.ID, tc.MaxTokens)
		}
		counts[tc.Subcategory]++
	}
	for _, sub := range []string{"appsec", "crypto", "secrets"} {
		if counts[sub] != 10 {
			t.Errorf("subcategory %q: got %d tests, want 10", sub, counts[sub])
		}
	}
}

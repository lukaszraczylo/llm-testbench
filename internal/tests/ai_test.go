package tests

import (
	"testing"

	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// TestRegisterAITests_Wiring checks registerAITests wires up all three ai
// subcategories - vector-search, llm-integration, rag - with 10 tests
// each, all categorized "ai", and no duplicate IDs across the three
// sibling files (testkit.Registry.Register panics on a duplicate ID,
// which would fail this test outright). This is the integration point the
// orchestrator's catalog wiring depends on; catalog.go itself is not
// touched by this worktree.
func TestRegisterAITests_Wiring(t *testing.T) {
	r := testkit.NewRegistry()
	registerAITests(r)

	const wantTotal = 30
	if r.Len() != wantTotal {
		t.Fatalf("registerAITests: registry has %d tests, want %d", r.Len(), wantTotal)
	}

	counts := map[string]int{}
	for _, tc := range r.All() {
		if tc.Category != "ai" {
			t.Errorf("test %q: Category = %q, want %q", tc.ID, tc.Category, "ai")
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
	for _, sub := range []string{"vector-search", "llm-integration", "rag"} {
		if counts[sub] != 10 {
			t.Errorf("subcategory %q: got %d tests, want 10", sub, counts[sub])
		}
	}
}

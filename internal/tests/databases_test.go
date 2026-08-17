package tests

import (
	"strings"
	"testing"

	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// TestRegisterDatabasesTests_Wiring checks registerDatabasesTests wires up
// all three databases subcategories - postgres, redis, sql-tuning - with 10
// tests each, all categorized "databases", all with the expected ID prefix
// per subcategory, MaxTokens omitted (0, per the round-2 authoring rule),
// and no duplicate IDs across the three sibling files (testkit.Registry.
// Register panics on a duplicate ID, which would fail this test outright).
func TestRegisterDatabasesTests_Wiring(t *testing.T) {
	r := testkit.NewRegistry()
	registerDatabasesTests(r)

	const wantTotal = 30
	if r.Len() != wantTotal {
		t.Fatalf("registerDatabasesTests: registry has %d tests, want %d", r.Len(), wantTotal)
	}

	wantPrefix := map[string]string{
		"postgres":   "pg-",
		"redis":      "redis-",
		"sql-tuning": "sql-",
	}

	counts := map[string]int{}
	for _, tc := range r.All() {
		if tc.Category != "databases" {
			t.Errorf("test %q: Category = %q, want %q", tc.ID, tc.Category, "databases")
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
			t.Errorf("test %q: MaxTokens = %d, want 0 (omitted)", tc.ID, tc.MaxTokens)
		}
		if prefix, ok := wantPrefix[tc.Subcategory]; ok && !strings.HasPrefix(tc.ID, prefix) {
			t.Errorf("test %q: subcategory %q wants ID prefix %q", tc.ID, tc.Subcategory, prefix)
		}
		counts[tc.Subcategory]++
	}
	for _, sub := range []string{"postgres", "redis", "sql-tuning"} {
		if counts[sub] != 10 {
			t.Errorf("subcategory %q: got %d tests, want 10", sub, counts[sub])
		}
	}
}

package tests

import "testing"

// TestAll_Invariants checks structural invariants across the whole
// catalog, rather than an exact id/category/count map (fix 6): four other
// authors are adding roughly 113 more tests to internal/tests in parallel
// worktrees, so a fixed catalog size or an exhaustive id list would break
// on every integration merge for reasons unrelated to a real bug. An
// exact-catalog check belongs at integration time, once the merge is
// final, not in a file every parallel author's branch also compiles.
func TestAll_Invariants(t *testing.T) {
	r := All()
	all := r.All()

	if len(all) == 0 {
		t.Fatal("catalog is empty")
	}

	seenIDs := make(map[string]bool, len(all))
	for _, tc := range all {
		if tc.ID == "" {
			t.Error("found a test with an empty ID")
			continue
		}
		if seenIDs[tc.ID] {
			t.Errorf("duplicate test ID %q", tc.ID)
		}
		seenIDs[tc.ID] = true

		if tc.Category == "" {
			t.Errorf("%s: Category is empty", tc.ID)
		}
		if tc.Subcategory == "" {
			t.Errorf("%s: Subcategory is empty", tc.ID)
		}
		if tc.Prompt == "" {
			t.Errorf("%s: Prompt is empty", tc.ID)
		}
		if tc.Description == "" {
			t.Errorf("%s: Description is empty", tc.ID)
		}
		if tc.Eval == nil {
			t.Errorf("%s: Eval is nil", tc.ID)
		}
		// MaxTokens == 0 is valid: runner.Runner floors it with
		// cfg.MaxTokensDefault (max(test.MaxTokens, cfg.MaxTokensDefault)),
		// so a test need not set its own MaxTokens at all. Only a
		// negative value is a bug.
		if tc.MaxTokens < 0 {
			t.Errorf("%s: MaxTokens = %d, want >= 0", tc.ID, tc.MaxTokens)
		}
	}
}

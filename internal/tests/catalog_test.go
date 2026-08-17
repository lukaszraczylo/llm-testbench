package tests

import "testing"

// wantCatalog is the full 17-test catalog per PLAN.md, id -> {category,
// subcategory}. TestAll_MatchesCatalog checks the registry against this
// list exactly, so an accidental removal, duplication, or miscategorized
// test fails loudly.
var wantCatalog = map[string][2]string{
	"go-struct-align":           {"programming", "golang"},
	"go-worker-pool":            {"programming", "golang"},
	"go-semver-classify":        {"programming", "golang"},
	"py-log-triage":             {"programming", "python"},
	"py-cosine":                 {"programming", "python"},
	"ts-debounce-composable":    {"programming", "typescript"},
	"c-struct-size":             {"programming", "c"},
	"macos-timeout-portability": {"operations", "macos"},
	"macos-launchd-cron":        {"operations", "macos"},
	"linux-pct-exec":            {"operations", "linux"},
	"linux-systemd-oneshot":     {"operations", "linux"},
	"k8s-crashloop-gitops":      {"operations", "kubernetes"},
	"web-robots-ai-crawlers":    {"research", "web"},
	"paper-hnsw-params":         {"research", "whitepapers"},
	"code-trace-go":             {"research", "codebase"},
	"agent-tool-routing":        {"agents", "tool-routing"},
	"agent-plan-ordering":       {"agents", "planning"},
}

func TestAll_MatchesCatalog(t *testing.T) {
	r := All()

	if r.Len() != len(wantCatalog) {
		t.Fatalf("registry has %d tests, want %d", r.Len(), len(wantCatalog))
	}

	for id, catSub := range wantCatalog {
		tc, ok := r.Get(id)
		if !ok {
			t.Errorf("missing test %q", id)
			continue
		}
		if tc.Category != catSub[0] {
			t.Errorf("%s: Category = %q, want %q", id, tc.Category, catSub[0])
		}
		if tc.Subcategory != catSub[1] {
			t.Errorf("%s: Subcategory = %q, want %q", id, tc.Subcategory, catSub[1])
		}
		if tc.Prompt == "" {
			t.Errorf("%s: Prompt is empty", id)
		}
		if tc.Eval == nil {
			t.Errorf("%s: Eval is nil", id)
		}
		if tc.Description == "" {
			t.Errorf("%s: Description is empty", id)
		}
		if tc.MaxTokens <= 0 {
			t.Errorf("%s: MaxTokens = %d, want > 0", id, tc.MaxTokens)
		}
	}
}

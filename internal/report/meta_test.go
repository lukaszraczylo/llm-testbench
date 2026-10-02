package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lukaszraczylo/llm-testbench/internal/runner"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func TestCatalogHash_StableAndSensitive(t *testing.T) {
	a := testkit.Test{ID: "a", Prompt: "p1"}
	b := testkit.Test{ID: "b", Prompt: "p2"}
	if CatalogHash([]testkit.Test{a, b}) != CatalogHash([]testkit.Test{b, a}) {
		t.Error("hash depends on input order, want stable")
	}
	b2 := b
	b2.Prompt = "p2 changed"
	if CatalogHash([]testkit.Test{a, b}) == CatalogHash([]testkit.Test{a, b2}) {
		t.Error("hash ignores a prompt change")
	}
}

func TestCompareWarnings(t *testing.T) {
	seed := 1
	base := &RunMeta{CatalogHash: "x", Temperature: 0, MaxTokensDefault: 100, Fingerprints: map[string][]string{"m": {"fp1"}}}
	if w := compareWarnings(base, base); len(w) != 0 {
		t.Errorf("identical meta warnings = %v, want none", w)
	}
	if w := compareWarnings(nil, base); len(w) != 0 {
		t.Errorf("legacy artifact warnings = %v, want none", w)
	}
	cur := &RunMeta{CatalogHash: "y", Temperature: 0, MaxTokensDefault: 200, Seed: &seed, Interrupted: true,
		Fingerprints: map[string][]string{"m": {"fp2"}}}
	got := strings.Join(compareWarnings(base, cur), "\n")
	for _, want := range []string{"catalog differs", "max_tokens_default differs", "seed differs", "interrupted", "different backend build"} {
		if !strings.Contains(got, want) {
			t.Errorf("warnings missing %q:\n%s", want, got)
		}
	}
}

func TestWriteArtifact_AtomicWithMetaRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.json")
	if err := os.WriteFile(path, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []testkit.Test{{ID: "t1", Category: "c", Prompt: "p"}}
	results := []runner.Result{{Model: "m", TestID: "t1", Fingerprint: "fp"}}

	if err := WriteArtifact(path, RunMeta{Version: "v1", Repeat: 1}, tests, []string{"m"}, results); err != nil {
		t.Fatalf("WriteArtifact() error = %v", err)
	}
	art, err := LoadArtifact(path)
	if err != nil {
		t.Fatalf("LoadArtifact() error = %v", err)
	}
	if art.Meta == nil || art.Meta.Version != "v1" || art.Meta.TestCount != 1 || art.Meta.CatalogHash == "" {
		t.Errorf("Meta = %+v, want version, test count and catalog hash set", art.Meta)
	}
	if got := art.Meta.Fingerprints["m"]; len(got) != 1 || got[0] != "fp" {
		t.Errorf("Fingerprints = %v, want m:[fp]", art.Meta.Fingerprints)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("dir has %d entries after write, want only the artifact (no temp leftovers)", len(entries))
	}
}

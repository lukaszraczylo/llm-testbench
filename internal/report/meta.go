package report

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/runner"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// RunMeta records the settings and environment a run used, so two
// artifacts can be judged comparable and a result reproduced.
type RunMeta struct {
	Fingerprints     map[string][]string `json:"fingerprints,omitempty"`
	Seed             *int                `json:"seed,omitempty"`
	Version          string              `json:"version"`
	StartedAt        string              `json:"started_at"`
	Endpoint         string              `json:"endpoint,omitempty"`
	CatalogHash      string              `json:"catalog_hash"`
	Temperature      float64             `json:"temperature"`
	MaxTokensDefault int                 `json:"max_tokens_default"`
	Repeat           int                 `json:"repeat"`
	TestCount        int                 `json:"test_count"`
	Interrupted      bool                `json:"interrupted,omitempty"`
}

// CatalogHash returns a stable digest of the test inputs that determine a
// model's answer (id, system prompt, prompt, token floor, tool names).
func CatalogHash(tests []testkit.Test) string {
	sorted := append([]testkit.Test(nil), tests...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	h := sha256.New()
	for _, t := range sorted {
		names := make([]string, 0, len(t.Tools))
		for _, tl := range t.Tools {
			names = append(names, tl.Name)
		}
		_, _ = fmt.Fprintf(h, "%s\x00%s\x00%s\x00%d\x00%s\x01", t.ID, t.System, t.Prompt, t.MaxTokens, strings.Join(names, ","))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// servedFingerprints collects the distinct backend fingerprints each model
// answered with; more than one means the serving build changed mid-run.
func servedFingerprints(results []runner.Result) map[string][]string {
	seen := make(map[string]map[string]bool)
	for _, r := range results {
		if r.Fingerprint == "" {
			continue
		}
		if seen[r.Model] == nil {
			seen[r.Model] = make(map[string]bool)
		}
		seen[r.Model][r.Fingerprint] = true
	}
	if len(seen) == 0 {
		return nil
	}
	out := make(map[string][]string, len(seen))
	for m, fps := range seen {
		for fp := range fps {
			out[m] = append(out[m], fp)
		}
		sort.Strings(out[m])
	}
	return out
}

// compareWarnings lists reasons two artifacts may not be like-for-like.
// Artifacts without metadata (older runs) produce no warnings.
func compareWarnings(base, cur *RunMeta) []string {
	if base == nil || cur == nil {
		return nil
	}
	var w []string
	if base.CatalogHash != cur.CatalogHash {
		w = append(w, fmt.Sprintf("catalog differs (%s vs %s): prompts changed between runs", base.CatalogHash, cur.CatalogHash))
	}
	if base.Temperature != cur.Temperature {
		w = append(w, fmt.Sprintf("temperature differs (%g vs %g)", base.Temperature, cur.Temperature))
	}
	if base.MaxTokensDefault != cur.MaxTokensDefault {
		w = append(w, fmt.Sprintf("max_tokens_default differs (%d vs %d)", base.MaxTokensDefault, cur.MaxTokensDefault))
	}
	if fmtSeed(base.Seed) != fmtSeed(cur.Seed) {
		w = append(w, fmt.Sprintf("seed differs (%s vs %s)", fmtSeed(base.Seed), fmtSeed(cur.Seed)))
	}
	if base.Interrupted || cur.Interrupted {
		w = append(w, "an artifact is from an interrupted run: missing results are not regressions")
	}
	for m, cfps := range cur.Fingerprints {
		if bfps, ok := base.Fingerprints[m]; ok && strings.Join(bfps, ",") != strings.Join(cfps, ",") {
			w = append(w, fmt.Sprintf("%s served by a different backend build (%s vs %s)", m, strings.Join(bfps, "+"), strings.Join(cfps, "+")))
		}
	}
	sort.Strings(w)
	return w
}

func fmtSeed(s *int) string {
	if s == nil {
		return "unset"
	}
	return strconv.Itoa(*s)
}

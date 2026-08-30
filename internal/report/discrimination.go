package report

import (
	"math"
	"sort"

	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// DiscriminationSpread is the cross-model score gap at which a test starts
// to separate models. The last full run showed a median spread of 0.0
// across 250 tests: nearly every test scored >=0.99 for every model, so
// the suite measured nothing about relative capability. This number makes
// that visible per test instead of averaged away.
const DiscriminationSpread = 0.05

// testStat is the per-test measurement rollup: cross-model discrimination
// (spread, only meaningful with 2+ models), attempt stability (did
// --repeat samples disagree), and aggregate score. Computed over the
// attempt-aggregated cells so a flaky test shows as unstable rather than
// as a random single score.
type testStat struct {
	testID      string
	category    string
	subcategory string
	mean        float64 // mean over model cell means (scored cells only)
	spread      float64 // max-min over model cell means; NaN with <2 scored models
	unstable    bool    // some model's attempts disagreed beyond epsilon
	truncated   bool    // some cell was token-budget truncated
	scored      bool    // at least one scored cell
}

// unstableEpsilon absorbs float noise in averaged scores; a real
// disagreement between attempts is orders of magnitude larger.
const unstableEpsilon = 1e-9

// testStats computes the per-test rollup for every test in tests, ordered
// by category, subcategory, ID (same order as the score table).
func testStats(tests []testkit.Test, models []string, idx cellIndex) []testStat {
	out := make([]testStat, 0, len(tests))
	for _, t := range sortedTests(tests) {
		st := testStat{testID: t.ID, category: t.Category, subcategory: t.Subcategory, spread: math.NaN()}
		var sum float64
		var n int
		var lo, hi float64
		for _, model := range models {
			c, ok := idx[t.ID][model]
			if !ok || c.errorAll || c.skippedAll || c.scored == 0 {
				continue
			}
			v := c.result.Score.Value
			if n == 0 {
				lo, hi = v, v
			} else {
				lo = math.Min(lo, v)
				hi = math.Max(hi, v)
			}
			sum += v
			n++
			st.scored = true
			if c.maxScore-c.minScore > unstableEpsilon {
				st.unstable = true
			}
			st.truncated = st.truncated || c.result.Truncated()
		}
		if n > 1 {
			st.spread = hi - lo
			st.mean = sum / float64(n)
		} else if n == 1 {
			st.mean = sum
		}
		out = append(out, st)
	}
	return out
}

// interestingStats narrows testStats to rows worth reading: tests that
// discriminate between models, tests whose repeated attempts disagreed,
// and (when no cross-model data exists) tests that any model failed to
// pass outright. Sorted by spread descending, unstable first within equal
// spread.
func interestingStats(stats []testStat, multiModel bool) []testStat {
	var out []testStat
	for _, st := range stats {
		if !st.scored {
			continue
		}
		switch {
		case st.unstable:
		case multiModel && !math.IsNaN(st.spread) && st.spread >= DiscriminationSpread:
		case !multiModel && st.mean < passThreshold:
		default:
			continue
		}
		out = append(out, st)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].unstable != out[j].unstable {
			return out[i].unstable
		}
		si, sj := out[i].spread, out[j].spread
		if math.IsNaN(si) {
			si = -1
		}
		if math.IsNaN(sj) {
			sj = -1
		}
		if si != sj {
			return si > sj
		}
		return out[i].testID < out[j].testID
	})
	return out
}

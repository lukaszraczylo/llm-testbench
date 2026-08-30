package report

import (
	"fmt"
	"io"
	"sort"
)

// PerfectScore is the score at which a (model, test) cell counts as a
// perfect pass. Partial-credit evaluators score between 0 and 1; anything
// below this is not a perfect pass.
const PerfectScore = 0.999

// SaturationRate is the share of a test's scored cells that must be
// perfect for the test to count as saturated - passed by every model it
// was run against, every attempt. Saturated tests carry no signal about
// relative capability; they only detect outright regressions.
const SaturationRate = 0.9

// SubHealth is the suite-health rollup for one category/subcategory pair,
// pooled across one or more artifacts (so single-model probe runs can be
// combined with a full multi-model run). Cells are attempt-aggregated
// (model, test) means: a pair counts once, taking its WORST artifact -
// health asks "can any observed run fail this test?", so pooling over
// re-runs must not let a lucky second attempt hide a real failure.
type SubHealth struct {
	Category    string
	Subcategory string
	WeakTests   []string // non-saturated test IDs, sorted - the signal-carriers
	Tests       int      // distinct tests with at least one scored cell
	Cells       int      // scored (model, test) cells
	Perfect     int      // cells at PerfectScore or above
	Saturated   int      // tests perfect in >= SaturationRate of their cells
	Unstable    int      // tests whose repeated attempts disagreed
	Truncated   int      // tests with a truncated cell
}

// PerfectRate is the share of scored cells that are perfect.
func (h SubHealth) PerfectRate() float64 {
	if h.Cells == 0 {
		return 0
	}
	return float64(h.Perfect) / float64(h.Cells)
}

// HealthReport pools every subcategory present in the artifacts.
type HealthReport struct {
	Subcategories []SubHealth
	Models        []string
	Tests         int // distinct scored tests across all subcategories
	Cells         int
	Perfect       int
	Saturated     int
	Artifacts     int
}

// healthTestID scopes a test ID by model for cell pooling.
type healthCellID struct{ model, testID string }

// healthTest accumulates one test's cells across artifacts.
type healthTest struct {
	cells                 map[healthCellID]float64
	category, subcategory string
	unstable, truncated   bool
}

// AuditHealth pools artifacts into a per-subcategory health view.
// Errored and skipped attempts never form cells (meanAttempts already
// separates them by Status); truncated attempts follow the same
// exclusion policy as compare. Per-test unstable/truncated flags are
// OR-ed across artifacts: once flaky, always flagged.
func AuditHealth(artifacts []Artifact) HealthReport {
	agg := map[string]*healthTest{} // key: testID
	modelSet := map[string]bool{}

	for _, a := range artifacts {
		for _, c := range meanAttempts(a.Results) {
			if c.Status != "" && c.Status != "scored" {
				continue
			}
			modelSet[c.Model] = true
			ht := agg[c.TestID]
			if ht == nil {
				ht = &healthTest{category: c.Category, subcategory: c.Subcategory, cells: map[healthCellID]float64{}}
				if ht.category == "" {
					ht.category = "unknown"
				}
				if ht.subcategory == "" {
					ht.subcategory = "unknown"
				}
				agg[c.TestID] = ht
			}
			id := healthCellID{c.Model, c.TestID}
			if prev, ok := ht.cells[id]; !ok || c.BaselineMean < prev {
				// Pool the worst observed mean for this cell. On the
				// first artifact BaselineMean is the current run's mean.
				ht.cells[id] = c.BaselineMean
			}
		}
		for _, s := range a.Stats {
			if ht, ok := agg[s.TestID]; ok {
				ht.unstable = ht.unstable || s.Unstable
				ht.truncated = ht.truncated || s.Truncated
			}
		}
	}

	models := make([]string, 0, len(modelSet))
	for m := range modelSet {
		models = append(models, m)
	}
	sort.Strings(models)

	bySub := map[string]*SubHealth{}
	var tot, totCells, totPerfect, totSat int
	for testID, ht := range agg {
		sk := ht.category + "/" + ht.subcategory
		sh := bySub[sk]
		if sh == nil {
			sh = &SubHealth{Category: ht.category, Subcategory: ht.subcategory}
			bySub[sk] = sh
		}
		sh.Tests++
		tot++
		var perfect int
		for _, s := range ht.cells {
			sh.Cells++
			totCells++
			if s >= PerfectScore {
				perfect++
				sh.Perfect++
				totPerfect++
			}
		}
		if sh.Cells > 0 && ht.unstable {
			sh.Unstable++
		}
		if ht.truncated {
			sh.Truncated++
		}
		if float64(perfect)/float64(len(ht.cells)) >= SaturationRate {
			sh.Saturated++
			totSat++
		} else {
			sh.WeakTests = append(sh.WeakTests, testID)
		}
	}

	report := HealthReport{Artifacts: len(artifacts), Models: models, Tests: tot, Cells: totCells, Perfect: totPerfect, Saturated: totSat}
	for _, sh := range bySub {
		sort.Strings(sh.WeakTests)
		report.Subcategories = append(report.Subcategories, *sh)
	}
	sort.Slice(report.Subcategories, func(i, j int) bool {
		a, b := report.Subcategories[i], report.Subcategories[j]
		if a.PerfectRate() != b.PerfectRate() {
			return a.PerfectRate() > b.PerfectRate()
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		return a.Subcategory < b.Subcategory
	})
	return report
}

// weakCap bounds listed weak tests per subcategory in the text report;
// the counts stay accurate regardless.
const weakCap = 8

// RenderHealth writes the audit: headline numbers, then one line per
// subcategory sorted by perfect-cell rate descending (most saturated -
// least informative - first), listing its non-saturated tests.
func RenderHealth(w io.Writer, r HealthReport) error {
	ew := &errWriter{w: w}
	perfectRate := 0.0
	if r.Cells > 0 {
		perfectRate = float64(r.Perfect) / float64(r.Cells)
	}
	ew.println(fmt.Sprintf("== Suite health: %d tests, %d scored (model,test) cells over %d model(s), %d artifact(s) ==",
		r.Tests, r.Cells, len(r.Models), r.Artifacts))
	ew.println(fmt.Sprintf("perfect cells: %d/%d (%.1f%%) | saturated tests (no signal): %d/%d\n",
		r.Perfect, r.Cells, perfectRate*100, r.Saturated, r.Tests))
	if r.Saturated == r.Tests && r.Tests > 0 {
		ew.println("every scored test is saturated: the suite cannot rank these models at all.")
	}
	ew.println(fmt.Sprintf("%-30s %5s %6s %8s %9s %8s %8s  WEAK TESTS",
		"SUBCATEGORY", "TESTS", "CELLS", "PERFECT%", "SATURATED", "UNSTBL", "TRUNC"))
	for _, sh := range r.Subcategories {
		name := sh.Category + "/" + sh.Subcategory
		weak := ""
		if len(sh.WeakTests) > 0 {
			weak = fmt.Sprintf("%d", len(sh.WeakTests))
			if len(sh.WeakTests) > weakCap {
				weak += ": " + joinN(sh.WeakTests, weakCap) + ", ..."
			} else {
				weak += ": " + joinAll(sh.WeakTests)
			}
		}
		ew.println(fmt.Sprintf("%-30s %5d %6d %8.1f %9d %8d %8d  %s",
			name, sh.Tests, sh.Cells, sh.PerfectRate()*100, sh.Saturated, sh.Unstable, sh.Truncated, weak))
	}
	return ew.err
}

func joinN(xs []string, n int) string {
	if len(xs) <= n {
		return joinAll(xs)
	}
	return joinAll(xs[:n])
}

func joinAll(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}

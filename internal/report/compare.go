package report

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
)

// Comparison is the per-(model, test) diff between two saved artifacts.
// Scores are means over attempts, so a test that flapped between runs
// shows up as a regression on one attempt basis only if the mean moved.
type Comparison struct {
	Model        string
	TestID       string
	Category     string
	Subcategory  string
	Status       string
	BaselineMean float64
	CurrentMean  float64
	Delta        float64
}

// RegressionDelta is the mean-score drop at which a test counts as a
// regression. Below this, run-to-run sampling noise dominates.
const RegressionDelta = 0.01

type attemptKey struct{ model, testID string }

// meanAttempts collapses saved attempts into per-(model,test) means and
// carries category metadata from the first attempt that has it. Truncated
// attempts are excluded from the mean when any attempt finished normally,
// mirroring the truncation policy in indexResults; when every scored
// attempt was truncated, the partial-text scores are used rather than
// reporting no data.
func meanAttempts(results []AttemptResult) map[attemptKey]Comparison {
	type agg struct {
		cat      string
		sb       string
		sum      float64
		n        int
		cleanSum float64
		cleanN   int
		errs     int
	}
	by := make(map[attemptKey]*agg)
	order := make([]attemptKey, 0, len(results))
	for _, r := range results {
		k := attemptKey{r.Model, r.TestID}
		a, seen := by[k]
		if !seen {
			a = &agg{}
			by[k] = a
			order = append(order, k)
		}
		if a.cat == "" {
			a.cat, a.sb = r.Category, r.Subcategory
		}
		if r.Error != "" {
			a.errs++
			continue
		}
		if r.Skipped {
			continue
		}
		a.sum += r.Score
		a.n++
		if !r.Truncated {
			a.cleanSum += r.Score
			a.cleanN++
		}
	}

	out := make(map[attemptKey]Comparison, len(by))
	for _, k := range order {
		a := by[k]
		c := Comparison{Model: k.model, TestID: k.testID, Category: a.cat, Subcategory: a.sb}
		switch {
		case a.n == 0:
			c.Status = "error"
		case a.cleanN > 0:
			c.BaselineMean = a.cleanSum / float64(a.cleanN)
		default:
			c.BaselineMean = a.sum / float64(a.n)
		}
		out[k] = c
	}
	return out
}

// CompareArtifacts diffs two artifacts per (model, test), sorted by
// delta ascending (worst regressions first), then test ID.
func CompareArtifacts(baseline, current Artifact) []Comparison {
	base := meanAttempts(baseline.Results)
	cur := meanAttempts(current.Results)

	keys := make(map[attemptKey]bool, len(base)+len(cur))
	for k := range base {
		keys[k] = true
	}
	for k := range cur {
		keys[k] = true
	}

	out := make([]Comparison, 0, len(keys))
	for k := range keys {
		b, hasB := base[k]
		c, hasC := cur[k]
		switch {
		case hasB && !hasC:
			b.CurrentMean = math.NaN()
			b.Delta = math.NaN()
			b.Status = "baseline-only"
			out = append(out, b)
		case !hasB && hasC:
			c.BaselineMean = math.NaN()
			c.Delta = math.NaN()
			c.Status = "current-only"
			out = append(out, c)
		default:
			m := Comparison{
				Model:        k.model,
				TestID:       k.testID,
				Category:     c.Category,
				Subcategory:  c.Subcategory,
				BaselineMean: b.BaselineMean,
				CurrentMean:  c.BaselineMean,
			}
			switch {
			case b.Status == "error" || c.Status == "error":
				m.Status = "error"
				m.Delta = math.NaN()
			default:
				m.Delta = m.CurrentMean - m.BaselineMean
				switch {
				case m.Delta <= -RegressionDelta:
					m.Status = "regression"
				case m.Delta >= RegressionDelta:
					m.Status = "improvement"
				default:
					m.Status = "stable"
				}
			}
			out = append(out, m)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		di, dj := out[i].Delta, out[j].Delta
		if math.IsNaN(di) != math.IsNaN(dj) {
			return math.IsNaN(di)
		}
		if math.IsNaN(di) && math.IsNaN(dj) {
			if out[i].Status != out[j].Status {
				return out[i].Status < out[j].Status
			}
			return out[i].TestID < out[j].TestID
		}
		if di != dj {
			return di < dj
		}
		if out[i].TestID != out[j].TestID {
			return out[i].TestID < out[j].TestID
		}
		return out[i].Model < out[j].Model
	})
	return out
}

// RenderCompare writes a comparison report: headline counts, then only
// changed/missing rows (stable rows are summarized, not listed).
func RenderCompare(w io.Writer, comparisons []Comparison) error {
	ew := &errWriter{w: w}

	var reg, imp, stable, added, removed, errored int
	for _, c := range comparisons {
		switch c.Status {
		case "regression":
			reg++
		case "improvement":
			imp++
		case "baseline-only":
			removed++
		case "current-only":
			added++
		case "error":
			errored++
		default:
			stable++
		}
	}

	ew.println(fmt.Sprintf("== Compare: %d regressions, %d improvements, %d stable, %d errors, %d baseline-only, %d current-only ==",
		reg, imp, stable, errored, removed, added))
	ew.println()
	ew.println("MODEL\tTEST\tCATEGORY\tSUBCATEGORY\tBASELINE\tCURRENT\tDELTA\tSTATUS")
	shown := 0
	for _, c := range comparisons {
		if c.Status == "stable" {
			continue
		}
		ew.println(strings.Join([]string{
			c.Model, c.TestID, c.Category, c.Subcategory,
			meanCellText(c.BaselineMean), meanCellText(c.CurrentMean),
			deltaText(c.Delta), c.Status,
		}, "\t"))
		shown++
	}
	if shown == 0 {
		ew.println("(no changed, missing, or erroring tests)")
	}
	return ew.err
}

func deltaText(v float64) string {
	if math.IsNaN(v) {
		return "N/A"
	}
	return fmt.Sprintf("%+.2f", v)
}

package tests

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
)

func TestCosineSimilarity_KnownCases(t *testing.T) {
	// Grounds the cosineSimilarity helper itself against two textbook
	// cases before trusting it to derive pyCosineWant.
	tests := []struct {
		name string
		a, b []float64
		want float64
	}{
		{"identical vectors", []float64{1, 2, 3}, []float64{1, 2, 3}, 1},
		{"orthogonal vectors", []float64{1, 0}, []float64{0, 1}, 0},
		{"opposite vectors", []float64{1, 0}, []float64{-1, 0}, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("cosineSimilarity(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestPyCosineWant_GroundTruth(t *testing.T) {
	// Independent re-derivation via math.Sqrt, not via the cosineSimilarity
	// helper under test, per PLAN.md's rule to recompute cheap ground
	// truths in the unit test.
	a, b := pyCosineVectorA, pyCosineVectorB
	var dot, sumA, sumB float64
	for i := range a {
		dot += a[i] * b[i]
		sumA += a[i] * a[i]
		sumB += b[i] * b[i]
	}
	want := dot / (math.Sqrt(sumA) * math.Sqrt(sumB))
	wantRounded := math.Round(want*10000) / 10000

	if pyCosineWant != wantRounded {
		t.Errorf("pyCosineWant = %v, independently recomputed = %v", pyCosineWant, wantRounded)
	}
}

func TestPyLogTriageWant_GroundTruth(t *testing.T) {
	// Independent re-derivation of the expected counts by scanning the raw
	// log text directly, rather than trusting the hand count in the
	// doc comment.
	lines := strings.Split(pyLogTriageLog, "\n")
	if len(lines) != 15 {
		t.Fatalf("pyLogTriageLog has %d lines, want 15", len(lines))
	}

	oom := strings.Count(pyLogTriageLog, "OOMKilled")
	pull := strings.Count(pyLogTriageLog, "ImagePullBackOff")
	if oom != 5 {
		t.Errorf("OOMKilled count = %d, want 5", oom)
	}
	if pull != 4 {
		t.Errorf("ImagePullBackOff count = %d, want 4", pull)
	}

	want := "ImagePullBackOff=4\nOOMKilled=5"
	if pyLogTriageWant != want {
		t.Errorf("pyLogTriageWant = %q, want %q", pyLogTriageWant, want)
	}
}

func TestPyLogTriageTest_Eval(t *testing.T) {
	tc := pyLogTriageTest()

	correctScript := "```python\n" + `counts = {"OOMKilled": 0, "ImagePullBackOff": 0}
log = """` + pyLogTriageLog + `"""
for line in log.splitlines():
    for key in counts:
        if key in line:
            counts[key] += 1
for key in sorted(counts):
    print(f"{key}={counts[key]}")
` + "```"

	wrongScript := "```python\nprint('OOMKilled=1')\nprint('ImagePullBackOff=1')\n```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct triage script", correctScript, 1},
		{"wrong counts", wrongScript, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Skipped {
				t.Skip("python3 not available, skipping exec-evaluator assertion")
			}
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestSoftmaxFirst_KnownCases(t *testing.T) {
	// Grounds the softmaxFirst helper itself before trusting it to derive
	// pySoftmaxWant.
	tests := []struct {
		name string
		v    []float64
		want float64
	}{
		{"uniform vector splits evenly", []float64{1, 1, 1}, 1.0 / 3.0},
		{"single element gets all the mass", []float64{5}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := softmaxFirst(tt.v)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("softmaxFirst(%v) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

func TestPySoftmaxWant_GroundTruth(t *testing.T) {
	// Independent re-derivation via math.Exp, not via the softmaxFirst
	// helper under test, per PLAN.md's rule to recompute cheap ground
	// truths in the unit test.
	v := pySoftmaxVector
	var sum float64
	for _, x := range v {
		sum += math.Exp(x)
	}
	want := math.Exp(v[0]) / sum
	wantRounded := math.Round(want*10000) / 10000

	if pySoftmaxWant != wantRounded {
		t.Errorf("pySoftmaxWant = %v, independently recomputed = %v", pySoftmaxWant, wantRounded)
	}
}

func TestPySoftmaxTest_Eval(t *testing.T) {
	tc := pySoftmaxTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "0.5585", 1},
		{"within tolerance", "0.5588", 1},
		{"prose wrapped", "The softmax probability is 0.5585.", 1},
		{"outside tolerance", "0.2", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestPyAsyncioGatherTrace_GroundTruth(t *testing.T) {
	// Runs the identical Python snippet embedded in pyAsyncioGatherTraceCode
	// via python3 and checks the print order is exactly B, A, C - the same
	// oracle mechanism as eval.PyRun, independent of the doc-comment
	// reasoning in python.go.
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found on PATH, skipping ground-truth cross-check")
	}

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "gather.py")
	if err := os.WriteFile(scriptPath, []byte(pyAsyncioGatherTraceCode), 0o600); err != nil {
		t.Fatalf("write gather.py: %v", err)
	}

	// #nosec G204 -- scriptPath is a fixed filename this test just wrote
	// into its own t.TempDir(); not external input.
	out, err := exec.Command("python3", scriptPath).Output() //nolint:gosec // see comment above
	if err != nil {
		t.Fatalf("running gather.py failed: %v", err)
	}

	got := strings.TrimSpace(string(out))
	want := "B\nA\nC"
	if got != want {
		t.Errorf("python3 gather.py stdout = %q, want %q", got, want)
	}
}

func TestPyAsyncioGatherTraceTest_Eval(t *testing.T) {
	tc := pyAsyncioGatherTraceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", `["B","A","C"]`, 1},
		{"correct fenced json", "```json\n[\"B\", \"A\", \"C\"]\n```", 1},
		{"correct with prose wrapper", `The order is: ["B","A","C"]`, 1},
		{"wrong: source order, ignores delays", `["A","B","C"]`, 0},
		{"wrong: reversed", `["C","A","B"]`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestPyMutableDefaultArg_GroundTruth runs the identical snippet via PyRun
// with a passthrough harness, so the doc-comment claim in python.go is
// cross-checked against the real python3 interpreter, not just reasoning.
func TestPyMutableDefaultArg_GroundTruth(t *testing.T) {
	score := eval.PyRun(eval.PassthroughHarness, "[1, 2, 3]").Evaluate(
		context.Background(), "```python\n"+pyMutableDefaultArgCode+"\n```",
	)
	if score.Skipped {
		t.Skip("python3 not available, skipping ground-truth cross-check")
	}
	if score.Value != 1 {
		t.Errorf("running pyMutableDefaultArgCode via PyRun scored %v, want 1 (detail: %s)", score.Value, score.Detail)
	}
}

func TestPyMutableDefaultArgTest_Eval(t *testing.T) {
	tc := pyMutableDefaultArgTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "[1, 2, 3]", 1},
		{"correct with surrounding whitespace", "  [1, 2, 3]  ", 1},
		{"correct with trailing newline", "[1, 2, 3]\n", 1},
		// A7 regression: fenced/quoted decoration on the correct answer
		// must still score full credit.
		{"fenced", "```\n[1, 2, 3]\n```", 1},
		{"quoted", `"[1, 2, 3]"`, 1},
		{"wrong: assumes a fresh list per call", "[3]", 0},
		{"wrong: only counts explicit-arg calls", "[1, 2]", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestPyDictComprehensionTrace_GroundTruth runs the identical snippet via
// PyRun with a passthrough harness.
func TestPyDictComprehensionTrace_GroundTruth(t *testing.T) {
	want := "[('apple', 5), ('banana', 6), ('cherry', 6), ('date', 4)]"
	score := eval.PyRun(eval.PassthroughHarness, want).Evaluate(
		context.Background(), "```python\n"+pyDictComprehensionTraceCode+"\n```",
	)
	if score.Skipped {
		t.Skip("python3 not available, skipping ground-truth cross-check")
	}
	if score.Value != 1 {
		t.Errorf("running pyDictComprehensionTraceCode via PyRun scored %v, want 1 (detail: %s)", score.Value, score.Detail)
	}
}

func TestPyDictComprehensionTraceTest_Eval(t *testing.T) {
	tc := pyDictComprehensionTraceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "[('apple', 5), ('banana', 6), ('cherry', 6), ('date', 4)]", 1},
		{"correct with surrounding whitespace", "  [('apple', 5), ('banana', 6), ('cherry', 6), ('date', 4)]  ", 1},
		{"correct uppercase-folded", "[('APPLE', 5), ('BANANA', 6), ('CHERRY', 6), ('DATE', 4)]", 1},
		// A7 regression: fenced decoration on the correct answer must
		// still score full credit.
		{"fenced", "```\n[('apple', 5), ('banana', 6), ('cherry', 6), ('date', 4)]\n```", 1},
		{"wrong: includes fig, forgets the length filter", "[('apple', 5), ('banana', 6), ('cherry', 6), ('date', 4), ('fig', 3)]", 0},
		{"wrong: unsorted, dict insertion order", "[('apple', 5), ('banana', 6), ('cherry', 6), ('fig', 3), ('date', 4)]", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestPyGeneratorExhaustionTrace_GroundTruth runs the identical snippet via
// PyRun with a passthrough harness.
func TestPyGeneratorExhaustionTrace_GroundTruth(t *testing.T) {
	score := eval.PyRun(eval.PassthroughHarness, "[0, 1, 2] []").Evaluate(
		context.Background(), "```python\n"+pyGeneratorExhaustionTraceCode+"\n```",
	)
	if score.Skipped {
		t.Skip("python3 not available, skipping ground-truth cross-check")
	}
	if score.Value != 1 {
		t.Errorf("running pyGeneratorExhaustionTraceCode via PyRun scored %v, want 1 (detail: %s)", score.Value, score.Detail)
	}
}

func TestPyGeneratorExhaustionTraceTest_Eval(t *testing.T) {
	tc := pyGeneratorExhaustionTraceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "[0, 1, 2] []", 1},
		{"correct with surrounding whitespace", "  [0, 1, 2] []  ", 1},
		{"correct with trailing newline", "[0, 1, 2] []\n", 1},
		// A7 regression: fenced/quoted decoration on the correct answer
		// must still score full credit.
		{"fenced", "```\n[0, 1, 2] []\n```", 1},
		{"quoted", `"[0, 1, 2] []"`, 1},
		{"wrong: assumes the generator restarts", "[0, 1, 2] [0, 1, 2]", 0},
		{"wrong: assumes an error is raised and printed", "StopIteration", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestPyPathlibRewriteTest_Eval(t *testing.T) {
	tc := pyPathlibRewriteTest()

	goodRewrite := "```python\n" + `from pathlib import Path


def find_configs(base_dir):
    return [str(p) for p in Path(base_dir).rglob("*.yaml")]
` + "```"

	goodRewriteAltImport := "```python\n" + `import pathlib


def find_configs(base_dir):
    return [str(p) for p in pathlib.Path(base_dir).rglob("*.yaml")]
` + "```"

	stillUsesOsPath := "```python\n" + pyOsPathCode + "\n```"

	missingRglob := "```python\n" + `from pathlib import Path


def find_configs(base_dir):
    return [str(p) for p in Path(base_dir).glob("*.yaml")]
` + "```"

	goodRewriteGlobRecursiveWildcard := "```python\n" + `from pathlib import Path


def find_configs(base_dir):
    return [str(p) for p in Path(base_dir).glob("**/*.yaml")]
` + "```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"good rewrite: from pathlib import Path", goodRewrite, 1},
		{"good rewrite: import pathlib", goodRewriteAltImport, 1},
		{"wrong: unchanged os.path/os.walk code", stillUsesOsPath, 0},
		{"partial: uses pathlib but non-recursive glob, not rglob", missingRglob, 2.0 / 3.0}, // (1*1 + 1*0 + 1*1)/3: pathlib import ok, neither rglob nor glob("**/ present, *.yaml ok
		{"good rewrite: glob(\"**/*.yaml\") is behaviorally equivalent to rglob (A16 bug probe)", goodRewriteGlobRecursiveWildcard, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestPyJSONTransformWant_GroundTruth(t *testing.T) {
	// Independent re-derivation of the expected per-department totals by
	// summing hardcoded literals matching pyJSONTransformRecords, rather
	// than trusting the hand sum in the doc comment.
	totals := map[string]int{
		"eng":   95000 + 105000 + 88000,
		"sales": 60000 + 58000,
		"ops":   72000,
	}
	want := "eng=288000\nops=72000\nsales=118000"
	if pyJSONTransformWant != want {
		t.Errorf("pyJSONTransformWant = %q, want %q", pyJSONTransformWant, want)
	}
	if totals["eng"] != 288000 || totals["sales"] != 118000 || totals["ops"] != 72000 {
		t.Errorf("independently recomputed totals = %v, want eng=288000 sales=118000 ops=72000", totals)
	}
}

func TestPyJSONTransformTest_Eval(t *testing.T) {
	tc := pyJSONTransformTest()

	correctScript := "```python\n" + pyJSONTransformRecords + `

totals = {}
for r in records:
    totals[r["dept"]] = totals.get(r["dept"], 0) + r["salary"]

for dept in sorted(totals):
    print(f"{dept}={totals[dept]}")
` + "```"

	wrongScript := "```python\nprint('eng=1')\nprint('ops=1')\nprint('sales=1')\n```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct transform script", correctScript, 1},
		{"wrong totals", wrongScript, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Skipped {
				t.Skip("python3 not available, skipping exec-evaluator assertion")
			}
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestPyRegexLogExtractionWant_GroundTruth(t *testing.T) {
	// Independent re-derivation of the expected counts by scanning the raw
	// log text directly with Go's regexp, rather than trusting the hand
	// count in the doc comment.
	statusPattern := regexp.MustCompile(`"\s*(\d{3})\s+\d+`)
	matches := statusPattern.FindAllStringSubmatch(pyRegexLogLines, -1)
	if len(matches) != 7 {
		t.Fatalf("found %d status codes in pyRegexLogLines, want 7", len(matches))
	}
	counts := map[string]int{}
	for _, m := range matches {
		counts[m[1]]++
	}
	if counts["200"] != 3 || counts["404"] != 2 || counts["500"] != 2 {
		t.Errorf("independently recomputed counts = %v, want 200=3 404=2 500=2", counts)
	}
	want := "200=3\n404=2\n500=2"
	if pyRegexLogExtractionWant != want {
		t.Errorf("pyRegexLogExtractionWant = %q, want %q", pyRegexLogExtractionWant, want)
	}
}

func TestPyRegexLogExtractionTest_Eval(t *testing.T) {
	tc := pyRegexLogExtractionTest()

	correctScript := "```python\nimport re\n\n" + pyRegexLogLines + `

pattern = re.compile(r'"\s*(\d{3})\s+\d+$')
counts = {}
for line in log_lines:
    m = pattern.search(line)
    if m:
        code = m.group(1)
        counts[code] = counts.get(code, 0) + 1

for code in sorted(counts):
    print(f"{code}={counts[code]}")
` + "```"

	wrongScript := "```python\nprint('200=1')\nprint('404=1')\nprint('500=1')\n```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct regex extraction script", correctScript, 1},
		{"wrong counts", wrongScript, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Skipped {
				t.Skip("python3 not available, skipping exec-evaluator assertion")
			}
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestPyCosineTest_Eval(t *testing.T) {
	tc := pyCosineTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "0.9896", 1},
		{"within tolerance", "0.9899", 1},
		{"prose wrapped", "The cosine similarity is 0.9896.", 1},
		{"outside tolerance", "0.5", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

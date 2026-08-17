package tests

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCodeExactAnswer(t *testing.T) {
	ev := codeExactAnswer("efabcd")

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"bare correct", "efabcd", 1},
		{"double-quoted correct", `"efabcd"`, 1},
		{"different case", "EFABCD", 1},
		{"trailing period", "efabcd.", 1},
		{"wrong string", "abcdef", 0},
		{"correct but truncated", "efabc", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ev.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestCodeTracePythonWant_GroundTruth(t *testing.T) {
	// Hand-derivation, double-checked: len("abcdef")=6 is even at every
	// step, so every call takes the rotate branch (s[1:]+s[0]) rather than
	// the reverse branch.
	s := "abcdef"
	for i := 0; i < 4; i++ {
		if len(s)%2 == 0 {
			s = s[1:] + string(s[0])
		} else {
			for l, r := 0, len(s)-1; l < r; l, r = l+1, r-1 {
				b := []byte(s)
				b[l], b[r] = b[r], b[l]
				s = string(b)
			}
		}
	}
	if s != codeTracePythonWant {
		t.Errorf("hand-derived trace = %q, want %q", s, codeTracePythonWant)
	}

	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found on PATH, skipping exec ground-truth cross-check")
	}

	script := codeTracePythonSource + "\nprint(transform(\"abcdef\", 4))\n"
	// #nosec G204 -- script is a fixed, hardcoded string built from the
	// same codeTracePythonSource constant embedded in the prompt, not
	// external or response-controlled input.
	cmd := exec.Command("python3", "-c", script) //nolint:gosec // see comment above
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("running python3 ground-truth script failed: %v", err)
	}
	got := strings.TrimSpace(stdout.String())
	if got != codeTracePythonWant {
		t.Errorf("python3-computed transform(\"abcdef\", 4) = %q, want %q", got, codeTracePythonWant)
	}
}

func TestCodeTracePythonTest_Eval(t *testing.T) {
	tc := codeTracePythonTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "efabcd", 1},
		{"correct quoted", `"efabcd"`, 1},
		{"wrong: forgot final rotation", "defabc", 0},
		{"wrong: reversed instead of rotated", "fedcba", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestCodeTraceTSWant_GroundTruth(t *testing.T) {
	// Hand-derivation, double-checked against codeTSPackTrace: acc=0;
	// i=0,x=3 (even index) -> 3; i=1,x=4 (odd) -> 3+8=11; i=2,x=5 (even) ->
	// 16; i=3,x=6 (odd) -> 16+12=28; i=4,x=7 (even) -> 35. 35 = 32+2+1 in
	// binary is 100011.
	acc := 0
	items := []int{3, 4, 5, 6, 7}
	for i, x := range items {
		if i%2 == 0 {
			acc += x
		} else {
			acc += x * 2
		}
	}
	if acc != 35 {
		t.Fatalf("hand-derived acc = %d, want 35", acc)
	}

	got := codeTSPackTrace(items)
	if got != codeTraceTSWant {
		t.Errorf("codeTSPackTrace(%v) = %q, want %q", items, got, codeTraceTSWant)
	}

	// BN4: cross-check against a real JS engine too, not just the Go
	// replica above. node cannot execute codeTraceTSSource's TypeScript
	// type annotations directly, so this is the same logic with the two
	// annotations (": number[]", ": string") stripped - everything else,
	// including the reduce/ternary/toString(2) logic under test, is
	// identical source.
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found on PATH, skipping exec ground-truth cross-check")
	}

	const jsSource = `function pack(items) {
  return items.reduce((acc, x, i) => {
    if (i % 2 === 0) return acc + x;
    return acc + x * 2;
  }, 0).toString(2);
}
console.log(pack([3, 4, 5, 6, 7]));
`
	// #nosec G204 -- jsSource is a fixed, hardcoded string (the TS source
	// embedded in the prompt with its two type annotations stripped), not
	// external or response-controlled input.
	cmd := exec.Command("node", "-e", jsSource) //nolint:gosec // see comment above
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("running node ground-truth script failed: %v", err)
	}
	nodeGot := strings.TrimSpace(stdout.String())
	if nodeGot != codeTraceTSWant {
		t.Errorf("node-computed pack([3,4,5,6,7]) = %q, want %q", nodeGot, codeTraceTSWant)
	}
}

func TestCodeTraceTSTest_Eval(t *testing.T) {
	tc := codeTraceTSTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "100011", 1},
		{"correct quoted", `"100011"`, 1},
		{"wrong: doubled every element instead of alternating", "1000110", 0},
		{"wrong: decimal instead of binary", "35", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

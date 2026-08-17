package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCStructSizeWant_GroundTruth cross-checks cStructSizeWant against the
// real cc toolchain's sizeof(struct Config), rather than trusting the
// hand-derived alignment arithmetic in the doc comment alone. Skips (does
// not fail) when cc is unavailable, matching the exec-evaluator contract.
func TestCStructSizeWant_GroundTruth(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not found on PATH, skipping ground-truth cross-check")
	}

	const src = `#include <stdio.h>
struct Config {
	char flag;
	double ratio;
	int count;
	short mode;
};
int main(void) {
	printf("%zu", sizeof(struct Config));
	return 0;
}
`
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "main.c")
	if err := os.WriteFile(srcPath, []byte(src), 0o600); err != nil {
		t.Fatalf("write main.c: %v", err)
	}
	binPath := filepath.Join(dir, "prog")

	// #nosec G204 -- compiling a fixed, hardcoded C source (src above) in a
	// t.TempDir() this test just created; not external input.
	build := exec.Command("cc", "-o", binPath, srcPath) //nolint:gosec // see comment above
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cc failed: %v\n%s", err, out)
	}

	// #nosec G204 -- binPath is the binary this test just compiled above,
	// in its own t.TempDir(); not an externally supplied executable path.
	out, err := exec.Command(binPath).Output() //nolint:gosec // see comment above
	if err != nil {
		t.Fatalf("running compiled program failed: %v", err)
	}

	got := strings.TrimSpace(string(out))
	if got != "24" {
		t.Errorf("cc-computed sizeof(struct Config) = %s, want 24 (matches cStructSizeWant)", got)
	}
	if cStructSizeWant != 24 {
		t.Errorf("cStructSizeWant = %d, want 24", cStructSizeWant)
	}
}

func TestCStructSizeTest_Eval(t *testing.T) {
	tc := cStructSizeTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "24", 1},
		{"prose wrapped", "sizeof(struct Config) is 24 bytes", 1},
		{"unpadded sum is wrong", "15", 0},
		{"off by one", "23", 0},
		// S6 regression: the trailing "64-bit" qualifier must not be
		// mistaken for the answer.
		{"answer followed by a 64-bit qualifier", "sizeof(struct Config) is 24 bytes on a 64-bit system", 1},
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

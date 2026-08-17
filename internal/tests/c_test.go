package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ccRun compiles and runs a complete C program via cc, returning trimmed
// stdout. It skips the calling test (does not fail it) when cc is
// unavailable, mirroring TestCStructSizeWant_GroundTruth's existing
// toolchain-detection contract.
func ccRun(t *testing.T, src string) string {
	t.Helper()
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not found on PATH, skipping ground-truth cross-check")
	}

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "main.c")
	if err := os.WriteFile(srcPath, []byte(src), 0o600); err != nil {
		t.Fatalf("write main.c: %v", err)
	}
	binPath := filepath.Join(dir, "prog")

	// #nosec G204 -- srcPath/binPath are fixed filenames this test just
	// wrote/will write into its own t.TempDir(); not external input.
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
	return strings.TrimSpace(string(out))
}

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

func TestCPointerArithmetic_GroundTruth(t *testing.T) {
	src := `#include <stdio.h>
int main(void) {
	` + cPointerArithmeticCode + `
	printf("%d", val);
	return 0;
}
`
	got := ccRun(t, src)
	if want := "43"; got != want {
		t.Errorf("cc output = %q, want %q", got, want)
	}
}

func TestCPointerArithmeticTest_Eval(t *testing.T) {
	tc := cPointerArithmeticTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "43", 1},
		{"prose wrapped", "val is 43", 1},
		{"answer followed by qualifier", "val = 43 on a 64-bit system", 1},
		{"wrong: forgets pointer arithmetic scales by element size", "13", 0},
		{"wrong: byte-based pointer diff", "val = 52", 0},
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

func TestCBitmaskOps_GroundTruth(t *testing.T) {
	src := `#include <stdio.h>
int main(void) {
	` + cBitmaskOpsCode + `
	printf("%u", flags);
	return 0;
}
`
	got := ccRun(t, src)
	if want := "33"; got != want {
		t.Errorf("cc output = %q, want %q", got, want)
	}
}

func TestCBitmaskOpsTest_Eval(t *testing.T) {
	tc := cBitmaskOpsTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "33", 1},
		{"prose wrapped", "flags is 33", 1},
		{"binary explained then decimal answer", "0b100001 = 33", 1},
		{"wrong: forgets to clear bit 2", "37", 0},
		{"wrong: forgets to toggle bit 0", "32", 0},
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

func TestCUnionSizeLP64_GroundTruth(t *testing.T) {
	src := `#include <stdio.h>
` + cUnionSizeLP64Code + `
int main(void) {
	printf("%zu", sizeof(union Packet));
	return 0;
}
`
	got := ccRun(t, src)
	if want := "8"; got != want {
		t.Errorf("cc output = %q, want %q", got, want)
	}
}

func TestCUnionSizeLP64Test_Eval(t *testing.T) {
	tc := cUnionSizeLP64Test()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "8", 1},
		{"prose wrapped", "sizeof(union Packet) is 8 bytes", 1},
		{"answer followed by 64-bit qualifier", "8 bytes on a 64-bit system", 1},
		{"wrong: sums members instead of taking the max", "19", 0},
		{"wrong: forgets struct-member alignment padding", "6", 0},
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

// TestCStringFunctionOutputWant_GroundTruth independently re-derives the
// expected count by scanning the same literal key list with Go's
// strings.HasPrefix, rather than trusting the hand count in the doc
// comment.
func TestCStringFunctionOutputWant_GroundTruth(t *testing.T) {
	keys := []string{"srv_web", "srv_db", "cache_1", "srv_api", "queue_2", "srv_auth"}
	count := 0
	for _, k := range keys {
		if strings.HasPrefix(k, "srv_") {
			count++
		}
	}
	if count != 4 {
		t.Errorf("independently recomputed count = %d, want 4", count)
	}
	if cStringFunctionOutputWant != "4" {
		t.Errorf("cStringFunctionOutputWant = %q, want %q", cStringFunctionOutputWant, "4")
	}
}

func TestCStringFunctionOutputTest_Eval(t *testing.T) {
	tc := cStringFunctionOutputTest()

	correctProgram := "```c\n" + `#include <stdio.h>
#include <string.h>

int main(void) {
	const char *keys[] = {` + cStringFunctionOutputKeys + `};
	int count = 0;
	for (int i = 0; i < 6; i++) {
		if (strncmp(keys[i], "srv_", 4) == 0) {
			count++;
		}
	}
	printf("%d\n", count);
	return 0;
}
` + "```"

	wrongProgram := "```c\n#include <stdio.h>\nint main(void) { printf(\"6\\n\"); return 0; }\n```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct strncmp counting program", correctProgram, 1},
		{"wrong: counts every key regardless of prefix", wrongProgram, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Skipped {
				t.Skip("cc not available, skipping exec-evaluator assertion")
			}
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestCIntegerPromotionOverflow_GroundTruth(t *testing.T) {
	src := `#include <stdio.h>
int main(void) {
	` + cIntegerPromotionOverflowCode + `
	printf("%d", sum);
	return 0;
}
`
	got := ccRun(t, src)
	if want := "44"; got != want {
		t.Errorf("cc output = %q, want %q", got, want)
	}
}

func TestCIntegerPromotionOverflowTest_Eval(t *testing.T) {
	tc := cIntegerPromotionOverflowTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "44", 1},
		{"prose wrapped", "sum is 44", 1},
		{"answer with modulo explanation", "300 mod 256 = 44", 1},
		{"wrong: assumes saturation instead of wraparound", "255", 0},
		{"wrong: forgets truncation back to 8 bits", "300", 0},
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

func TestCArrayDecaySizeof_GroundTruth(t *testing.T) {
	src := `#include <stdio.h>

` + cArrayDecaySizeofCode + `
`
	got := ccRun(t, src)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("cc output = %q, want 2 lines", got)
	}
	if lines[0] != "40" {
		t.Errorf("sizeof(arr) in main = %q, want %q", lines[0], "40")
	}
	if lines[1] != "8" {
		t.Errorf("sizeof(arr) in function = %q, want %q", lines[1], "8")
	}
	if cArrayDecaySizeofWant != 48 {
		t.Errorf("cArrayDecaySizeofWant = %d, want 48 (= 40 + 8)", cArrayDecaySizeofWant)
	}
}

func TestCArrayDecaySizeofTest_Eval(t *testing.T) {
	tc := cArrayDecaySizeofTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "48", 1},
		{"prose wrapped", "The sum is 48.", 1},
		{"answer with breakdown", "40 + 8 = 48", 1},
		{"wrong: assumes both report the full array size", "80", 0},
		{"wrong: assumes both decay to pointer size", "16", 0},
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

func TestCMacroExpansionPitfallWant_GroundTruth(t *testing.T) {
	src := `#include <stdio.h>

#define SQUARE(x) x * x

int main(void) {
	printf("%d\n", SQUARE(2 + 3));
	return 0;
}
`
	got := ccRun(t, src)
	if cMacroExpansionPitfallWant != "11" {
		t.Errorf("cMacroExpansionPitfallWant = %q, want %q", cMacroExpansionPitfallWant, "11")
	}
	if got != "11" {
		t.Errorf("cc output = %q, want %q", got, "11")
	}
}

func TestCMacroExpansionPitfallTest_Eval(t *testing.T) {
	tc := cMacroExpansionPitfallTest()

	correctProgram := "```c\n" + `#include <stdio.h>

#define SQUARE(x) x * x

int main(void) {
	printf("%d\n", SQUARE(2 + 3));
	return 0;
}
` + "```"

	// A model that "fixes" the macro with parentheses no longer reproduces
	// the pitfall's actual (unparenthesized) expansion, so it must score 0
	// against this test's want ("11"), even though (2+3)*(2+3)=25 is what a
	// naive reading of "SQUARE" would suggest.
	parenthesizedProgram := "```c\n" + `#include <stdio.h>

#define SQUARE(x) ((x) * (x))

int main(void) {
	printf("%d\n", SQUARE(2 + 3));
	return 0;
}
` + "```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct: reproduces the macro exactly as given", correctProgram, 1},
		{"wrong: silently parenthesizes the macro, changing its output", parenthesizedProgram, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Skipped {
				t.Skip("cc not available, skipping exec-evaluator assertion")
			}
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestCUndefinedBehaviorSpotTest_Eval(t *testing.T) {
	tc := cUndefinedBehaviorSpotTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", `{"line": 6, "kind": "signed-integer-overflow"}`, 1},
		{"correct fenced json", "```json\n{\"line\": 6, \"kind\": \"signed-integer-overflow\"}\n```", 1},
		{"correct with prose wrapper", `The answer is: {"line": 6, "kind": "signed-integer-overflow"}`, 1},
		{"wrong line", `{"line": 5, "kind": "signed-integer-overflow"}`, 0.5},
		{"wrong kind", `{"line": 6, "kind": "uninitialized-read"}`, 0.5},
		{"both wrong", `{"line": 7, "kind": "buffer-overflow"}`, 0},
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

func TestCStructBitfieldPacking_GroundTruth(t *testing.T) {
	src := `#include <stdio.h>
` + cStructBitfieldPackingCode + `
int main(void) {
	printf("%zu", sizeof(struct Flags));
	return 0;
}
`
	got := ccRun(t, src)
	if want := "4"; got != want {
		t.Errorf("cc output = %q, want %q", got, want)
	}
}

func TestCStructBitfieldPackingTest_Eval(t *testing.T) {
	tc := cStructBitfieldPackingTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "4", 1},
		{"prose wrapped", "sizeof(struct Flags) is 4 bytes", 1},
		{"bit count then byte answer", "3+5+24 = 32 bits = 4 bytes", 1},
		{"wrong: one byte per bitfield member", "3", 0},
		{"wrong: assumes each bitfield gets its own storage unit", "12", 0},
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

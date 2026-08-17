package eval

import (
	"context"
	"os/exec"
	"testing"
)

func requireToolchain(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("toolchain %s not found on PATH, skipping", name)
	}
}

// goHarness is a complete, standalone harness.go: its own package clause
// and imports, calling Double (defined separately in solution.go, same
// package) - no {{CODE}} placeholder, per GoRun's two-file contract.
const goHarness = `package main

import "fmt"

func main() {
	fmt.Println(Double(21))
}
`

func TestGoRun_Pass(t *testing.T) {
	requireToolchain(t, "go")
	e := GoRun(goHarness, "42")
	response := "```go\nfunc Double(n int) int { return n * 2 }\n```"
	got := e.Evaluate(context.Background(), response)
	if got.Value != 1 {
		t.Errorf("Evaluate() = %v, want 1 (detail: %s)", got.Value, got.Detail)
	}
}

func TestGoRun_WrongOutput(t *testing.T) {
	requireToolchain(t, "go")
	e := GoRun(goHarness, "42")
	response := "```go\nfunc Double(n int) int { return n + 2 }\n```"
	got := e.Evaluate(context.Background(), response)
	if got.Value != 0 {
		t.Errorf("Evaluate() = %v, want 0 (detail: %s)", got.Value, got.Detail)
	}
}

func TestGoRun_CompileError(t *testing.T) {
	requireToolchain(t, "go")
	e := GoRun(goHarness, "42")
	response := "```go\nfunc Double(n int) int { this is not valid go\n```"
	got := e.Evaluate(context.Background(), response)
	if got.Value != 0 {
		t.Errorf("Evaluate() = %v, want 0 for compile error", got.Value)
	}
	if got.Skipped {
		t.Error("Evaluate() should not be Skipped for a compile error")
	}
}

func TestStripLeadingPackageClause(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{
			name: "leading package clause is stripped",
			code: "package main\n\nfunc F() {}",
			want: "\nfunc F() {}",
		},
		{
			name: "blank lines before package clause are tolerated",
			code: "\n\npackage foo\nfunc F() {}",
			want: "\n\nfunc F() {}",
		},
		{
			name: "no package clause leaves code untouched",
			code: "import \"fmt\"\n\nfunc F() {}",
			want: "import \"fmt\"\n\nfunc F() {}",
		},
		{
			name: "package clause not on the first line is left alone",
			code: "func F() {}\n\npackage main",
			want: "func F() {}\n\npackage main",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripLeadingPackageClause(tt.code)
			if got != tt.want {
				t.Errorf("stripLeadingPackageClause(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestCheckToolchain_MissingBinaryIsSkipped(t *testing.T) {
	s := checkToolchain("definitely-not-a-real-toolchain-xyz")
	if s == nil || !s.Skipped {
		t.Fatal("checkToolchain() for missing binary should return Skipped score")
	}
}

const pyPassthroughHarness = PassthroughHarness

func TestPyRun_Pass(t *testing.T) {
	requireToolchain(t, "python3")
	e := PyRun(pyPassthroughHarness, "OOMKilled=2\nImagePullBackOff=1")
	response := "```python\nprint('OOMKilled=2')\nprint('ImagePullBackOff=1')\n```"
	got := e.Evaluate(context.Background(), response)
	if got.Value != 1 {
		t.Errorf("Evaluate() = %v, want 1 (detail: %s)", got.Value, got.Detail)
	}
}

func TestPyRun_WrongOutput(t *testing.T) {
	requireToolchain(t, "python3")
	e := PyRun(pyPassthroughHarness, "OOMKilled=2")
	response := "```python\nprint('OOMKilled=3')\n```"
	got := e.Evaluate(context.Background(), response)
	if got.Value != 0 {
		t.Errorf("Evaluate() = %v, want 0 (detail: %s)", got.Value, got.Detail)
	}
}

func TestPyRun_RuntimeError(t *testing.T) {
	requireToolchain(t, "python3")
	e := PyRun(pyPassthroughHarness, "anything")
	response := "```python\nraise ValueError('boom')\n```"
	got := e.Evaluate(context.Background(), response)
	if got.Value != 0 || got.Skipped {
		t.Errorf("Evaluate() = %+v, want zero non-skipped score for runtime error", got)
	}
}

const cHarness = `#include <stdio.h>

{{CODE}}

int main(void) {
    printf("%d\n", triple(7));
    return 0;
}
`

func TestCRun_Pass(t *testing.T) {
	requireToolchain(t, "cc")
	e := CRun(cHarness, "21")
	response := "```c\nint triple(int n) { return n * 3; }\n```"
	got := e.Evaluate(context.Background(), response)
	if got.Value != 1 {
		t.Errorf("Evaluate() = %v, want 1 (detail: %s)", got.Value, got.Detail)
	}
}

func TestCRun_CompileError(t *testing.T) {
	requireToolchain(t, "cc")
	e := CRun(cHarness, "21")
	response := "```c\nthis is not valid c code {{{\n```"
	got := e.Evaluate(context.Background(), response)
	if got.Value != 0 {
		t.Errorf("Evaluate() = %v, want 0 for compile error", got.Value)
	}
}

func TestExecEvaluators_ToolchainMissingSkipsNotFails(t *testing.T) {
	// This test does not require any toolchain; it verifies the Skipped
	// contract directly against a bogus PATH so it runs in every environment.
	t.Setenv("PATH", t.TempDir())

	goScore := GoRun(goHarness, "42").Evaluate(context.Background(), "```go\nfunc Double(n int) int { return n*2 }\n```")
	if !goScore.Skipped {
		t.Errorf("GoRun with empty PATH: Skipped = false, want true (%+v)", goScore)
	}

	pyScore := PyRun(pyPassthroughHarness, "x").Evaluate(context.Background(), "```python\nprint('x')\n```")
	if !pyScore.Skipped {
		t.Errorf("PyRun with empty PATH: Skipped = false, want true (%+v)", pyScore)
	}

	cScore := CRun(cHarness, "21").Evaluate(context.Background(), "```c\nint triple(int n){return n*3;}\n```")
	if !cScore.Skipped {
		t.Errorf("CRun with empty PATH: Skipped = false, want true (%+v)", cScore)
	}
}

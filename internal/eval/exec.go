package eval

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// leadingPackageClausePattern matches a Go source line that is nothing but
// a package clause (optionally indented), used to defensively strip a
// model's own "package xxx" line before splicing its code into
// solution.go. Only the first non-blank line is ever checked/stripped; a
// package clause appearing later (inside a string, comment, or as a
// genuine syntax error) is left alone.
var leadingPackageClausePattern = regexp.MustCompile(`^\s*package\s+\w+\s*$`)

// stripLeadingPackageClause removes a leading "package xxx" line from code,
// if the first non-blank line is one. This lets a model's response be
// either a bare code fragment or a full file (package clause + imports +
// declarations) and still compile as part of solution.go's package main.
func stripLeadingPackageClause(code string) string {
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if leadingPackageClausePattern.MatchString(line) {
			return strings.Join(append(lines[:i:i], lines[i+1:]...), "\n")
		}
		break
	}
	return code
}

// execTimeout bounds every compile/run step of the exec evaluators.
const execTimeout = 30 * time.Second

// CodePlaceholder marks where the extracted response code is substituted
// into a harness template.
const CodePlaceholder = "{{CODE}}"

// PassthroughHarness is a harness that runs the extracted code verbatim,
// with no surrounding driver. Use it when the response is expected to be a
// complete, self-contained program (e.g. a full Python script) rather than
// a fragment plugged into a test driver.
const PassthroughHarness = CodePlaceholder

// buildSource substitutes code into harness at CodePlaceholder.
func buildSource(harness, code string) string {
	return strings.ReplaceAll(harness, CodePlaceholder, code)
}

// goEnvCache memoizes `go env GOCACHE`/`go env GOMODCACHE` so every
// exec-evaluator call does not pay for an extra subprocess: they are read
// once per process and reused.
var (
	goEnvOnce     sync.Once
	goCacheDir    string
	goModCacheDir string
)

// goEnvValue shells out to `go env key` and returns its trimmed output, or
// "" if the go tool is unavailable or the call fails.
func goEnvValue(key string) string {
	// #nosec G204 -- key is always one of the two fixed, hardcoded literals
	// ("GOCACHE"/"GOMODCACHE") passed by minimalExecEnv below, never
	// response/user-controlled input.
	out, err := exec.Command("go", "env", key).Output() //nolint:gosec // key is one of two hardcoded literals, see comment above
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// minimalExecEnv builds a minimal environment allowlist for compiling and
// running model-generated code, rather than inheriting the operator's full
// environment: PATH (needed to find go/python3/cc and any tool they shell
// out to), HOME and TMPDIR pinned to homeDir (an isolated, single-call temp
// directory, never the operator's real home), and GOCACHE/GOMODCACHE
// inherited from the operator's actual Go environment so `go run` reuses
// the existing build/module cache instead of rebuilding it from scratch on
// every call.
func minimalExecEnv(homeDir string) []string {
	goEnvOnce.Do(func() {
		goCacheDir = goEnvValue("GOCACHE")
		goModCacheDir = goEnvValue("GOMODCACHE")
	})

	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + homeDir,
		"TMPDIR=" + homeDir,
	}
	if goCacheDir != "" {
		env = append(env, "GOCACHE="+goCacheDir)
	}
	if goModCacheDir != "" {
		env = append(env, "GOMODCACHE="+goModCacheDir)
	}
	return env
}

// mkWorkDirs creates an isolated per-call temp root plus a "work"
// subdirectory inside it. Callers write source files into work and run
// commands with cmd.Dir = work, while pointing HOME/TMPDIR (via
// minimalExecEnv) at root, never at work itself: the Go toolchain
// specifically refuses to detect a go.mod when the module directory is
// exactly equal to the resolved system temp root ("ignoring go.mod in
// system temp root"), which fires precisely when TMPDIR is set to the same
// directory `go run` is invoked from. Keeping root and work as distinct
// (parent, child) directories avoids that while still confining
// HOME/TMPDIR to a directory nothing else uses.
func mkWorkDirs(prefix string) (root, work string, err error) {
	root, err = os.MkdirTemp("", prefix)
	if err != nil {
		return "", "", err
	}
	work = filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		return "", "", err
	}
	return root, work, nil
}

// checkToolchain reports a Skipped score if name is not on PATH, nil
// otherwise.
func checkToolchain(name string) *Score {
	if _, err := exec.LookPath(name); err != nil {
		s := Score{Skipped: true, Detail: fmt.Sprintf("toolchain missing: %s", name)}
		return &s
	}
	return nil
}

// compareStdout scores full credit when stdout, trimmed, equals want,
// trimmed. cmdErr, when non-nil, always scores zero and surfaces stderr.
func compareStdout(stdout, want string, cmdErr error, stderr string) Score {
	if cmdErr != nil {
		return Score{Value: 0, Detail: fmt.Sprintf("execution failed: %v; stderr: %s", cmdErr, strings.TrimSpace(stderr))}
	}
	got := strings.TrimSpace(stdout)
	wantTrimmed := strings.TrimSpace(want)
	if got == wantTrimmed {
		return Score{Value: 1, Detail: "stdout matches expected"}
	}
	return Score{Value: 0, Detail: fmt.Sprintf("stdout = %q, want %q", got, wantTrimmed)}
}

// goRunEval compiles/runs a two-file Go module: solution.go (the model's
// code, wrapped in package main) plus harness.go (a complete, independent
// driver file supplied by the test author).
type goRunEval struct {
	harness string
	want    string
}

// GoRun returns an Evaluator that writes the response's first fenced Go
// code block to its own solution.go (defensively stripping a leading
// "package xxx" line and prefixing "package main"), writes harness
// verbatim to its own harness.go (a complete file: its own package clause,
// imports, and func main), `go run`s both together in an isolated temp
// module, and compares trimmed stdout to want. Splitting the model's code
// and the harness into separate files means each may import what it needs
// independently, instead of splicing code into a single shared import
// block. If the `go` toolchain is unavailable, the Score is Skipped rather
// than zero.
func GoRun(harness, want string) Evaluator {
	return goRunEval{harness: harness, want: want}
}

func (g goRunEval) Evaluate(ctx context.Context, response string) Score {
	if s := checkToolchain("go"); s != nil {
		return *s
	}

	code := ExtractCodeBlock(response, "go")
	solution := "package main\n\n" + stripLeadingPackageClause(code)

	root, work, err := mkWorkDirs("llmtb-go-*")
	if err != nil {
		return Score{Value: 0, Detail: fmt.Sprintf("mkdtemp: %v", err)}
	}
	defer func() { _ = os.RemoveAll(root) }() // best-effort cleanup; failure is not actionable

	if writeErr := os.WriteFile(filepath.Join(work, "solution.go"), []byte(solution), 0o600); writeErr != nil {
		return Score{Value: 0, Detail: fmt.Sprintf("write solution.go: %v", writeErr)}
	}
	if writeErr := os.WriteFile(filepath.Join(work, "harness.go"), []byte(g.harness), 0o600); writeErr != nil {
		return Score{Value: 0, Detail: fmt.Sprintf("write harness.go: %v", writeErr)}
	}
	if modErr := os.WriteFile(filepath.Join(work, "go.mod"), []byte("module llmtbharness\n\ngo 1.23\n"), 0o600); modErr != nil {
		return Score{Value: 0, Detail: fmt.Sprintf("write go.mod: %v", modErr)}
	}

	runCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "go", "run", ".")
	cmd.Dir = work
	cmd.Env = minimalExecEnv(root)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	return compareStdout(stdout.String(), g.want, err, stderr.String())
}

// pyRunEval runs a Python program built from a harness template plus the
// extracted Python code block from the response.
type pyRunEval struct {
	harness string
	want    string
}

// PyRun returns an Evaluator that substitutes the response's first fenced
// Python code block into harness at CodePlaceholder, runs it with python3,
// and compares trimmed stdout to want. If python3 is unavailable, the Score
// is Skipped rather than zero.
func PyRun(harness, want string) Evaluator {
	return pyRunEval{harness: harness, want: want}
}

func (p pyRunEval) Evaluate(ctx context.Context, response string) Score {
	if s := checkToolchain("python3"); s != nil {
		return *s
	}

	code := ExtractCodeBlock(response, "python")
	source := buildSource(p.harness, code)

	root, work, err := mkWorkDirs("llmtb-py-*")
	if err != nil {
		return Score{Value: 0, Detail: fmt.Sprintf("mkdtemp: %v", err)}
	}
	defer func() { _ = os.RemoveAll(root) }() // best-effort cleanup; failure is not actionable

	scriptPath := filepath.Join(work, "script.py")
	if writeErr := os.WriteFile(scriptPath, []byte(source), 0o600); writeErr != nil {
		return Score{Value: 0, Detail: fmt.Sprintf("write script.py: %v", writeErr)}
	}

	runCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	// #nosec G204 -- this is the eval package's whole purpose: compile/run
	// model-generated code under an isolated temp dir and a minimal env
	// allowlist (minimalExecEnv). See README's "Requirements" section for
	// the corresponding operator-facing warning (S13).
	cmd := exec.CommandContext(runCtx, "python3", scriptPath) //nolint:gosec // see comment above
	cmd.Dir = work
	cmd.Env = minimalExecEnv(root)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	return compareStdout(stdout.String(), p.want, err, stderr.String())
}

// cRunEval compiles/runs a C program built from a harness template plus the
// extracted C code block from the response.
type cRunEval struct {
	harness string
	want    string
}

// CRun returns an Evaluator that substitutes the response's first fenced C
// code block into harness at CodePlaceholder, compiles it with cc, runs the
// binary, and compares trimmed stdout to want. If cc is unavailable, the
// Score is Skipped rather than zero.
func CRun(harness, want string) Evaluator {
	return cRunEval{harness: harness, want: want}
}

func (c cRunEval) Evaluate(ctx context.Context, response string) Score {
	if s := checkToolchain("cc"); s != nil {
		return *s
	}

	code := ExtractCodeBlock(response, "c")
	source := buildSource(c.harness, code)

	root, work, err := mkWorkDirs("llmtb-c-*")
	if err != nil {
		return Score{Value: 0, Detail: fmt.Sprintf("mkdtemp: %v", err)}
	}
	defer func() { _ = os.RemoveAll(root) }() // best-effort cleanup; failure is not actionable

	srcPath := filepath.Join(work, "main.c")
	if writeErr := os.WriteFile(srcPath, []byte(source), 0o600); writeErr != nil {
		return Score{Value: 0, Detail: fmt.Sprintf("write main.c: %v", writeErr)}
	}
	binPath := filepath.Join(work, "prog")

	buildCtx, buildCancel := context.WithTimeout(ctx, execTimeout)
	defer buildCancel()
	// #nosec G204 -- compiling model-generated C source under an isolated
	// temp dir and a minimal env allowlist is this evaluator's job; see the
	// PyRun comment above and README's "Requirements" warning (S13).
	buildCmd := exec.CommandContext(buildCtx, "cc", "-o", binPath, srcPath) //nolint:gosec // see comment above
	buildCmd.Dir = work
	buildCmd.Env = minimalExecEnv(root)
	var buildStderr bytes.Buffer
	buildCmd.Stderr = &buildStderr
	if buildErr := buildCmd.Run(); buildErr != nil {
		return Score{Value: 0, Detail: fmt.Sprintf("cc failed: %v; stderr: %s", buildErr, strings.TrimSpace(buildStderr.String()))}
	}

	runCtx, runCancel := context.WithTimeout(ctx, execTimeout)
	defer runCancel()
	// #nosec G204 -- binPath is the binary this same call just compiled,
	// into a fresh, isolated temp dir this evaluator created; not an
	// externally supplied executable path.
	runCmd := exec.CommandContext(runCtx, binPath) //nolint:gosec // see comment above
	runCmd.Dir = work
	runCmd.Env = minimalExecEnv(root)
	var stdout, stderr bytes.Buffer
	runCmd.Stdout = &stdout
	runCmd.Stderr = &stderr
	err = runCmd.Run()

	return compareStdout(stdout.String(), c.want, err, stderr.String())
}

package tests

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// TestDelGitBisectStepsWant_GroundTruth recomputes delGitBisectStepsWant
// from first principles (ceil(log2(N))) rather than trusting the hardcoded
// literal in delivery_git.go.
func TestDelGitBisectStepsWant_GroundTruth(t *testing.T) {
	got := int(math.Ceil(math.Log2(float64(delGitBisectCommitCount))))
	if got != delGitBisectStepsWant {
		t.Fatalf("ceil(log2(%d)) = %d, want %d", delGitBisectCommitCount, got, delGitBisectStepsWant)
	}
}

func TestDelGitBisectStepsTest_Eval(t *testing.T) {
	tc := delGitBisectStepsTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "bare number", response: "8", want: 1},
		{name: "sentence form", response: "The answer is 8.", want: 1},
		{name: "different sentence form", response: "There are 8 bisect steps needed.", want: 1},
		{name: "wrong: repeats commit count", response: "137", want: 0},
		{name: "wrong: off by one", response: "7", want: 0},
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

func TestDelGitRebaseVsMergeTest_Eval(t *testing.T) {
	tc := delGitRebaseVsMergeTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: `{"scenario_a":"rebase","scenario_b":"merge"}`, want: 1},
		{name: "all correct fenced", response: "```json\n{\"scenario_a\":\"rebase\",\"scenario_b\":\"merge\"}\n```", want: 1},
		{name: "scenario_a wrong", response: `{"scenario_a":"merge","scenario_b":"merge"}`, want: 0.5},
		{name: "scenario_b wrong", response: `{"scenario_a":"rebase","scenario_b":"rebase"}`, want: 0.5},
		{name: "both swapped", response: `{"scenario_a":"merge","scenario_b":"rebase"}`, want: 0},
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

func TestDelGitConventionalCommitClassifyTest_Eval(t *testing.T) {
	tc := delGitConventionalCommitClassifyTest()

	allCorrect := `{"commit1":"feat","commit2":"fix","commit3":"chore","commit4":"docs","commit5":"refactor","commit6":"test"}`

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "all correct", response: allCorrect, want: 1},
		{name: "all correct fenced with prose", response: "Here is my classification:\n```json\n" + allCorrect + "\n```", want: 1},
		{
			name:     "one wrong: commit1 misclassified as chore",
			response: `{"commit1":"chore","commit2":"fix","commit3":"chore","commit4":"docs","commit5":"refactor","commit6":"test"}`,
			want:     5.0 / 6.0,
		},
		{
			name:     "two wrong: commit3 and commit5 swapped types",
			response: `{"commit1":"feat","commit2":"fix","commit3":"refactor","commit4":"docs","commit5":"chore","commit6":"test"}`,
			want:     4.0 / 6.0,
		},
		{
			name:     "all wrong",
			response: `{"commit1":"docs","commit2":"chore","commit3":"feat","commit4":"test","commit5":"fix","commit6":"refactor"}`,
			want:     0,
		},
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

func TestDelGitWorktreeUseCaseTest_Eval(t *testing.T) {
	tc := delGitWorktreeUseCaseTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: exact command",
			response: "Use `git worktree add ../v1.4.0-check v1.4.0` to create a second, linked working directory.",
			want:     1,
		},
		{
			name:     "correct: relaxed prose",
			response: "Run git worktree add ../v1.4.0-check v1.4.0 to build the old tag in another directory.",
			want:     1,
		},
		{
			name:     "correct: alternate phrasing",
			response: "git worktree is exactly this: git worktree add ../v1.4.0-check v1.4.0 links a second checkout to the same repo.",
			want:     1,
		},
		{
			name:     "wrong: suggests re-cloning, mentions the tag only",
			response: "Just run git clone again into a new folder and checkout v1.4.0 there.",
			want:     1.0 / 3.0,
		},
		{
			name:     "wrong: suggests stash, no worktree or tag path mentioned",
			response: "Use git stash to save your changes, then checkout the old tag in a scratch directory, then stash pop when done.",
			want:     0,
		},
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

func TestDelGitForceWithLeaseVsForceTest_Eval(t *testing.T) {
	tc := delGitForceWithLeaseVsForceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: recommends force-with-lease, negates bare force",
			response: "Use git push --force-with-lease on the shared branch; never use a bare --force because it would silently overwrite the remote branch if it moved.",
			want:     1,
		},
		{
			name:     "correct: only ever mentions the safe form",
			response: "On a shared branch always push with --force-with-lease; it aborts if the remote moved since your last fetch, unlike a plain overwrite.",
			want:     1,
		},
		{
			name:     "correct: bare force mentioned but clearly negated",
			response: "Don't ever run a bare --force on release/2.4; instead use --force-with-lease, which fails safely if the remote changed.",
			want:     1,
		},
		{
			name:     "wrong: recommends bare force outright",
			response: "Just use git push --force on the shared branch to make sure your rewritten history replaces theirs.",
			want:     0,
		},
		{
			name:     "wrong: recommends force-with-lease but also an unnegated bare-force fallback",
			response: "Push with --force-with-lease normally, but if the lease check fails just fall back to --force to get it through.",
			want:     0.5,
		},
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

func TestDelGitHookChoiceTest_Eval(t *testing.T) {
	tc := delGitHookChoiceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "exact", response: "commit-msg", want: 1},
		{name: "different case", response: "Commit-Msg", want: 1},
		{name: "surrounding whitespace", response: " commit-msg \n", want: 1},
		{name: "wrong: pre-commit", response: "pre-commit", want: 0},
		{name: "wrong: pre-push", response: "pre-push", want: 0},
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

func TestDelGitDetachedHeadRecoveryTest_Eval(t *testing.T) {
	tc := delGitDetachedHeadRecoveryTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "correct order", response: `["branch","checkout"]`, want: 1},
		{name: "correct order fenced", response: "```json\n[\"branch\",\"checkout\"]\n```", want: 1},
		{name: "correct order different case", response: `["Branch","Checkout"]`, want: 1},
		{name: "reversed order", response: `["checkout","branch"]`, want: 0},
		{name: "wrong second subcommand", response: `["branch","switch"]`, want: 0},
		{name: "single-command checkout -b is equally correct (D6)", response: `["checkout"]`, want: 1},
		{name: "single-command switch -c is equally correct (D6)", response: `["switch"]`, want: 1},
		{name: "bare branch alone leaves HEAD detached, not accepted (D6)", response: `["branch"]`, want: 0},
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

func TestDelGitCherryPickVsRevertTest_Eval(t *testing.T) {
	tc := delGitCherryPickVsRevertTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "exact", response: "revert", want: 1},
		{name: "different case", response: "Revert", want: 1},
		{name: "surrounding whitespace", response: " revert \n", want: 1},
		{name: "wrong: cherry-pick", response: "cherry-pick", want: 0},
		{name: "wrong: reset", response: "reset", want: 0},
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

func TestDelGitGitignoreNegationTraceTest_Eval(t *testing.T) {
	tc := delGitGitignoreNegationTraceTest()

	correctSet := `["build/keep.txt","build/output.bin","debug.log","docs/notes.tmp"]`
	correctSetReordered := `["docs/notes.tmp","debug.log","build/output.bin","build/keep.txt"]`

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "exact correct set", response: correctSet, want: 1},
		{name: "correct set, different order", response: correctSetReordered, want: 1},
		{name: "correct set, fenced", response: "```json\n" + correctSet + "\n```", want: 1},
		{
			name:     "wrong: falls for the negation trap, includes docs/important.tmp",
			response: `["build/keep.txt","build/output.bin","debug.log","docs/notes.tmp","docs/important.tmp"]`,
			want:     0.8,
		},
		{
			name:     "wrong: assumes negation always works, drops build/keep.txt",
			response: `["build/output.bin","debug.log","docs/notes.tmp"]`,
			want:     0.75,
		},
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

func TestDelGitStashPopConflictTest_Eval(t *testing.T) {
	tc := delGitStashPopConflictTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{name: "exact", response: "kept", want: 1},
		{name: "different case", response: "Kept", want: 1},
		{name: "surrounding whitespace", response: " kept \n", want: 1},
		{name: "wrong: dropped", response: "dropped", want: 0},
		{name: "wrong: removed", response: "removed", want: 0},
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

// delGitTestEnv builds a minimal, isolated environment for exec.Command
// calls against a throwaway git repository: PATH (to find git), HOME
// pointed at the isolated dir (never the operator's real home), and
// GIT_CONFIG_NOSYSTEM/GIT_CONFIG_GLOBAL disabled so neither the system nor
// the operator's real ~/.gitconfig (which may set core.hooksPath to a
// global pre-commit hook) is consulted. GIT_AUTHOR_*/GIT_COMMITTER_* supply
// a commit identity directly, so no "git config user.email" step is needed
// either. This keeps the test hermetic: no network, no dependency on the
// operator's actual git configuration.
func delGitTestEnv(dir string) []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + dir,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=llmtb-test",
		"GIT_AUTHOR_EMAIL=llmtb-test@example.com",
		"GIT_COMMITTER_NAME=llmtb-test",
		"GIT_COMMITTER_EMAIL=llmtb-test@example.com",
	}
}

// delRunGit runs `git args...` inside dir with env, returning combined
// stdout+stderr and any error.
func delRunGit(t *testing.T, dir string, env []string, args ...string) (string, error) {
	t.Helper()
	// #nosec G204 -- args are always fixed literals or the fixed
	// delGitignoreTreeFiles list defined in delivery_git.go, never
	// response/external/user-controlled input.
	cmd := exec.Command("git", args...) //nolint:gosec // see comment above
	cmd.Dir = dir
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// TestDelGitGitignoreNegationTrace_GroundTruth verifies delGitignoreIgnoredWant
// against a real, throwaway git repository (git.md's "verify empirically
// where cheap" guidance) rather than trusting delivery_git.go's derivation
// comment alone. Skips if git is not installed.
func TestDelGitGitignoreNegationTrace_GroundTruth(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	dir := t.TempDir()
	env := delGitTestEnv(dir)

	if err := os.MkdirAll(filepath.Join(dir, "build"), 0o700); err != nil {
		t.Fatalf("mkdir build: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o700); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(delGitignoreFile+"\n"), 0o600); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	for _, f := range delGitignoreTreeFiles {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x\n"), 0o600); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}

	if out, err := delRunGit(t, dir, env, "init", "-q"); err != nil {
		t.Fatalf("git init: %v (%s)", err, out)
	}

	args := append([]string{"check-ignore"}, delGitignoreTreeFiles...)
	out, err := delRunGit(t, dir, env, args...)
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			t.Fatalf("git check-ignore: %v (%s)", err, out)
		}
		// exit code 1 just means "none of the given paths are ignored",
		// which is not the case here but is a valid, non-fatal outcome of
		// the command in general.
	}

	var got []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			got = append(got, line)
		}
	}
	sort.Strings(got)

	want := append([]string(nil), delGitignoreIgnoredWant...)
	sort.Strings(want)

	if !slices.Equal(got, want) {
		t.Fatalf("git check-ignore ignored set = %v, want %v", got, want)
	}
}

// TestDelGitStashPopConflict_GroundTruth verifies, against a real,
// throwaway git repository, that a conflicting `git stash pop` keeps the
// stash entry rather than dropping it. Skips if git is not installed.
func TestDelGitStashPopConflict_GroundTruth(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	dir := t.TempDir()
	env := delGitTestEnv(dir)
	fpath := filepath.Join(dir, "f.txt")

	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(fpath, []byte(content), 0o600); err != nil {
			t.Fatalf("write f.txt: %v", err)
		}
	}
	mustGit := func(args ...string) string {
		t.Helper()
		out, err := delRunGit(t, dir, env, args...)
		if err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
		return out
	}

	mustGit("init", "-q")
	write("line1\nline2\nline3\n")
	mustGit("add", "f.txt")
	mustGit("commit", "-q", "-m", "init")

	write("line1\nline2-stashed\nline3\n")
	mustGit("stash", "push", "-q", "-m", "work")

	write("line1\nline2-committed\nline3\n")
	mustGit("add", "f.txt")
	mustGit("commit", "-q", "-m", "conflicting")

	if _, err := delRunGit(t, dir, env, "stash", "pop"); err == nil {
		t.Fatal("git stash pop: want a conflict (non-nil error) given the two changes touch the same line, got nil")
	}

	out := mustGit("stash", "list")
	if strings.TrimSpace(out) == "" {
		t.Fatal("git stash list: want the stash entry kept after a conflicting pop, got an empty list")
	}
}

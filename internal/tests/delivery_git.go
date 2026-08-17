package tests

import (
	"context"
	"regexp"
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerDeliveryGitTests(r *testkit.Registry) {
	r.Register(delGitBisectStepsTest())
	r.Register(delGitRebaseVsMergeTest())
	r.Register(delGitConventionalCommitClassifyTest())
	r.Register(delGitWorktreeUseCaseTest())
	r.Register(delGitForceWithLeaseVsForceTest())
	r.Register(delGitHookChoiceTest())
	r.Register(delGitDetachedHeadRecoveryTest())
	r.Register(delGitCherryPickVsRevertTest())
	r.Register(delGitGitignoreNegationTraceTest())
	r.Register(delGitStashPopConflictTest())
}

// --- negation-aware helpers, shared across delivery_git.go and
// delivery_release.go (same package). Duplicated in spirit from
// operations/kubernetes's noLiveKubectlMutation/negationCuePattern rather
// than imported from kubernetes.go, so this category's files stay
// self-contained: round-2 authors work in isolated worktrees on
// databases/security/delivery/ai in parallel, and depending on another
// category's unexported symbols would create a merge-time coupling this
// worktree cannot see or coordinate on. ---

// delNegationCuePattern matches a word that turns a mention of a forbidden
// phrase into a warning against doing it, rather than an instruction to do
// it (e.g. "never use bare --force", "must not silently modify").
var delNegationCuePattern = regexp.MustCompile(`(?i)\b(don'?t|do not|never|avoid|instead of|not|cannot|can'?t|rather than|without|no need|must not|should not|shouldn'?t)\b`)

// delNegationWindow is how many characters before a forbidden-phrase match
// are searched for a negation cue.
const delNegationWindow = 60

// delNegationWindowStart returns the earliest byte offset in response to
// search for a negation cue before a forbidden-phrase match starting at
// start. The window is the current line, extended back to the start of the
// immediately preceding line when that line is non-empty, so a hard-wrapped
// sentence whose negation cue landed on the line above still counts. Either
// way the window never reaches more than delNegationWindow characters
// before start.
func delNegationWindowStart(response string, start int) int {
	curLineStart := strings.LastIndexByte(response[:start], '\n') + 1

	windowFloor := curLineStart
	if curLineStart > 0 {
		prevLineEnd := curLineStart - 1
		prevLineStart := strings.LastIndexByte(response[:prevLineEnd], '\n') + 1
		if strings.TrimSpace(response[prevLineStart:prevLineEnd]) != "" {
			windowFloor = prevLineStart
		}
	}

	return max(start-delNegationWindow, windowFloor)
}

// delNoUnnegatedMention returns an Evaluator scoring full credit unless
// forbidden matches response at some position with no negation cue in the
// preceding window - i.e. the response recommends the forbidden thing
// outright rather than warning against it. This is deliberately not
// eval.NotContains, which would also zero out the best possible answer (one
// that correctly explains why NOT to do the forbidden thing).
func delNoUnnegatedMention(forbidden *regexp.Regexp, safeDetail string) eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		matches := forbidden.FindAllStringIndex(response, -1)
		if len(matches) == 0 {
			return eval.Score{Value: 1, Detail: "no forbidden mention: " + safeDetail}
		}
		for _, loc := range matches {
			start := loc[0]
			windowStart := delNegationWindowStart(response, start)
			if !delNegationCuePattern.MatchString(response[windowStart:start]) {
				return eval.Score{Value: 0, Detail: "unnegated forbidden mention"}
			}
		}
		return eval.Score{Value: 1, Detail: "every forbidden mention is negated"}
	})
}

// delForceFlagPattern matches every "--force" mention. delForceWithLeaseSuffixPattern
// checks whether one of those matches is immediately followed by
// "-with-lease" - the safe form, exempt from the bare-force check entirely
// rather than needing a negation cue.
var delForceFlagPattern = regexp.MustCompile(`(?i)--force\b`)
var delForceWithLeaseSuffixPattern = regexp.MustCompile(`(?i)^-with-lease`)

// delNoBareForcePush scores full credit unless the response recommends a
// bare "--force" push (not "--force-with-lease") with no negation cue
// nearby. A response that only ever mentions "--force-with-lease" never
// matches delForceFlagPattern's bare form (the suffix check exempts it), so
// it always scores full credit; a response that recommends bare --force
// outright scores zero.
func delNoBareForcePush() eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		matches := delForceFlagPattern.FindAllStringIndex(response, -1)
		if len(matches) == 0 {
			return eval.Score{Value: 1, Detail: "no --force mention"}
		}
		for _, loc := range matches {
			start, end := loc[0], loc[1]
			if delForceWithLeaseSuffixPattern.MatchString(response[end:]) {
				continue // "--force-with-lease": the safe form, not bare force
			}
			windowStart := delNegationWindowStart(response, start)
			if !delNegationCuePattern.MatchString(response[windowStart:start]) {
				return eval.Score{Value: 0, Detail: "unnegated bare --force mention"}
			}
		}
		return eval.Score{Value: 1, Detail: "every bare --force mention is negated or absent"}
	})
}

// delGitBisectCommitCount is the number of candidate commits between the
// last known-good commit and the current bad HEAD in delGitBisectStepsTest.
const delGitBisectCommitCount = 137

// delGitBisectStepsWant is the worst-case number of `git bisect` steps
// needed to find the single first-bad commit among
// delGitBisectCommitCount candidates by binary search.
//
// ground truth: `git bisect` performs a binary search over the candidate
// commit range, halving the remaining range on each step until exactly one
// commit remains. The worst-case step count for N candidates is
// ceil(log2(N)): 2^7 = 128 < 137 <= 256 = 2^8, so ceil(log2(137)) = 8.
// delivery_git_test.go recomputes this with math.Ceil(math.Log2(137))
// rather than trusting the hardcoded literal alone.
var delGitBisectStepsWant = 8

// delGitBisectStepsTest: derive the worst-case number of `git bisect`
// binary-search steps needed to find one bad commit among N candidates.
func delGitBisectStepsTest() testkit.Test {
	prompt := `A regression was introduced somewhere among the last 137
commits on main: the commit at the tip of main is bad, and the commit 137
commits back (the last tagged release) is known good. Using "git bisect"
(binary search over the candidate commits, testing one commit per step),
what is the maximum number of "git bisect" steps needed to find the exact
first commit that introduced the regression? Respond with only the integer
number of steps.`

	return testkit.Test{
		ID:          "git-bisect-steps",
		Category:    "delivery",
		Subcategory: "git",
		Description: "Derive the worst-case git bisect step count (ceil(log2(N))) for 137 candidate commits.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], delGitBisectStepsWant, 0),
	}
}

// delGitRebaseVsMergeTest: pick rebase or merge for a solo short-lived
// branch versus a shared long-lived branch.
//
// ground truth: rebasing a branch nobody else has pulled is safe and
// produces a clean, linear, single-author history before review - exactly
// what interactive rebase is for. Rewriting history on a branch other
// people have already pulled and built work on top of forces everyone else
// to reconcile diverged history; merging (which adds a new commit rather
// than rewriting existing ones) is the only one of the two that does not
// rewrite commits other people already depend on.
func delGitRebaseVsMergeTest() testkit.Test {
	prompt := `For each of these two situations, should you use "git rebase"
or "git merge" to bring in the latest changes?

scenario_a: "You are the sole author of a short-lived feature branch that
has never been pushed anywhere, and you want a clean, linear, single-author
history before opening a pull request."
scenario_b: "You are integrating a long-lived, shared release branch that
several other people have already pulled and built further commits on top
of; you must not rewrite any commit they already depend on."

Respond with only a JSON object:
{"scenario_a":"rebase"|"merge","scenario_b":"rebase"|"merge"}`

	evaluator := eval.Mean(
		eval.JSONField("scenario_a", "rebase"),
		eval.JSONField("scenario_b", "merge"),
	)

	return testkit.Test{
		ID:          "git-rebase-vs-merge",
		Category:    "delivery",
		Subcategory: "git",
		Description: "Choose rebase for a solo unshared branch and merge for a shared branch others already depend on.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// delGitConventionalCommits is the inline set of 6 commit messages for
// delGitConventionalCommitClassifyTest, one per standard conventional-commit
// type, grounded in this repository's own real history (agents.go's
// per-subcategory file split, ExtractLastNumber's dot-prefixed-decimal
// handling, exec.go's toolchain-missing skip behavior, and the errgroup
// dependency named in PLAN.md).
const delGitConventionalCommits = `commit1: "feat(runner): add bounded concurrency to the model x test fan-out via errgroup"
commit2: "fix(eval): correct ExtractLastNumber rejecting a standalone dot-prefixed decimal like \".9896\""
commit3: "chore(deps): bump golang.org/x/sync to v0.22.0"
commit4: "docs: document how to add a new evaluator in README.md"
commit5: "refactor(tests): split agents.go into per-subcategory files to stay under 600 lines"
commit6: "test(exec): add toolchain-missing skip cases for GoRun/PyRun/CRun"`

// delGitConventionalCommitClassifyTest: classify 6 commit messages, one per
// standard conventional-commit type, by their type prefix.
func delGitConventionalCommitClassifyTest() testkit.Test {
	prompt := `Here are 6 commit messages from a project using Conventional
Commits:

` + delGitConventionalCommits + `

Classify each commit by its conventional-commit type (one of: feat, fix,
chore, docs, refactor, test). Respond with only a JSON object:
{"commit1":"...","commit2":"...","commit3":"...","commit4":"...","commit5":"...","commit6":"..."}`

	evaluator := eval.Mean(
		eval.JSONField("commit1", "feat"),
		eval.JSONField("commit2", "fix"),
		eval.JSONField("commit3", "chore"),
		eval.JSONField("commit4", "docs"),
		eval.JSONField("commit5", "refactor"),
		eval.JSONField("commit6", "test"),
	)

	return testkit.Test{
		ID:          "git-conventional-commit-classify",
		Category:    "delivery",
		Subcategory: "git",
		Description: "Classify 6 commit messages into their conventional-commit type from the message prefix.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// delGitWorktreeUseCaseTest: name git worktree as the mechanism for a
// second, independently checked-out working directory from the same
// repository.
//
// ground truth: "git worktree add <path> <ref>" creates a second, linked
// working directory backed by the same .git repository, checked out to a
// different ref, without stashing, committing, or cloning again - exactly
// the described need (an in-progress feature branch with uncommitted
// changes stays untouched in the main working directory while an old tag is
// built and tested elsewhere).
func delGitWorktreeUseCaseTest() testkit.Test {
	prompt := `You are mid-way through an in-progress feature branch with
uncommitted changes in your main working directory. You need to build and
test the old release tag v1.4.0 at the same time, without stashing your
changes and without cloning the repository again. Name the git subcommand
that creates a second, linked working directory from the same repository
checked out to a different ref, and give the exact command to create one at
path ../v1.4.0-check checked out to tag v1.4.0.`

	return testkit.Test{
		ID:          "git-worktree-use-case",
		Category:    "delivery",
		Subcategory: "git",
		Description: "Name git worktree as the way to check out a second ref from the same repo without stashing or re-cloning.",
		Prompt:      prompt,
		Eval:        eval.ContainsAll("git worktree add", "v1.4.0-check", "v1.4.0"),
	}
}

// delGitForceWithLeaseVsForceTest: require --force-with-lease on a shared
// branch, and require any mention of bare --force to be negated.
//
// ground truth: "--force-with-lease" refuses the push if the remote branch
// has moved since your last fetch (someone else pushed), failing safely
// instead of silently overwriting their work; bare "--force" overwrites
// whatever is on the remote unconditionally, with no such check, so it must
// never be recommended unnegated for a push to a branch other people also
// push to.
func delGitForceWithLeaseVsForceTest() testkit.Test {
	prompt := `Two teammates also push to the shared branch "release/2.4".
After an interactive rebase, you rewrote your local commits and must
force-push your branch. Which git push flag should you use on this shared
branch, and why must a bare "--force" push never be used there instead?`

	evaluator := eval.All(
		eval.W(eval.ContainsAny("--force-with-lease", "force-with-lease"), 2),
		eval.W(delNoBareForcePush(), 2),
	)

	return testkit.Test{
		ID:          "git-force-with-lease-vs-force",
		Category:    "delivery",
		Subcategory: "git",
		Description: "Require --force-with-lease over bare --force for a force-push to a branch shared with teammates.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// delGitHookChoiceTest: pick commit-msg as the hook that enforces a commit
// message format at commit time, on the developer's own machine.
//
// ground truth: commit-msg receives the path to the drafted commit message
// file and runs (and can reject it) at the moment of commit, before the
// commit is created - exactly when a message-format policy must be
// enforced. pre-commit inspects staged file contents, not the message; and
// pre-push runs too late, after commits (and any bad messages) already
// exist locally.
func delGitHookChoiceTest() testkit.Test {
	prompt := `Your team wants to reject any commit whose message does not
follow the Conventional Commits format, at the moment the commit is
created, before it ever leaves the developer's machine. Name the single git
hook (its exact filename under .git/hooks/) that enforces this. Respond
with only the hook name.`

	return testkit.Test{
		ID:          "git-hook-choice",
		Category:    "delivery",
		Subcategory: "git",
		Description: "Pick commit-msg as the hook that enforces a commit message format at commit time.",
		Prompt:      prompt,
		Eval:        eval.Equals("commit-msg"),
	}
}

// delGitDetachedHeadRecoveryTest: recover 3 commits made in detached HEAD
// state onto a new branch, in the one order that works.
//
// ground truth: while still on the detached HEAD (pointing at the 3 new
// commits), "git branch <name>" creates a new branch ref at the current
// commit without moving HEAD, capturing the work; only then does "git
// checkout <name>" move HEAD onto that branch so it stops being detached.
// Reversing the order (checkout first) would move HEAD away from the
// detached commits before a branch ref exists to save them, losing the
// work to garbage collection.
func delGitDetachedHeadRecoveryTest() testkit.Test {
	prompt := `You ran "git checkout <old-commit-sha>" directly (not a
branch name), then made 3 new commits while HEAD was detached. You only now
realize you need to keep that work, and HEAD is still detached. Give the
ordered list of git subcommands (just the subcommand names, e.g. "branch")
you must run, in order, to save the 3 commits onto a new branch without
losing them. Respond with only a JSON array of subcommand names, e.g.
["a","b"].`

	return testkit.Test{
		ID:          "git-detached-head-recovery",
		Category:    "delivery",
		Subcategory: "git",
		Description: "Order git branch then git checkout to save 3 detached-HEAD commits onto a new branch.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals([]string{"branch", "checkout"}),
	}
}

// delGitCherryPickVsRevertTest: pick revert for undoing an already-shared
// commit without rewriting public history.
//
// ground truth: "git revert" creates a new commit that undoes the target
// commit's changes, leaving all existing history (including the original
// commit) intact - safe once a commit has been pulled by other people.
// "git cherry-pick" applies an existing commit's changes onto another
// branch; it does not undo anything, so it cannot be the answer to "undo
// its effect".
func delGitCherryPickVsRevertTest() testkit.Test {
	prompt := `Commit abc1234 was merged into the shared "main" branch three
days ago and has already been pulled by other teammates. You must undo its
effect without rewriting any commit history that others have already
pulled. Which single git operation do you use: cherry-pick or revert?
Respond with only one word: cherry-pick or revert.`

	return testkit.Test{
		ID:          "git-cherry-pick-vs-revert",
		Category:    "delivery",
		Subcategory: "git",
		Description: "Pick git revert (not cherry-pick) to undo an already-shared commit without rewriting history.",
		Prompt:      prompt,
		Eval:        eval.Equals("revert"),
	}
}

// delGitignoreFile is the inline .gitignore content for
// delGitGitignoreNegationTraceTest.
const delGitignoreFile = `*.log
build/
!build/keep.txt
docs/*.tmp
!docs/important.tmp`

// delGitignoreTreeFiles is every file path in the inline tree for
// delGitGitignoreNegationTraceTest, in prompt display order.
var delGitignoreTreeFiles = []string{
	"app.go",
	"debug.log",
	"build/output.bin",
	"build/keep.txt",
	"docs/notes.tmp",
	"docs/important.tmp",
	"docs/readme.md",
}

// delGitignoreIgnoredWant is the set of paths from delGitignoreTreeFiles
// that git actually treats as ignored under delGitignoreFile.
//
// ground truth: "*.log" ignores debug.log. "build/" ignores the whole
// build/ directory, which ignores build/output.bin AND build/keep.txt -
// git's documented behavior is that a negation pattern cannot re-include a
// file whose parent directory is itself excluded, since git does not
// descend into an excluded directory to evaluate file-level patterns at
// all, so "!build/keep.txt" has no effect. "docs/*.tmp" ignores
// docs/notes.tmp; "!docs/important.tmp" DOES re-include docs/important.tmp,
// because docs/ itself was never excluded as a directory (only files
// matching *.tmp within it were), so git does descend into docs/ and the
// later, more specific negation pattern wins. app.go and docs/readme.md
// match no pattern. delivery_git_test.go verifies this exact set against a
// real throwaway git repository (git status --ignored / git check-ignore)
// rather than trusting this comment alone, skipping if git is unavailable.
var delGitignoreIgnoredWant = []string{
	"build/keep.txt",
	"build/output.bin",
	"debug.log",
	"docs/notes.tmp",
}

// delGitGitignoreNegationTraceTest: trace which files a .gitignore with
// mixed exclude/negate rules actually ignores, including the
// excluded-parent-directory negation trap.
func delGitGitignoreNegationTraceTest() testkit.Test {
	prompt := `Here is a repository's .gitignore:

` + "```gitignore\n" + delGitignoreFile + "\n```" + `

And here is the full list of files that exist in the working tree:
app.go, debug.log, build/output.bin, build/keep.txt, docs/notes.tmp,
docs/important.tmp, docs/readme.md

Which of these files does git actually treat as ignored? Respond with only
a JSON array of the ignored file paths.`

	return testkit.Test{
		ID:          "git-gitignore-negation-trace",
		Category:    "delivery",
		Subcategory: "git",
		Description: "Trace which files a .gitignore with mixed exclude/negate rules ignores, including the excluded-parent-directory negation trap.",
		Prompt:      prompt,
		Eval:        eval.JSONStringSet(delGitignoreIgnoredWant),
	}
}

// delGitStashPopConflictTest: state whether a conflicting `git stash pop`
// drops the stash entry.
//
// ground truth: "git stash pop" only drops the stash entry after a clean
// apply. On a conflict it leaves the working tree with conflict markers AND
// explicitly keeps the stash entry in the stash list (git itself prints
// "The stash entry is kept in case you need it again"), so no work is lost;
// the user must resolve the conflict and then run "git stash drop"
// manually. delivery_git_test.go verifies this against a real throwaway git
// repository by staging a genuine stash/commit conflict and checking `git
// stash list` afterward, skipping if git is unavailable.
func delGitStashPopConflictTest() testkit.Test {
	prompt := `You run "git stash push" to save uncommitted changes to a
file, then make and commit a new change to that same file, and later run
"git stash pop". The stashed change conflicts with your new commit on that
file. Immediately after "git stash pop" reports the conflict, is the stash
entry dropped from the stash list, or kept? Respond with only one word:
dropped or kept.`

	return testkit.Test{
		ID:          "git-stash-pop-conflict",
		Category:    "delivery",
		Subcategory: "git",
		Description: "State that a conflicting git stash pop keeps the stash entry rather than dropping it.",
		Prompt:      prompt,
		Eval:        eval.Equals("kept"),
	}
}

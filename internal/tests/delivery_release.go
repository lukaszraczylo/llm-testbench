package tests

import (
	"regexp"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerDeliveryReleaseTests(r *testkit.Registry) {
	r.Register(delRelSemverBumpChangelogTest())
	r.Register(delRelCleanTreeRequirementTest())
	r.Register(delRelRollbackOrderingTest())
	r.Register(delRelCanaryVsBlueGreenTest())
	r.Register(delRelTagToReleaseSequenceTest())
	r.Register(delRelCommitChangelogMappingTest())
	r.Register(delRelChecksumsSigningPurposeTest())
	r.Register(delRelPipelineStageOrderingTest())
	r.Register(delRelPinVsRangePolicyTest())
	r.Register(delRelHotfixFlowOrderingTest())
}

// delRelSemverBumpChangelogTest: derive the next semver version from a
// merged-change list dominated by one feat commit.
//
// ground truth: of the four merged changes, the fix is a patch-level
// change, the chore and docs entries are not release-relevant on their own,
// and the feat entry adds a new, backward-compatible, optional query
// parameter - a minor-level change under Semantic Versioning. The highest
// severity present (feat) determines the bump: v2.3.1 with a minor bump
// becomes 2.4.0 (MINOR increments, PATCH resets to 0).
func delRelSemverBumpChangelogTest() testkit.Test {
	prompt := `Since the last release v2.3.1, these changes have merged:

- fix(auth): correct token refresh race condition
- feat(api): add optional "include_deleted" query parameter to GET /items (backward compatible)
- chore(ci): cache go build output between runs
- docs: fix typo in CONTRIBUTING.md

None of these changes removes or renames any existing field, endpoint, or
behavior. Per Semantic Versioning, what is the next version number?
Respond with only a JSON object: {"version":"X.Y.Z"}`

	return testkit.Test{
		ID:          "rel-semver-bump-changelog",
		Category:    "delivery",
		Subcategory: "release-engineering",
		Description: "Derive the next semver version (minor bump to 2.4.0) from a merged-change list dominated by one backward-compatible feat.",
		Prompt:      prompt,
		Eval:        eval.JSONField("version", "2.4.0"),
	}
}

// delRelSilentlyModifyPattern matches "silently modif(y|ies|ying)" -
// delRelCleanTreeRequirementTest's forbidden phrase, deliberately quoted
// from the prompt's own wording so a response that engages with the
// question at all is likely to reuse it, whether to recommend it (wrong,
// unnegated) or warn against it (correct, negated).
var delRelSilentlyModifyPattern = regexp.MustCompile(`(?i)silently modif\w*`)

// delRelCleanTreeRequirementTest: explain why release tooling requires a
// clean git tree, and require that a hook silently mutating tracked files
// mid-release is only ever mentioned in a negated (warned-against) form.
//
// ground truth: release tooling (e.g. goreleaser) refuses to run against a
// dirty tree because the published artifact, its checksums, and its
// signature must correspond exactly to a known, reviewed commit; a hook
// that silently modifies tracked files right before packaging would ship
// code that was never actually committed or reviewed, breaking that
// guarantee, so it must never be allowed - not merely worked around.
func delRelCleanTreeRequirementTest() testkit.Test {
	prompt := `Your release pipeline runs a release tool (for example
goreleaser) against a tagged commit. A teammate proposes adding a pre-build
hook that would silently modify tracked source files (auto-formatting them)
immediately before the tool packages the release, with no separate commit
for the change.

In 2-3 sentences: explain why release tooling requires a clean git working
tree before it runs, and say whether a pre-build hook should be allowed to
silently modify tracked files this way.`

	evaluator := eval.All(
		eval.W(eval.ContainsAny("clean tree", "clean working tree", "clean git tree", "uncommitted", "dirty"), 2),
		eval.W(delNoUnnegatedMention(delRelSilentlyModifyPattern, "no unsafe recommendation to silently modify tracked files"), 2),
	)

	return testkit.Test{
		ID:          "rel-clean-tree-requirement",
		Category:    "delivery",
		Subcategory: "release-engineering",
		Description: "Explain the clean-tree requirement and require any mention of a hook silently mutating tracked files to be negated.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// delRelRollbackOrderingWant is the one defensible incident-response order
// for delRelRollbackOrderingTest.
//
// ground truth: page-oncall first, so the response is coordinated and no
// one else takes a conflicting action at the same time; then
// stop-new-traffic-if-needed, to bound ongoing user harm before the actual
// fix lands; then redeploy-previous-image, the fix itself; then
// confirm-health-checks-pass, verifying the fix actually worked before
// declaring the incident over; then post-incident-notes, written up only
// once the incident is resolved.
var delRelRollbackOrderingWant = []string{
	"page-oncall",
	"stop-new-traffic-if-needed",
	"redeploy-previous-image",
	"confirm-health-checks-pass",
	"post-incident-notes",
}

// delRelRollbackOrderingTest: order the steps of a production rollback
// after a failed deploy.
func delRelRollbackOrderingTest() testkit.Test {
	prompt := `Release v3.1.0 was just deployed and is now failing health
checks in production. The previous release v3.0.4 was healthy. Give the
ordered list of steps to safely roll back to the last known-good release,
choosing from and ordering exactly these labels:
["redeploy-previous-image", "page-oncall", "confirm-health-checks-pass",
"stop-new-traffic-if-needed", "post-incident-notes"]

Respond with only a JSON array of the labels, in the order you would
perform them.`

	return testkit.Test{
		ID:          "rel-rollback-ordering",
		Category:    "delivery",
		Subcategory: "release-engineering",
		Description: "Order a production rollback: page-oncall, stop traffic, redeploy, confirm health, then post-incident notes.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals(delRelRollbackOrderingWant),
	}
}

// delRelCanaryVsBlueGreenTest: pick canary for gradual percentage-based
// rollout, blue-green for instant all-or-nothing cutover.
//
// ground truth: canary is specifically the strategy that exposes a change
// to a small, gradually increasing percentage of real traffic while
// watching metrics, with the ability to halt at any percentage - exactly
// scenario_a. blue-green is specifically the strategy with two full,
// independently warmed environments and an instant all-or-nothing traffic
// switch that can flip back immediately - exactly scenario_b.
func delRelCanaryVsBlueGreenTest() testkit.Test {
	prompt := `For each of these two situations, is "canary" or "blue-green"
the matching deployment strategy?

scenario_a: "Expose a risky new feature to a small percentage (e.g. 5%) of
real production traffic first, gradually increasing that percentage while
watching error-rate metrics, with the ability to automatically halt the
rollout at any percentage if metrics regress."
scenario_b: "An instant, all-or-nothing cutover from the old version to the
new version, using a full, previously warmed standby environment, so 100%
of traffic switches at once and can be flipped back to the old environment
instantly if anything goes wrong."

Respond with only a JSON object:
{"scenario_a":"canary"|"blue-green","scenario_b":"canary"|"blue-green"}`

	evaluator := eval.Mean(
		eval.JSONField("scenario_a", "canary"),
		eval.JSONField("scenario_b", "blue-green"),
	)

	return testkit.Test{
		ID:          "rel-canary-vs-bluegreen",
		Category:    "delivery",
		Subcategory: "release-engineering",
		Description: "Choose canary for a gradual percentage rollout and blue-green for an instant all-or-nothing cutover.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// delRelTagToReleaseWant is the one defensible order from a verified main
// branch to a published GitHub release with notes.
//
// ground truth: run-tests verifies main is releasable before it is tagged
// at all. create-git-tag then tags that verified commit, and push-tag
// publishes the tag to the remote - a prerequisite for anything that
// references it. generate-changelog builds release notes from the commits
// since the last tag. create-github-release publishes the GitHub release
// against the now-pushed tag, using those generated notes. announce comes
// last, only once the release genuinely exists.
var delRelTagToReleaseWant = []string{
	"run-tests",
	"create-git-tag",
	"push-tag",
	"generate-changelog",
	"create-github-release",
	"announce",
}

// delRelTagToReleaseSequenceTest: order the steps from a verified main
// branch to a published, announced GitHub release.
func delRelTagToReleaseSequenceTest() testkit.Test {
	prompt := `Starting from an already-merged, untested-since-merge main
branch, give the ordered sequence of steps to reach a published,
announced GitHub release with generated release notes, choosing from and
ordering exactly these labels:
["run-tests", "create-git-tag", "push-tag", "generate-changelog",
"create-github-release", "announce"]

Respond with only a JSON array of the labels, in the order you would
perform them.`

	return testkit.Test{
		ID:          "rel-tag-to-release-sequence",
		Category:    "delivery",
		Subcategory: "release-engineering",
		Description: "Order tests, tag, push-tag, changelog, GitHub release, and announcement from a merged main branch to a published release.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals(delRelTagToReleaseWant),
	}
}

// delRelCommitChangelogMappingTest: map 4 commits to their Keep a Changelog
// section.
//
// ground truth (Keep a Changelog sections: Added, Changed, Deprecated,
// Removed, Fixed, Security): a plain "feat" commit adds new behavior ->
// Added. A plain "fix" commit -> Fixed. A "perf" commit changes existing
// behavior's characteristics without adding or fixing a defect -> Changed.
// A "fix(security)" commit specifically patches a vulnerability, which Keep
// a Changelog calls out as its own Security section rather than the
// generic Fixed section.
func delRelCommitChangelogMappingTest() testkit.Test {
	prompt := `Using the Keep a Changelog section names (Added, Changed,
Deprecated, Removed, Fixed, Security), map each of these 4 commits to the
single changelog section it belongs in:

commit_a: "feat(runner): add bounded concurrency to the test fan-out via errgroup"
commit_b: "fix(eval): correct off-by-one in numeric extraction"
commit_c: "perf(exec): reuse the go env cache across evaluator calls"
commit_d: "fix(security): patch a path traversal vulnerability in the template loader"

Respond with only a JSON object:
{"commit_a":"...","commit_b":"...","commit_c":"...","commit_d":"..."}`

	evaluator := eval.Mean(
		eval.JSONField("commit_a", "Added"),
		eval.JSONField("commit_b", "Fixed"),
		eval.JSONField("commit_c", "Changed"),
		eval.JSONField("commit_d", "Security"),
	)

	return testkit.Test{
		ID:          "rel-commit-changelog-mapping",
		Category:    "delivery",
		Subcategory: "release-engineering",
		Description: "Map feat/fix/perf/security-fix commits to their Keep a Changelog section, including the Security carve-out.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// delRelChecksumsSigningPurposeTest: explain what a checksums file and its
// signature each protect against.
//
// ground truth: a checksums (e.g. SHA256) file lets a downloader verify the
// binary was not corrupted or truncated in transit - an integrity check.
// On its own, though, an attacker who swaps the binary can just regenerate
// a matching checksum, so a signature over the checksums file is what ties
// it back to a trusted publisher identity - an authenticity/provenance
// check that a bare checksum cannot provide by itself.
func delRelChecksumsSigningPurposeTest() testkit.Test {
	prompt := `A release pipeline publishes SHA256 checksums alongside each
binary artifact, and additionally signs the checksums file with a GPG or
cosign key. In 1-2 sentences, explain what problem each of these two things
solves for someone downloading the release: the checksums file, and the
signature over it.`

	evaluator := eval.All(
		eval.W(eval.ContainsAny("integrity", "corrupt", "tamper", "not been altered", "not been modified"), 2),
		eval.W(eval.ContainsAny("authentic", "provenance", "came from", "verify the publisher", "signed by", "identity"), 2),
	)

	return testkit.Test{
		ID:          "rel-checksums-signing-purpose",
		Category:    "delivery",
		Subcategory: "release-engineering",
		Description: "Explain checksums as an integrity check and the signature over them as an authenticity/provenance check.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// delRelPipelineStageOrderingWant is the one defensible pipeline stage
// order for delRelPipelineStageOrderingTest.
//
// ground truth: lint runs first because it is the fastest check and fails
// fast on style/static issues before spending time on anything else; then
// unit-test (fast, isolated); then build, compiling/packaging only once the
// code is known-good; then integration-test, which needs the built
// artifact; then publish-artifact, only after both test tiers pass;
// deploy-staging promotes the published artifact to a pre-prod
// environment; deploy-production is the final promotion, only after
// staging is verified.
var delRelPipelineStageOrderingWant = []string{
	"lint",
	"unit-test",
	"build",
	"integration-test",
	"publish-artifact",
	"deploy-staging",
	"deploy-production",
}

// delRelPipelineStageOrderingTest: order a CI/CD pipeline's stages.
func delRelPipelineStageOrderingTest() testkit.Test {
	prompt := `Order these CI/CD pipeline stages into the sequence a
well-designed pipeline would run them in, fastest/cheapest checks first and
each stage only running once its prerequisites have passed:
["deploy-production", "build", "lint", "publish-artifact",
"integration-test", "deploy-staging", "unit-test"]

Respond with only a JSON array of the labels, in the order they should
run.`

	return testkit.Test{
		ID:          "rel-pipeline-stage-ordering",
		Category:    "delivery",
		Subcategory: "release-engineering",
		Description: "Order pipeline stages fail-fast-first: lint, unit-test, build, integration-test, publish, staging, production.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals(delRelPipelineStageOrderingWant),
	}
}

// delRelPinVsRangePolicyTest: pick exact pinning for an application's own
// dependencies, and a version range for a published library.
//
// ground truth: an application that is deployed (not consumed by other
// projects) should pin exact versions so every environment resolves the
// identical dependency graph, changing only via an explicit, reviewed bump
// - exactly scenario_a. A published library should instead declare a
// version range, because pinning an exact version would force every
// consumer into dependency-resolution conflicts whenever two libraries in
// their own dependency tree pin different exact versions of the same
// shared dependency - exactly scenario_b.
func delRelPinVsRangePolicyTest() testkit.Test {
	prompt := `For each of these two situations, should dependencies be
declared as an exact pinned version, or as a version range?

scenario_a: "A production application's own direct dependencies, where
every environment (dev, CI, staging, production) must build against
exactly the same resolved dependency graph, with nothing changing
underneath it between releases without an explicit, reviewed bump."
scenario_b: "A publicly published library that other projects will import
as a dependency, where being overly strict would force every consumer into
dependency-resolution conflicts whenever two libraries in their own tree
pin different exact versions of the same shared dependency."

Respond with only a JSON object:
{"scenario_a":"pin"|"range","scenario_b":"pin"|"range"}`

	evaluator := eval.Mean(
		eval.JSONField("scenario_a", "pin"),
		eval.JSONField("scenario_b", "range"),
	)

	return testkit.Test{
		ID:          "rel-pin-vs-range-policy",
		Category:    "delivery",
		Subcategory: "release-engineering",
		Description: "Choose exact pinning for an application's own dependencies and a version range for a published library.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// delRelHotfixFlowOrderingWant is the one defensible order for an urgent
// production hotfix when main has diverged with unrelated unfinished work.
//
// ground truth: branch-from-production-tag starts from exactly the code
// currently running in production, not from main, which carries unrelated
// in-progress work that must not ship early. apply-minimal-fix makes the
// smallest change that resolves the incident, nothing else. run-tests
// verifies the fix before shipping it under time pressure.
// tag-hotfix-release cuts the release from this branch. deploy ships the
// fix to production immediately, the urgent action. merge-back-to-main
// comes last: it matters so the fix is not lost on the next regular
// release, but it does not block getting the fix into production first.
var delRelHotfixFlowOrderingWant = []string{
	"branch-from-production-tag",
	"apply-minimal-fix",
	"run-tests",
	"tag-hotfix-release",
	"deploy",
	"merge-back-to-main",
}

// delRelHotfixFlowOrderingTest: order an urgent hotfix flow when main has
// diverged with unrelated in-progress work that must not ship yet.
func delRelHotfixFlowOrderingTest() testkit.Test {
	prompt := `Production is on fire and needs an urgent hotfix. The "main"
branch has since diverged with unrelated, unfinished work that must NOT
ship as part of this fix. Order these steps into the sequence you would
actually perform them in:
["merge-back-to-main", "tag-hotfix-release", "branch-from-production-tag",
"deploy", "run-tests", "apply-minimal-fix"]

Respond with only a JSON array of the labels, in the order you would
perform them.`

	return testkit.Test{
		ID:          "rel-hotfix-flow-ordering",
		Category:    "delivery",
		Subcategory: "release-engineering",
		Description: "Order an urgent hotfix flow branched from production (not diverged main), through deploy, merging back to main last.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals(delRelHotfixFlowOrderingWant),
	}
}

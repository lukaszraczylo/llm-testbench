package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerSecuritySecretsTests(r *testkit.Registry) {
	r.Register(secHardcodedSecretSpotTest())
	r.Register(secRemediationOrderTest())
	r.Register(secVaultTradeoffTest())
	r.Register(secK8sSecretBase64Test())
	r.Register(secRotationNoDowntimeTest())
	r.Register(secLeastPrivilegeScopeTest())
	r.Register(secDiffSecretSpotTest())
	r.Register(secForkPRCIExposureTest())
	r.Register(secSSHAgentIdentityTest())
	r.Register(secGitignoreHistoryNuanceTest())
}

// secHardcodedSecretFixture is a synthetic Go source file (FIXTURE:
// prompt-only, never compiled) for secHardcodedSecretSpotTest.
const secHardcodedSecretFixture = `1: package main
2:
3: import "net/http"
4:
5: const stripeAPIKey = "sk-live-51H-FIXTURE-0000000000000000"
6:
7: func NewPaymentClient() *http.Client {
8:     return &http.Client{}
9: }`

// secHardcodedSecretSpotTest: spot a live API key hardcoded as a Go
// constant.
//
// ground truth: line 5 hardcodes a live Stripe secret key directly in
// source. It ships to every clone of the repository, and to every commit
// in its history forever, instead of being loaded from an environment
// variable or a secret manager at runtime.
func secHardcodedSecretSpotTest() testkit.Test {
	prompt := `Here is a Go source file. Each displayed line is prefixed with
its 1-based position in this listing:

` + secHardcodedSecretFixture + `

Which line number hardcodes a live secret directly in source? Respond with
only a JSON object: {"line":<number>}`

	return testkit.Test{
		ID:          "sec-hardcoded-secret-spot",
		Category:    "security",
		Subcategory: "secrets",
		Description: "Spot a live Stripe API key hardcoded as a Go constant.",
		Prompt:      prompt,
		Eval:        eval.JSONField("line", 5),
	}
}

// secRemediationOrderWant is the single defensible ordering for
// secRemediationOrderTest.
//
// ground truth: the credential is already compromised the instant it was
// pushed to a shared remote - anyone with clone access, CI logs, or a
// cached fork already has it. Rewriting history does not un-expose a
// credential that has already been fetched elsewhere, so rotating it at
// the source (the provider) is the only step that actually closes the
// exposure, and it must happen first. Only after the live credential is
// rotated does it make sense to (2) remove the secret from the current
// code, (3) rewrite git history to purge it from old commits, and (4)
// force-push and notify collaborators so their existing clones don't
// silently resurrect the purged history on their next pull.
var secRemediationOrderWant = []string{
	"rotate-the-leaked-credential",
	"remove-secret-from-current-code",
	"rewrite-git-history-to-purge-it",
	"force-push-and-notify-collaborators",
}

func secRemediationOrderTest() testkit.Test {
	prompt := `A secret was committed to a git repository and already pushed
to a shared remote that multiple collaborators have cloned.

Order these 4 remediation steps correctly:
["force-push-and-notify-collaborators", "remove-secret-from-current-code",
"rotate-the-leaked-credential", "rewrite-git-history-to-purge-it"]

Respond with only a JSON array containing all 4 step ids in the correct
order.`

	return testkit.Test{
		ID:          "sec-remediation-order",
		Category:    "security",
		Subcategory: "secrets",
		Description: "Order a committed-secret remediation: rotate the credential first, since history rewriting cannot un-expose an already-cloned secret.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals(secRemediationOrderWant),
	}
}

// secVaultTradeoffTest: identify a SealedSecret, not a plain env-var
// literal or a plain (base64) Kubernetes Secret, as safe to commit to a
// GitOps git repository.
//
// ground truth: base64 is a reversible encoding, not encryption, so both a
// literal env var in the Deployment YAML and a plain Kubernetes Secret
// manifest are effectively plaintext the moment they land in git history.
// A SealedSecret is asymmetrically encrypted, decryptable only by the
// private key held inside the cluster's controller, so the ciphertext
// committed to git carries no usable secret to anyone who only has the git
// repository.
func secVaultTradeoffTest() testkit.Test {
	prompt := `A database password must be supplied to a pod in a cluster
whose manifests are all committed to a GitOps git repository. Three ways to
supply it are on the table:

- env-var-literal: the password as a plaintext value directly in the
  Deployment YAML.
- plain-k8s-secret: a Kubernetes Secret manifest (its data field is
  base64-encoded) committed directly to git as-is.
- sealed-secret: a SealedSecret, asymmetrically encrypted so that only the
  cluster's own controller (holding the matching private key) can decrypt
  it, committed to git as ciphertext.

Which one of these three is actually safe to commit to the git repository
as-is? Respond with only a JSON object:
{"safe_to_commit":"<one of: env-var-literal, plain-k8s-secret, sealed-secret>"}`

	return testkit.Test{
		ID:          "sec-vault-tradeoff",
		Category:    "security",
		Subcategory: "secrets",
		Description: "Identify a SealedSecret, not a plaintext env var or a plain base64 Kubernetes Secret, as safe to commit to a GitOps repository.",
		Prompt:      prompt,
		Eval:        eval.JSONField("safe_to_commit", "sealed-secret"),
	}
}

// secK8sSecretBase64Test: confirm base64 encoding provides no
// confidentiality on its own.
//
// ground truth: base64 is a reversible ENCODING with no key involved -
// anyone who can read the manifest text can decode it with a single
// command (base64 -d) to recover the plaintext. It provides zero
// confidentiality by itself; real encryption-at-rest for a Kubernetes
// Secret needs an additional mechanism (etcd encryption, SealedSecrets, or
// an external secret manager).
func secK8sSecretBase64Test() testkit.Test {
	prompt := `A Kubernetes Secret manifest stores its data field values
base64-encoded.

Is base64 encoding, by itself, a form of encryption that keeps the
secret's value confidential from anyone who can read the manifest? Respond
with only one word: yes or no.`

	return testkit.Test{
		ID:          "sec-k8s-secret-base64",
		Category:    "security",
		Subcategory: "secrets",
		Description: "Confirm that Kubernetes Secret base64 encoding is not encryption and provides no confidentiality by itself.",
		Prompt:      prompt,
		Eval:        eval.ExactToken("no"),
	}
}

// secRotationNoDowntimeWant is the single defensible ordering for
// secRotationNoDowntimeTest.
//
// ground truth: during a rolling deploy some replicas are still running
// with the old credential while others have already picked up the new
// one, so the database must accept BOTH credentials throughout the
// rollout. That means: (1) create the new credential alongside the old one
// (both now valid), (2) deploy the config carrying the new credential to
// all replicas (a rolling deploy that takes time, during which both
// credentials must keep working), (3) verify every replica has confirmed
// it is actually using the new credential, and only then (4) revoke the
// old credential. Revoking the old credential any earlier is exactly what
// causes an outage, since replicas that have not yet rolled would suddenly
// fail to authenticate.
var secRotationNoDowntimeWant = []string{
	"create-new-credential-alongside-old",
	"deploy-config-with-new-credential-to-all-replicas",
	"verify-all-replicas-using-new-credential",
	"revoke-old-credential",
}

func secRotationNoDowntimeTest() testkit.Test {
	prompt := `A database password used by a running service with multiple
replicas must be rotated without causing any request to fail
authentication during the rollout.

Order these 4 steps correctly:
["verify-all-replicas-using-new-credential", "revoke-old-credential",
"create-new-credential-alongside-old",
"deploy-config-with-new-credential-to-all-replicas"]

Respond with only a JSON array containing all 4 step ids in the correct
order.`

	return testkit.Test{
		ID:          "sec-rotation-no-downtime",
		Category:    "security",
		Subcategory: "secrets",
		Description: "Order a zero-downtime database-credential rotation across a multi-replica rolling deploy.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals(secRotationNoDowntimeWant),
	}
}

// secLeastPrivilegeScopeTest: pick the credential scoped to exactly what a
// CI job needs, not a broader one that could also do the job.
//
// ground truth: the job's actual need is "push one image to one
// repository." Granting anything broader - org-wide write, or worse,
// account-wide admin - hands the CI runner, and anything that compromises
// it (a malicious build dependency, a leaked runner log), far more power
// than the task requires. The least-privilege-adequate credential is
// scoped to push access on exactly that one repository.
func secLeastPrivilegeScopeTest() testkit.Test {
	prompt := `A CI job's only job is to push a Docker image to one specific
container image repository after a successful build. Nothing else in the
pipeline needs registry access.

Which credential scope should this CI job be given? Respond with only a
JSON object:
{"scope":"<one of: account-wide-admin-key, org-wide-write-key, single-repo-push-key>"}`

	return testkit.Test{
		ID:          "sec-least-privilege-scope",
		Category:    "security",
		Subcategory: "secrets",
		Description: "Pick a single-repository push-only credential over a broader org- or account-wide key for a CI image-push job.",
		Prompt:      prompt,
		Eval:        eval.JSONField("scope", "single-repo-push-key"),
	}
}

// secDiffSecretFixture is a synthetic unified diff (FIXTURE: prompt-only,
// never compiled) for secDiffSecretSpotTest. Each displayed line is
// prefixed with its 1-based position in this listing.
// #nosec G101 -- prompt-only fixture text, never compiled; it demonstrates a
// plaintext credential landing in a diff about to be committed, which is
// exactly the bug this test asks the model to spot.
const secDiffSecretFixture = `1: @@ -10,6 +10,8 @@
2:  func NewMailer() *Mailer {
3:      return &Mailer{
4:          Host: "smtp.example.com",
5: +        Username: "no-reply@example.com",
6: +        Password: "Sup3rSecretSmtpPass!",
7:      }
8:  }`

// secDiffSecretSpotTest: spot the added line that introduces a plaintext
// credential in a diff about to be committed.
//
// ground truth: added line 6 introduces the SMTP account's plaintext
// password literal directly into source about to be committed and pushed -
// a credential leak the moment this diff lands. Added line 5 is a
// non-secret username/email and is not itself a credential leak.
func secDiffSecretSpotTest() testkit.Test {
	prompt := `Here is a unified diff about to be committed. Each displayed
line is prefixed with its 1-based position in this listing:

` + secDiffSecretFixture + `

Which line number introduces a plaintext credential into source about to
be committed? Respond with only a JSON object: {"line":<number>}`

	return testkit.Test{
		ID:          "sec-diff-secret-spot",
		Category:    "security",
		Subcategory: "secrets",
		Description: "Spot the added diff line that introduces a plaintext SMTP password about to be committed.",
		Prompt:      prompt,
		Eval:        eval.JSONField("line", 6),
	}
}

// secForkPRCIExposureTest: confirm repository secrets must not be exposed
// to workflow runs triggered by pull requests from forks.
//
// ground truth: a fork PR's workflow YAML and code are attacker-controlled
// - the fork's author can modify the workflow to print or exfiltrate any
// secret made available to it as an environment variable. Secrets must
// never be exposed to a pull_request-triggered run originating from a
// fork; the standard fix is gating secret-bearing steps to trusted
// triggers (pushes to protected branches, or a maintainer-approved
// pull_request_target with the checkout pinned) instead.
func secForkPRCIExposureTest() testkit.Test {
	prompt := `A CI pipeline runs on every pull request, including ones
opened from forks by contributors who did not write the workflow file
themselves. The pipeline exposes repository secrets (e.g. a deploy token)
as environment variables to that pull request's workflow run.

Should repository secrets be exposed to a workflow run triggered by a pull
request from a fork? Respond with only one word: yes or no.`

	return testkit.Test{
		ID:          "sec-fork-pr-ci-exposure",
		Category:    "security",
		Subcategory: "secrets",
		Description: "Confirm repository secrets must not be exposed to CI workflow runs triggered by pull requests from forks.",
		Prompt:      prompt,
		Eval:        eval.ExactToken("no"),
	}
}

// secSSHAgentIdentityTest: name IdentityAgent as the ssh_config directive
// for a 1Password-style external agent, and explain why it beats a
// disk-resident IdentityFile.
//
// ground truth: IdentityAgent points the ssh client at an external agent's
// socket (e.g. 1Password's) instead of the default ssh-agent. With it, the
// private key material never touches disk at all - it stays inside the
// external agent's vault, unlocked only while that agent is unlocked. An
// IdentityFile pointing at a raw key on disk is a persistent copy of the
// private key sitting in the filesystem indefinitely, which is worse than
// never having a disk copy in the first place, encrypted passphrase or not.
func secSSHAgentIdentityTest() testkit.Test {
	prompt := `You use 1Password as your SSH agent instead of the default
ssh-agent or a key file on disk.

Which ~/.ssh/config directive tells the ssh client to use 1Password's
agent socket instead of the default agent, and why is a hardcoded
private-key IdentityFile path a worse choice for this setup?`

	evaluator := eval.Mean(
		eval.ContainsAll("IdentityAgent"),
		eval.ContainsAny("disk", "filesystem", "never leaves", "never touch", "persistent copy", "sitting on disk"),
	)

	return testkit.Test{
		ID:          "sec-ssh-agent-identity",
		Category:    "security",
		Subcategory: "secrets",
		Description: "Name IdentityAgent as the ssh_config directive for a 1Password-style agent, versus a disk-resident IdentityFile.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// secGitignoreHistoryNuanceTest: confirm a later .gitignore entry and
// working-tree deletion do not remove a secret from earlier git history.
//
// ground truth: .gitignore only stops git from tracking NEW changes to a
// path going forward; deleting the file in a later commit only removes it
// from the CURRENT tree. The file's full contents still exist, unencrypted,
// inside the commit object from 3 commits ago, and a full git clone
// fetches the entire history by default - so the key is trivially
// recoverable (git log -p, git show <commit>:.env) regardless of
// .gitignore or the later deletion. Only rewriting history, or rotating
// the credential, actually removes the exposure.
func secGitignoreHistoryNuanceTest() testkit.Test {
	prompt := `A .env file containing a live API key was committed to git 3
commits ago. Later, .env was added to .gitignore and deleted from the
working tree in the latest commit.

Is the API key still present anywhere a git clone of this repository would
expose, even though .gitignore now excludes .env and it is gone from the
current working tree? Respond with only one word: yes or no.`

	return testkit.Test{
		ID:          "sec-gitignore-history-nuance",
		Category:    "security",
		Subcategory: "secrets",
		Description: "Confirm a later .gitignore entry and working-tree deletion do not remove a secret already present in earlier git history.",
		Prompt:      prompt,
		Eval:        eval.ExactToken("yes"),
	}
}

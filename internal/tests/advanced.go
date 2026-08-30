package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// registerAdvancedTests adds the harder, distractor-rich scenarios designed
// after the 2026-08-30 suite-health review, which found 223/283 tests
// saturated (every model passes). Each test here targets a failure mode
// that still discriminated: multi-constraint reasoning, semantic tracing of
// composed gotchas, forensic root-causing from noisy evidence, and
// constraint math. Every answer is a structured JSON value scored by an
// exact evaluator, never a prose-matching heuristic (the source of every
// false positive in that review). Each lands in an EXISTING subcategory so
// the catalog shape (one pair per category/subcategory) is unchanged.
func registerAdvancedTests(r *testkit.Registry) {
	r.Register(advAgentPlanInfeasibleTest())
	r.Register(advAgentDelegConflictingPolicyTest())
	r.Register(advAgentToolAdversarialDescTest())
	r.Register(advRAGForensicsStageTest())
	r.Register(advLLMContextBudgetTest())
	r.Register(advPgExplainForensicsTest())
	r.Register(advSQLMVCCAnomalyTest())
	r.Register(advDockerLayerRebuildTest())
	r.Register(advRelFreezeWindowTest())
	r.Register(advScenMultiLogRootcauseTest())
	r.Register(advLinuxCapacityFillTest())
	r.Register(advHardComposedTraceTest())
	r.Register(advPyComposedMutableTest())
	r.Register(advCodeConflictingEvidenceTest())
	r.Register(advSecExploitChainTest())
	r.Register(advSecRotationBlastRadiusTest())
}

// advAgentPlanInfeasibleTest: a plan that cannot succeed because a later
// step needs an artifact no earlier step produces. The discriminator is
// admitting infeasibility instead of confabulating an order.
//
// ground truth: step "deploy" needs the built image; the only build step
// ("build") needs the compiled binary from "compile"; "compile" needs
// dependencies from "fetch-deps"; but "fetch-deps" is never listed as
// runnable because it needs network egress the sandbox forbids (stated).
// So the binding blocker is fetch-deps and the plan is infeasible.
func advAgentPlanInfeasibleTest() testkit.Test {
	prompt := `You must order these build steps into a runnable sequence:

- compile: needs the fetched dependencies; produces the binary.
- build-image: needs the binary; produces the container image.
- deploy: needs the container image.
- fetch-deps: needs outbound network egress to the package registry;
  produces the fetched dependencies.

Constraint: this runner has NO outbound network egress (air-gapped), and
no dependency cache is present.

Decide whether a runnable ordering exists. Respond with only a JSON object:
{"feasible": true|false, "blocking_step": "<step id or empty string>"}`

	return testkit.Test{
		ID:          "adv-agent-plan-infeasible",
		Category:    "agents",
		Subcategory: "planning",
		Description: "Detect that an air-gapped runner makes fetch-deps (and the whole chain) infeasible rather than confabulating an order.",
		Prompt:      prompt,
		Eval: eval.All(
			eval.W(eval.JSONField("feasible", false), 2),
			eval.W(eval.JSONField("blocking_step", "fetch-deps"), 1),
		),
	}
}

// advAgentDelegConflictingPolicyTest: pick the one assignment that satisfies
// three simultaneous constraints; the distractors each violate exactly one.
//
// ground truth: the task needs GPU, must finish under a 2h deadline, and
// must run in-region (data residency). agent-cpu-eu has no GPU (violates
// capability); agent-gpu-us is out of region (violates residency);
// agent-gpu-eu-slow takes 3h (violates deadline); agent-gpu-eu-fast meets
// all three. Answer: agent-gpu-eu-fast.
func advAgentDelegConflictingPolicyTest() testkit.Test {
	prompt := `Assign this task to exactly one agent. The task REQUIRES a GPU,
must complete within a 2-hour deadline, and must run in the EU region
(data residency).

- agent-cpu-eu: EU region, no GPU, would finish in 1h.
- agent-gpu-us: US region, has GPU, would finish in 30m.
- agent-gpu-eu-slow: EU region, has GPU, would finish in 3h.
- agent-gpu-eu-fast: EU region, has GPU, would finish in 90m.

Respond with only a JSON object: {"agent": "<agent id>"}`

	return testkit.Test{
		ID:          "adv-agent-deleg-conflicting-policy",
		Category:    "agents",
		Subcategory: "delegation",
		Description: "Choose the single agent satisfying GPU + deadline + region when each distractor violates exactly one constraint.",
		Prompt:      prompt,
		Eval:        eval.JSONField("agent", "agent-gpu-eu-fast"),
	}
}

// advAgentToolAdversarialDescTest: the tool whose NAME suggests the task is
// wrong; the correct tool is identified only by reading descriptions.
//
// ground truth: task is to permanently delete a file. "file_manager" sounds
// right but its description only LISTS files (read-only). "trash_bin"
// sounds like recycling but its description permanently removes a path.
// Answer: trash_bin.
func advAgentToolAdversarialDescTest() testkit.Test {
	prompt := `You must permanently delete the file at /data/tmp/old.log.
Pick the correct tool by its description, not its name:

- file_manager: "Returns a listing of files under a directory. Read-only."
- disk_report: "Reports free and used disk space per mount."
- trash_bin: "Permanently removes the file or directory at the given path
  from disk. Not recoverable."
- archive_tool: "Compresses a path into a .tar.gz beside it."

Respond with only a JSON object: {"tool": "<tool name>"}`

	return testkit.Test{
		ID:          "adv-agent-tool-adversarial-desc",
		Category:    "agents",
		Subcategory: "tool-routing",
		Description: "Select the deletion tool by description when its name (trash_bin) misleads and file_manager sounds right but is read-only.",
		Prompt:      prompt,
		Eval:        eval.JSONField("tool", "trash_bin"),
	}
}

// advRAGForensicsStageTest: given a retrieval trace and a wrong answer,
// name the failing pipeline stage.
//
// ground truth: the embedding query retrieved the right documents (they
// appear in the trace with high scores and contain the fact), the reranker
// kept them top-3, but the generation cited a number found in NONE of the
// retrieved chunks - a generation-stage hallucination, not a retrieval or
// rerank failure. Answer: generation.
func advRAGForensicsStageTest() testkit.Test {
	prompt := `A RAG system answered "the timeout is 45 seconds" but the
correct value is 30. Here is the trace:

- retrieval: top-3 chunks returned, all from the config reference; chunk 1
  (score 0.91) contains the sentence "request_timeout defaults to 30s".
- rerank: order unchanged; chunk 1 still ranked first.
- generation: the model produced "45 seconds"; the string "45" appears in
  none of the three retrieved chunks.

Which single pipeline stage introduced the error? Respond with only a JSON
object: {"stage": "retrieval"|"rerank"|"generation"}`

	return testkit.Test{
		ID:          "adv-rag-forensics-stage",
		Category:    "ai",
		Subcategory: "rag",
		Description: "Attribute a wrong RAG answer to the generation stage when retrieval and rerank both surfaced the correct grounded chunk.",
		Prompt:      prompt,
		Eval:        eval.JSONField("stage", "generation"),
	}
}

// advLLMContextBudgetTest: how many retrieved documents fit given a fixed
// context window and reservations.
//
// ground truth: 200000 window - 8000 output reserve - 1200 system - 300
// query = 190500 available; each doc is 3500 tokens; floor(190500/3500) =
// 54.
func advLLMContextBudgetTest() testkit.Test {
	prompt := `A model has a 200000-token context window. You must reserve
8000 tokens for the output, 1200 for the system prompt, and 300 for the
user query. Each retrieved document is 3500 tokens. How many whole
documents can you fit into the remaining context? Respond with only the
integer.`

	return testkit.Test{
		ID:          "adv-llm-context-budget",
		Category:    "ai",
		Subcategory: "llm-integration",
		Description: "Compute the document count that fits a 200K window after output/system/query reservations (floor(190500/3500)=54).",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], 54, 0),
	}
}

// advPgExplainForensicsTest: read an EXPLAIN ANALYZE plan with a red herring
// and name the real problem.
//
// ground truth: the plan shows a Seq Scan on a 2M-row table with a Filter
// removing 1,999,900 rows (the actual cost driver), while a small Nested
// Loop over 100 rows is a red herring. The fix class is an index on the
// filtered column. Answer: the seq-scan / missing-index problem.
func advPgExplainForensicsTest() testkit.Test {
	prompt := `Here is a Postgres EXPLAIN (ANALYZE) fragment for a slow query:

Nested Loop  (cost=0.29..8.34 rows=100 actual time=0.02..0.14 rows=100)
  ->  Seq Scan on events  (rows=2000000 actual time=0.01..812.4 rows=2000000)
        Filter: (tenant_id = 42)
        Rows Removed by Filter: 1999900
  ->  Index Scan using users_pkey on users  (actual time=0.001..0.001 rows=1)

Planning Time: 0.2 ms
Execution Time: 851.9 ms

Identify the single dominant cause of the slow execution, and the fix
class. Respond with only a JSON object:
{"cause": "seq-scan"|"nested-loop"|"planning"|"index-scan",
 "fix": "add-index"|"raise-work-mem"|"vacuum"|"rewrite-join"}`

	return testkit.Test{
		ID:          "adv-pg-explain-forensics",
		Category:    "databases",
		Subcategory: "postgres",
		Description: "Attribute an 852ms plan to the Seq Scan filtering out ~2M rows (not the cheap nested loop) and pick add-index.",
		Prompt:      prompt,
		Eval: eval.All(
			eval.W(eval.JSONField("cause", "seq-scan"), 2),
			eval.W(eval.JSONField("fix", "add-index"), 1),
		),
	}
}

// advSQLMVCCAnomalyTest: name the concurrency anomaly from an interleaved
// transaction timeline at a stated isolation level.
//
// ground truth: under READ COMMITTED, txn A reads balance=100, then txn B
// commits balance=150, then txn A reads again and sees 150 - the same row
// read twice in one transaction yielded different committed values: a
// non-repeatable read. It is not a dirty read (B committed) nor a phantom
// (same row, not a new row matching a predicate).
func advSQLMVCCAnomalyTest() testkit.Test {
	prompt := `Two transactions run under READ COMMITTED isolation:

t1: A: BEGIN
t2: A: SELECT balance FROM accounts WHERE id=1  -> 100
t3: B: BEGIN
t4: B: UPDATE accounts SET balance=150 WHERE id=1
t5: B: COMMIT
t6: A: SELECT balance FROM accounts WHERE id=1  -> 150
t7: A: COMMIT

Name the concurrency anomaly transaction A experienced. Respond with only a
JSON object:
{"anomaly": "dirty-read"|"non-repeatable-read"|"phantom-read"|"none"}`

	return testkit.Test{
		ID:          "adv-sql-mvcc-anomaly",
		Category:    "databases",
		Subcategory: "sql-tuning",
		Description: "Classify a re-read yielding a different committed value under READ COMMITTED as a non-repeatable read (not dirty or phantom).",
		Prompt:      prompt,
		Eval:        eval.JSONField("anomaly", "non-repeatable-read"),
	}
}

// advDockerLayerRebuildTest: predict exactly which layers rebuild after a
// specific source change.
//
// ground truth: editing a file under src/ busts the "COPY src" layer (C);
// every layer after it rebuilds (D: npm run build); the final-stage
// "COPY --from=build /app/dist" (F) consumes D's output so it rebuilds too.
// The package-file COPY (A) and npm ci (B) are before the change and stay
// cached; the nginx base (E) is unaffected. Answer set: {C, D, F}.
func advDockerLayerRebuildTest() testkit.Test {
	prompt := `Given this Dockerfile, with BuildKit layer caching and a warm
cache from a previous identical build:

  FROM node:20 AS build
  WORKDIR /app
  COPY package.json package-lock.json ./   # layer A
  RUN npm ci                               # layer B
  COPY src ./src                           # layer C
  RUN npm run build                        # layer D
  FROM nginx:alpine                        # layer E
  COPY --from=build /app/dist /usr/share/nginx/html   # layer F

You edit one file inside src/ (no dependency changes). Which layers must
rebuild? Respond with only a JSON array of the layer letters that rebuild,
e.g. ["A","B"].`

	return testkit.Test{
		ID:          "adv-docker-layer-rebuild",
		Category:    "delivery",
		Subcategory: "containers",
		Description: "Predict that a src/ edit rebuilds exactly the COPY src, npm build, and dist-copy layers (C, D, F), not the cached package layers.",
		Prompt:      prompt,
		Eval:        eval.JSONStringSet([]string{"C", "D", "F"}),
	}
}

// advRelFreezeWindowTest: order release steps around a change-freeze
// constraint.
//
// ground truth: the deploy-to-prod step must NOT fall inside the freeze;
// every other step (build, test, stage-deploy, smoke-test) can run during
// the freeze because they do not touch prod. The only step to defer past
// the freeze is deploy-prod. Answer: deploy-prod.
func advRelFreezeWindowTest() testkit.Test {
	prompt := `A change freeze forbids any change to PRODUCTION between now and
Monday 09:00. Your release pipeline has these steps:

- build: compiles artifacts (no environment touched).
- unit-test: runs tests (no environment touched).
- stage-deploy: deploys to the staging environment.
- smoke-test: runs checks against staging.
- deploy-prod: deploys to production.

Which single step must be deferred until after the freeze lifts? Respond
with only a JSON object: {"deferred_step": "<step id>"}`

	return testkit.Test{
		ID:          "adv-rel-freeze-window",
		Category:    "delivery",
		Subcategory: "release-engineering",
		Description: "Identify deploy-prod as the only step a production change freeze blocks, while build/test/stage steps may proceed.",
		Prompt:      prompt,
		Eval:        eval.JSONField("deferred_step", "deploy-prod"),
	}
}

// advScenMultiLogRootcauseTest: interleaved logs from three sources with one
// root cause and red herrings.
//
// ground truth: the load balancer logs 502s, the app logs OOMKilled restart
// events, and the kubelet logs memory-pressure eviction - all downstream of
// the app being OOM-killed. The 502s are a symptom (backend gone), the
// eviction is a symptom (node pressure from the same leak). The root cause
// is the application OOM. Answer: oom.
func advScenMultiLogRootcauseTest() testkit.Test {
	prompt := `Three log streams during an incident, timestamps interleaved:

11:02:01 lb: upstream 10.42.3.9:8080 returned 502
11:02:03 app: container killed, reason OOMKilled, restart #4
11:02:03 kubelet: memory pressure on node gst02, evicting low-priority pods
11:02:05 lb: upstream 10.42.3.9:8080 connection refused
11:02:07 app: heap usage 1.98Gi of 2Gi limit before exit

Identify the single ROOT cause (not a downstream symptom). Respond with
only a JSON object:
{"root_cause": "lb-misconfig"|"oom"|"node-eviction"|"network"}`

	return testkit.Test{
		ID:          "adv-scen-multilog-rootcause",
		Category:    "operations",
		Subcategory: "scenario",
		Description: "Root-cause interleaved lb/app/kubelet logs to the application OOM, treating the 502s and eviction as downstream symptoms.",
		Prompt:      prompt,
		Eval:        eval.JSONField("root_cause", "oom"),
	}
}

// advLinuxCapacityFillTest: linear capacity projection.
//
// ground truth: 500GB volume, 90% alert threshold = 450GB, currently 366GB
// used, growing 12GB/day. (450-366)/12 = 84/12 = 7 whole days until it
// reaches the threshold.
func advLinuxCapacityFillTest() testkit.Test {
	prompt := `A 500 GB data volume is currently 366 GB used and grows by a
steady 12 GB per day. An alert fires when usage reaches 90% of capacity.
In how many whole days from now will usage first reach the 90% alert
threshold? Respond with only the integer number of days.`

	return testkit.Test{
		ID:          "adv-linux-capacity-fill",
		Category:    "operations",
		Subcategory: "linux",
		Description: "Project days until a 500GB volume at 366GB growing 12GB/day reaches the 90% (450GB) threshold: 7.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], 7, 0),
	}
}

// advHardComposedTraceTest: a Go trace composing loop-variable capture, a
// deferred named-return mutation, and closure order.
//
// ground truth (verified by running it): three closures each capture the
// Go 1.22+ per-iteration loop variable i (1, 2, 3) and add it to the named
// return result; result becomes 6; return result + 10 sets result to 16;
// the deferred func doubles it to 32.
func advHardComposedTraceTest() testkit.Test {
	prompt := "In Go 1.22 or later, what does calc() print?\n\n" +
		"```go\n" +
		"func calc() (result int) {\n" +
		"\tdefer func() { result *= 2 }()\n" +
		"\tfns := []func(){}\n" +
		"\tfor i := 1; i <= 3; i++ {\n" +
		"\t\tfns = append(fns, func() { result += i })\n" +
		"\t}\n" +
		"\tfor _, fn := range fns {\n" +
		"\t\tfn()\n" +
		"\t}\n" +
		"\treturn result + 10\n" +
		"}\n" +
		"```\n\n" +
		"Respond with only the integer printed."

	return testkit.Test{
		ID:          "adv-hard-composed-trace",
		Category:    "programming",
		Subcategory: "hard",
		Description: "Trace Go 1.22 per-iteration capture (1+2+3) plus return+10 plus a deferred double: 32.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], 32, 0),
	}
}

// advPyComposedMutableTest: a Python trace composing a mutable default
// argument with a list comprehension of closures.
//
// ground truth (verified by running it): f uses a shared mutable default
// list acc; fns[0]() calls f(0), appending 0 -> returns [0]... but the
// closures capture i by late binding, and range(3) leaves i=2, so each
// lambda calls f(2). fns[0]() -> f(2) appends 2 -> [2]. Then f(10) appends
// to the SAME shared list -> [2, 10]. The second printed line is [2, 10].
func advPyComposedMutableTest() testkit.Test {
	prompt := "What does the SECOND print statement output?\n\n" +
		"```python\n" +
		"def f(x, acc=[]):\n" +
		"    acc.append(x)\n" +
		"    return acc\n" +
		"\n" +
		"fns = [lambda: f(i) for i in range(3)]\n" +
		"print(fns[0]())\n" +
		"print(f(10))\n" +
		"```\n\n" +
		"The second print outputs a list of two integers. Respond with only a " +
		"JSON object naming its two elements in order: " +
		"{\"first_element\": <int>, \"second_element\": <int>}"

	return testkit.Test{
		ID:          "adv-py-composed-mutable",
		Category:    "programming",
		Subcategory: "python",
		Description: "Trace late-binding closure (i=2) plus a shared mutable default list across two calls: second print is [2, 10].",
		Prompt:      prompt,
		Eval: eval.All(
			eval.W(eval.JSONField("first_element", 2), 1),
			eval.W(eval.JSONField("second_element", 10), 1),
		),
	}
}

// advCodeConflictingEvidenceTest: two sources disagree; pick the claim that
// survives with the correct reason.
//
// ground truth: the README says the default port is 8080; the actual
// config file (authoritative, it is what the program reads) says 9090. When
// documentation and the live config disagree, the config the program
// actually loads wins. Answer: 9090, source config.
func advCodeConflictingEvidenceTest() testkit.Test {
	prompt := `Two sources describe the service's listen port:

- README.md (last edited 14 months ago): "The service listens on port
  8080 by default."
- config/server.yaml (the file the running process loads on startup):
  "listen_port: 9090"

There is no environment-variable override. Which port does the running
service actually listen on, and which source is authoritative? Respond with
only a JSON object: {"port": <number>, "authoritative": "readme"|"config"}`

	return testkit.Test{
		ID:          "adv-code-conflicting-evidence",
		Category:    "research",
		Subcategory: "codebase",
		Description: "Resolve a README-vs-config port conflict to the config the process actually loads (9090), not the stale doc.",
		Prompt:      prompt,
		Eval: eval.All(
			eval.W(eval.JSONField("port", 9090), 2),
			eval.W(eval.JSONField("authoritative", "config"), 1),
		),
	}
}

// advSecExploitChainTest: two individually-benign behaviors compose into an
// exploit; name the composed vulnerability class.
//
// ground truth: an open redirect (benign-looking) plus the OAuth flow
// echoing the redirect_uri without an allowlist lets an attacker steal the
// authorization code by redirecting the victim's callback to an
// attacker-controlled host - an account-takeover via open-redirect in the
// OAuth redirect_uri. The class asked for is "open-redirect".
func advSecExploitChainTest() testkit.Test {
	prompt := `Two facts about an app, each individually looking minor:

1. GET /go?next=<url> issues an HTTP 302 to whatever <url> is passed, with
   no host allowlist.
2. The OAuth login uses /go as the post-login landing page and forwards the
   authorization code to the landing URL's query string.

Combined, what vulnerability class does this enable? Respond with only a
JSON object:
{"vuln": "open-redirect"|"csrf"|"sql-injection"|"xss"|"ssrf"}`

	return testkit.Test{
		ID:          "adv-sec-exploit-chain",
		Category:    "security",
		Subcategory: "appsec",
		Description: "Compose an unvalidated redirect with an OAuth code hand-off into an open-redirect account-takeover chain.",
		Prompt:      prompt,
		Eval:        eval.JSONField("vuln", "open-redirect"),
	}
}

// advSecRotationBlastRadiusTest: dual-key rotation-overlap math across
// dependent consumers with different cache TTLs.
//
// ground truth: consumers cache a validated key for up to their TTL, so the
// old key must stay accepted for at least the longest consumer TTL after
// the new key is published, or an in-flight cached validation breaks. TTLs
// are 30, 15, and 5 minutes; the minimum safe overlap window is the max =
// 30 minutes.
func advSecRotationBlastRadiusTest() testkit.Test {
	prompt := `You are rotating a signing key with a dual-key (old+new)
overlap window. Three independent services validate signatures and cache
the validated key material for up to their own TTL before re-fetching:

- service-a: 30-minute cache TTL
- service-b: 15-minute cache TTL
- service-c: 5-minute cache TTL

For how many minutes, at minimum, must the OLD key stay accepted after the
new key is published, so that no service rejects a signature it should
accept? Respond with only the integer number of minutes.`

	return testkit.Test{
		ID:          "adv-sec-rotation-blast-radius",
		Category:    "security",
		Subcategory: "secrets",
		Description: "Size the dual-key overlap window to the longest consumer cache TTL (max(30,15,5)=30 minutes).",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], 30, 0),
	}
}

# Expansion spec — ≥10 tests per subcategory

Orchestrator plan (fable). Authors: sonnet, one per category, isolated worktrees.
Verification: opus, after integration. Baseline commit: 26d8c22.

## Targets (existing → target)

| Category    | Subcategory  | Have | Target |
|-------------|--------------|------|--------|
| programming | golang       | 3    | 10     |
| programming | python       | 2    | 10     |
| programming | typescript   | 1    | 10     |
| programming | c            | 1    | 10     |
| operations  | macos        | 2    | 10     |
| operations  | linux        | 2    | 10     |
| operations  | kubernetes   | 1    | 10     |
| research    | web          | 1    | 10     |
| research    | whitepapers  | 1    | 10     |
| research    | codebase     | 1    | 10     |
| agents      | tool-routing | 1    | 10     |
| agents      | planning     | 1    | 10     |
| agents      | delegation   | 0    | 10 (new subcategory) |

Total ≥130.

## Non-negotiable authoring rules (lessons from two opus reviews + live run)

1. **Self-contained prompts.** Every fixture (code, logs, excerpts, robots.txt, rosters) inline
   in the prompt. No external references, no live web.
2. **Deterministic.** One canonical expected answer — and the evaluator must accept EVERY
   materially-correct form of it. Before writing the evaluator, list 3 plausible correct
   phrasings and 2 plausible wrong answers; assert all 5 in the unit test. If you cannot
   enumerate the correct forms, redesign the test (usually: force a JSON or number-only answer).
3. **Answer-format forcing.** Structured answers: "Respond with only a JSON object/array, no
   other text." Numeric: "Respond with only the number." This is the main determinism lever.
4. **Never bare `NotContains`** for anything a good answer might mention in negated form
   ("do not run kubectl edit"). Use the negation-aware pattern from kubernetes.go if needed.
5. **Numbers:** use `eval.Numeric` + word-boundary-safe extraction. Beware fixtures whose
   prose contains numbers (LP64, x86_64, 64-bit) — prompt must demand number-only replies.
6. **Ground truth discipline:** every constant carries `// ground truth:` derivation; recompute
   in the unit test where cheap (call the same function, compute the math, run the code).
7. **Omit `MaxTokens`** on all new tests (0 = runtime default; reasoning models burn 3k+
   tokens thinking — small caps caused mass truncation zeros in the live run).
8. **No Microsoft content.** No hello-worlds. Ground tests in the operator's real work:
   Go vector-search/HNSW engines, golangci strictness (fieldalignment, gosec), semver/
   conventional commits, goreleaser, Proxmox/LXC (`pct`), Talos/ArgoCD/Traefik/CNPG home
   cluster GitOps, macOS BSD-vs-GNU pain, launchd, 1Password SSH agent, Vue 3 + Vite +
   Tailwind frontends, Encore, ONNX rerankers, embeddings/recall metrics, agent orchestration.
9. **Namespace helpers** with a subcategory prefix (`k8s…`, `py…`, `wp…`) — four authors share
   package `tests`; collisions break the merge.
10. **File ownership:** touch ONLY your category's files in `internal/tests/` (+ their
    `_test.go`). Add new files per subcategory if a file grows past ~600 lines
    (`agents_delegation.go` etc.). NEVER touch `catalog_test.go` (its exact-map assertion is
    being replaced centrally — expect it to fail; verify your work with
    `go test ./internal/tests/ -run '<YourTests>'` and full runs of the other packages).
11. **Exec evaluators** (`GoRun`/`PyRun`/`CRun`) for tests where correctness = behaviour.
    Reuse existing harness patterns from golang.go/python.go/c.go. Budget: exec tests are
    expensive to evaluate — aim for 2-3 per programming subcategory, rest static/numeric/JSON.
12. **Register** every test in your category's register function; keep IDs kebab-case,
    prefixed sensibly (`go-`, `py-`, `ts-`, `c-`, `macos-`, `linux-`, `k8s-`, `web-`,
    `paper-`, `code-`, `agent-`).
13. Gates you must pass in your worktree before committing: `gofmt -l .` empty,
    `go build ./...`, `go vet ./...`, `go test ./internal/... ./cmd/... -run '.'` green
    EXCEPT the known catalog_test exact-map failure, `golangci-lint run` clean,
    `gosec -tests ./...` clean.
14. Finish with ONE commit: `test(catalog): expand <category> to 10 tests per subcategory`.
    Conventional commit, body in active voice, sentences ≤25 words, no contractions.

## Topic menus (author may substitute equivalents of the same difficulty if a topic cannot
## be made deterministic; keep counts ≥10 per subcategory)

### programming/golang (+7)
channel deadlock spot (inline code, "which line blocks forever, why" → JSON {line, reason-keyword});
defer/recover order trace (exact output); slice append aliasing trace (exact output, GoRun or derived
constant); generics constraint fix (broken code inline → fixed code, GoRun); errgroup bounded fan-out
(GoRun harness proving limit); method sets / pointer-vs-value receiver interface satisfaction (JSON);
context cancellation propagation trace (exact); map iteration order question (contains-based, careful).

### programming/python (+8)
softmax of inline vector (numeric, 4dp); asyncio.gather vs sequential await trace (exact);
mutable default argument bug (inline code → "what does third call return" exact); dict/set
comprehension trace (exact); regex log extraction script (PyRun, inline fixture); pathlib rewrite
of os.path code (ContainsAll pathlib idioms + NotContains-style care); generator exhaustion trace
(exact); JSON transform script (PyRun); GIL/threading question with single factual answer (JSON);
f-string format spec output (exact).

### programming/typescript (+9)
discriminated union narrowing trace (exact/JSON); generic utility type authoring
(Contains structural checks); Promise.allSettled result shape trace (JSON); Vue 3 ref vs reactive
unwrapping behaviour (JSON answer); computed vs watch choice scenario (JSON); ESLint v9 flat-config
migration question (ContainsAll: export default, defineConfig/array, no .eslintrc); tsconfig strict
family (JSON: which flag catches inline bug); array method chain trace (exact); optional chaining +
nullish coalescing trace (exact); type-only import syntax (Contains).

### programming/c (+9)
pointer arithmetic trace (numeric); bitmask set/clear/toggle result (numeric); union size LP64
(numeric); string function output (CRun); integer promotion/overflow trace (numeric); array decay
sizeof trace (numeric — inline both sizeof forms); macro expansion pitfall (exact); undefined
behaviour spot (JSON {line, kind}); struct bitfield packing size (numeric, derivation comment);
const pointer vs pointer-to-const (JSON).

### operations/macos (+8)
BSD sed -i in-place syntax (Regex accepting -i '' forms); launchctl bootstrap vs load (Contains);
plutil/defaults plist read command (Regex); mdfind/Spotlight query (Contains); APFS snapshot
create/list commands (Contains); ssh IdentityAgent config for a custom agent socket (Contains —
1Password-style agent); caffeinate usage (Contains); BSD vs GNU stat format flags (Regex/Contains);
dscl or security keychain CLI single-answer task (Contains); zsh PATH precedence question (exact).

### operations/linux (+8)
journalctl unit+time filter command (Regex); cgroup v2 OOM diagnosis from inline memory.events
(JSON); nftables/iptables DNAT one-liner (Regex); LVM extend sequence ordering (JSON array of
steps); ssh -L port-forward command from scenario (Regex); cron expression for given schedule
(exact); systemd timer vs cron unit pair (ContainsAll OnCalendar, WantedBy=timers.target);
/proc/meminfo available-memory interpretation (numeric from inline fixture); rsync
archive+delete+exclude command (Regex); find+xargs -print0 pipeline (Regex).

### operations/kubernetes (+9)
ImagePullBackOff from inline events (JSON {cause, fix-keyword}); Pending pod with node taints
(JSON); PVC Pending with storageclass mismatch (JSON); Service selector mismatch debugging from
inline YAML (JSON {field, value}); HPA not scaling — missing metrics-server (Contains); NetworkPolicy
blocking DNS (ContainsAll port 53/UDP + label fix); ArgoCD OutOfSync causes from inline app status
(JSON); Traefik IngressRoute host rule fix from inline YAML (Regex/JSON); QoS class of inline pod
spec (exact: Guaranteed/Burstable/BestEffort); readiness vs liveness misuse scenario (JSON);
CNPG-style StatefulSet PDB question (Contains).

### research/web (+9)
extract product data from inline HTML to JSON (JSONField set); hreflang reciprocal-link error in
inline snippet (JSON); sitemap.xml rules question with single factual answer (exact); HTTP status
code semantics for scenarios (JSON mapping); CORS preflight scenario — which header missing (exact);
DNS record type for scenario (exact); security headers audit of inline response headers → which
missing (JSON array); URL parsing components of inline URL (JSON); canonical vs 301 choice scenario
(exact); robots.txt crawl-delay/wildcard evaluation on inline file (JSON).

### research/whitepapers (+9)
All excerpt-QA: author writes a technically-correct 120-180 word excerpt inline, asks 2-4 precise
extraction/derivation questions, JSON answers. Topics: product quantization (m, nbits, memory math —
compute compression ratio numeric); LSM-tree compaction (write amplification derivation numeric);
Raft (election timeout ranges, quorum size for N=5 numeric); Bloom filter false-positive rate
(numeric from formula with given m,n,k); transformer attention (d_k scaling reason, JSON);
CAP theorem partition scenario (exact choice); TF-IDF compute for inline mini-corpus (numeric);
B-tree order/fanout height derivation (numeric); recall@k computation from inline result lists
(numeric — retrieval evaluation, on-brand); HNSW distance computations counting (numeric).

### research/codebase (+9)
trace inline Python function output (exact); trace inline TS function output (exact); bug line in
inline diff (JSON {line}); data race spot in inline Go snippet (JSON {variable}); dead code
identification (JSON array of function names); big-O of inline function (exact); import cycle
detection from inline import lists (JSON array); return-type inference of inline generic function
(exact); loop iteration count with off-by-one subtlety (numeric); which commit broke it — git bisect
reasoning over inline commit list + symptom (exact commit id).

### agents/tool-routing (+9)
Variants of roster+tasks→JSON mapping, escalating difficulty: distractor tools with overlapping
descriptions; task needing NO tool (answer from context); multi-step task → ordered tool sequence
(JSON array); cheapest-adequate-tool choice (cost field in roster); parallel-vs-sequential tool
dispatch decision (JSON); tool with missing required parameter — must answer "cannot" (JSON);
ambiguous task → clarify vs act (JSON); tool output → next tool chaining (JSON); read-only vs
mutating tool safety choice (JSON).

### agents/planning (+9)
Dependency-graph orderings (varying graphs, JSON arrays); critical path length (numeric); which
steps parallelize (JSON array of sets — careful: canonicalize ordering in evaluator); rollback
trigger placement (JSON); idempotency — which steps safe to retry (JSON array); plan repair after
step failure (JSON); milestone/dependency date math (numeric); resource-constrained ordering
(JSON); detect the cycle in a task graph (JSON array); minimal replan after requirement change
(JSON).

### agents/delegation (10, new)
Roster mirrors real specialist agents (code-writer, code-reviewer, test-writer, security-reviewer,
devops-debugger, docs-writer, performance-engineer, release-manager, web-researcher, orchestrator).
Tasks → which specialist (JSON); which TWO in sequence for build-then-verify (JSON array); what
context must the handoff include (JSON array of essentials from an enumerated list); when to
verify vs trust (JSON); batch related fixes vs separate dispatches (JSON); main-thread vs delegate
decision for a quick edit (JSON); reviewer independence — same agent must not verify its own work
(JSON); escalation to human criteria (JSON); parallel dispatch safety — shared-file conflict spot
(JSON); minimal-privilege agent choice (JSON).

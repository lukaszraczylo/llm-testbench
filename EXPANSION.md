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

---

# Round 2 — four new categories (databases, security, delivery, ai)

Same non-negotiable authoring rules as above. Every subcategory gets 10 tests.
New-category authors create ONLY new files (`databases*.go`, `security*.go`,
`delivery*.go`, `ai*.go` + `_test.go` siblings) — do NOT touch `catalog.go` or
`catalog_test.go`; the orchestrator wires register calls at integration. Verify
your registrations with a wiring test that calls your register function directly
(see `agents_test.go` TestRegisterAgentsTests_Wiring for the pattern). Prefix
helpers: `db`/`sec`/`del`/`ai`.

### databases/postgres
EXPLAIN plan interpretation (inline plan → JSON seq-vs-index + reason); index choice for inline
query+schema (JSON); N+1 detection in inline code (JSON); isolation anomaly naming for an inline
schedule (exact); deadlock from two inline transactions (JSON lock order); replica failover
behaviour question (contains); bloat/VACUUM diagnosis from inline pg_stat fixture (JSON);
pool sizing from formula (numeric); partial index applicability (JSON); LISTEN/NOTIFY vs polling
scenario (exact).

### databases/redis
Structure choice for 4 scenarios (JSON map); TTL+eviction policy behaviour (exact); INCR
atomicity vs GET/SET race (JSON); MULTI vs pipeline semantics (exact); SCAN vs KEYS in prod
(negation-aware contains); pub/sub delivery guarantees (exact); Lua script atomicity (exact);
memory estimate of inline dataset (numeric); cache stampede mitigation (JSON); keyspace design
anti-pattern spot (JSON).

### databases/sql-tuning
All trace-style where possible — give inline table rows, ask query results: aggregate query
result (numeric); JOIN type row counts (numeric); NULL in WHERE row count (numeric); GROUP BY +
HAVING output (JSON); window function output (JSON); composite index column order for inline
query (JSON array); keyset vs OFFSET pagination (exact); covering index question (exact);
equivalent-rewrite multiple choice (JSON); ORDER BY index serving (JSON).

### security/appsec
Line-spot style on inline code: SQLi (JSON line+fix keyword); XSS sink (JSON); path traversal
(JSON); SSRF (JSON); IDOR/authz gap (JSON); open redirect (JSON line); CSRF requirement scenario
(exact); rate-limit placement (JSON); input validation boundary (contains); secret-in-log spot
(JSON line).

### security/crypto
Password hashing bcrypt/argon2 vs SHA (negation-aware contains); constant-time compare
(contains hmac.Equal/subtle); JWT alg=none pitfall (JSON); TLS floor version (exact); AES-GCM
nonce reuse consequence (contains); crypto/rand vs math/rand (negation-aware contains); key
rotation ordering (JSON array); HMAC vs asymmetric signature choice (exact); cert chain
validation question (exact); hash vs encrypt for PII (JSON).

### security/secrets
Hardcoded secret spot (JSON line); committed-secret remediation ordering — rotate THEN rewrite
history (JSON array); vault/sealed-secrets/env tradeoff (JSON); k8s Secret base64 ≠ encryption
(exact); rotation without downtime ordering (JSON array); least-privilege key scoping (JSON);
secret in inline diff spot (JSON); fork-PR CI secret exposure (exact); SSH agent (1Password-style
IdentityAgent) handling (contains); .gitignore-vs-history nuance (exact).

### delivery/git
bisect step count for N commits (numeric, log2 derivation); rebase vs merge scenario (exact);
conventional-commit classification set (JSON); worktree use case (contains); force-with-lease
vs force (negation-aware contains); hook choice for a policy (exact pre-commit/pre-push/
commit-msg); detached HEAD recovery (JSON array); cherry-pick vs revert (exact); .gitignore
negation trace on inline tree (JSON: ignored files); stash pop conflict behaviour (exact).

### delivery/containers
Layer-cache bust spot in inline Dockerfile (JSON line); ENTRYPOINT+CMD interaction trace
(exact command line); multi-stage size benefit (contains); image size math (numeric); non-root
USER + capabilities (contains); HEALTHCHECK semantics (exact); .dockerignore effect (JSON);
COPY vs ADD (exact); build-arg vs runtime env (JSON); layer count of inline Dockerfile (numeric).

### delivery/release-engineering
semver bump from inline changelog (JSON); release tooling requires clean tree — why (contains);
rollback ordering (JSON array); canary vs blue-green choice (exact); tag → GitHub release
sequence (JSON array); commit-to-changelog section mapping (JSON); checksums/signing purpose
(contains); pipeline stage ordering (JSON array); pin vs range dependency policy (exact);
hotfix flow ordering (JSON array).

### ai/vector-search
cosine vs dot for normalized vectors (exact); recall@k on inline result lists (numeric, fresh
fixture); HNSW efSearch tradeoff direction (exact); PQ memory math (numeric, different params
from whitepapers test); pre- vs post-filtering ANN (exact); RRF hybrid fusion computation
(numeric on inline ranks); distance→similarity conversion (numeric); near-duplicate threshold
reasoning (numeric); index build/query tradeoff (JSON); embedding dimension tradeoff (JSON).

### ai/llm-integration
OpenAI-compatible field semantics (JSON: temperature/max_tokens/stop behaviour); token budget
math for reasoning models — completion budget vs answer size (numeric; ground in this very
framework's truncation incident); 429 retry/backoff policy (JSON); context overflow strategy
(JSON); tool-call response handling trace (JSON); system vs user role placement (exact);
temperature=0 determinism caveat (contains); SSE stream event parsing (exact); embedding batch
efficiency math (numeric); stop-sequence behaviour trace (exact).

### ai/rag
Chunk size tradeoff (JSON); reranker placement in pipeline (JSON array); retrieval failure
mode keyword-vs-semantic for an inline query/corpus (exact); citation grounding requirement
(contains); context selection vs stuffing (exact); index staleness on doc update (JSON);
RAG eval metric choice (JSON); hallucination mitigation ordering (JSON array); multi-hop
decomposition of an inline question (JSON array); pre-assembly dedup rationale (contains).

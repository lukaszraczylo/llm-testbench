# llm-testbench — LLM accuracy testing framework

Plan authored by fable (planner/orchestrator). Implementation: sonnet. Verification: opus.

## Goal

Go framework that runs a catalog of realistic tests (category → subcategory) against
multiple LLMs via an OpenAI-compatible endpoint, scores responses deterministically,
and renders a comparison table.

## Non-goals / constraints

- **No Microsoft-related content anywhere** (no Azure, PowerShell, Windows, TypeScript-the-MS-angle is fine as a language).
- No LLM-as-judge in v1 — deterministic evaluators only.
- Unit tests never hit the network. Live runs are explicit (`llmtest run`).

## Verified facts (grounded this session)

- Endpoint: `https://llm-gateway.example.com/v1` — keyless, standard OpenAI chat/completions.
- Models all available: `uni/deepseek-v4-flash-0731`, `uni/qwen3.6-27b`, `uni/btl-4`.
- Real response observed: content may carry leading whitespace (`"\n\nOK"`) — **normalize before evaluating**: trim, strip a leading `<think>...</think>` block if present.
- Toolchain on this machine: go1.26.5 darwin/arm64. `python3` and `cc` (clang) assumed present on macOS; exec-evaluators must detect and mark `skipped` if a toolchain is missing (score excluded, not zero).

## Architecture

Module: `github.com/lukaszraczylo/llm-testbench`. Go ≥1.23. Dependencies allowed:
`gopkg.in/yaml.v3`, `golang.org/x/sync/errgroup`. Nothing else without a reason.

```
cmd/llmtest/main.go       CLI: run | list. Flags: --config, --category, --subcategory,
                          --models (csv override), --format table|markdown|json,
                          --concurrency, --timeout, --verbose
internal/config/          Config load (YAML) + env override LLMTB_API_KEY, validation
internal/llm/             Client interface + OpenAI-compatible impl (retry w/ backoff ×2
                          on transport/5xx, temperature 0, per-request timeout)
internal/eval/            Evaluator interface + implementations (below)
internal/testkit/         Test type, Registry, response normalization
internal/runner/          model×test fan-out, bounded concurrency (errgroup), Result
internal/report/          table (text/tabwriter), markdown, json renderers
internal/tests/           the catalog: one file per category
```

### Core types (shape, adjust idiomatically)

```go
// llm
type Message struct { Role, Content string }
type Request struct { Model string; Messages []Message; MaxTokens int; Temperature float64 }
type Response struct { Text string; PromptTokens, CompletionTokens int; Latency time.Duration }
type Client interface { Complete(ctx context.Context, req Request) (Response, error) }

// eval
type Score struct { Value float64; Detail string; Skipped bool } // Value 0..1
type Evaluator interface { Evaluate(ctx context.Context, response string) Score }

// testkit
type Test struct {
    ID, Category, Subcategory, Description string
    System, Prompt string
    MaxTokens int
    Eval eval.Evaluator
}

// runner
type Result struct { Model, TestID string; Score eval.Score; Latency time.Duration; Tokens int; Err error }
```

### Evaluators (DRY, generics where natural)

- `ContainsAll(substrings...)` / `ContainsAny(...)` — case-insensitive; partial credit for All (matched/total).
- `NotContains(...)` — composable guard (e.g. must NOT suggest `kubectl edit`).
- `Regex(pattern)` — full credit on match.
- `Equals(want)` — trimmed, case-insensitive.
- `Numeric[T constraints.Float | constraints.Integer](extract func(string) (T, error), want, tol T)` — extracts last number (or JSON field) from response.
- `JSONField(path, want)` — parses first JSON object/array in response (strip code fences), compares field. Use generics for want type.
- `All(weighted ...WeightedEvaluator)` / composite — weighted mean.
- Exec evaluators (compile/run extracted code, compare stdout):
  - `GoRun(harness string)` — extract first fenced code block, append/wrap with per-test harness `main`/driver, `go run` in temp dir (own go.mod), compare stdout to expected. Timeout 30s.
  - `PyRun(harness string)` — same via `python3`.
  - `CRun(harness string)` — compile with `cc`, run, compare stdout.
  - Toolchain missing → `Score{Skipped: true, Detail: "toolchain missing: X"}`.
- Shared helper: `ExtractCodeBlock(response, lang string)` — first matching fenced block, fallback to whole response if no fences.

**Response normalization** (testkit, applied once before eval): trim space, strip leading `<think>…</think>` (and `<reasoning>` variants), collapse Windows line endings.

### Scoring & report

- Per test: 0..1. Skipped tests excluded from aggregates (shown as `skip`).
- Table 1: rows = tests (grouped by category/subcategory), columns = models, cells = `score (Ntok)`
  where Ntok = prompt+completion tokens that test burned on that model — or `ERR`/`skip`.
- Table 2: category rollup — rows = category, columns = models, cells = mean score.
- Table 3: model summary — overall mean, tests passed (≥0.99), partial, failed, errors, mean latency,
  total tokens, and **avg tok/s** = Σ completion_tokens / Σ latency-seconds across all non-skipped tests
  (aggregate ratio, not mean of per-test ratios — robust to short responses).
- `--format markdown` renders the same as GitHub tables; `json` dumps raw results.

### Config (`config.yaml`, example committed as `config.example.yaml`)

```yaml
endpoint: https://llm-gateway.example.com/v1
api_key: ""            # optional; env LLMTB_API_KEY wins
models:
  - uni/deepseek-v4-flash-0731
  - uni/qwen3.6-27b
  - uni/btl-4
concurrency: 4
request_timeout: 120s
max_tokens_default: 4000
```

## Test catalog (16 tests — implement ALL)

Grounded in the operator's real historical tasks (vector search engine work, Proxmox/LXC homelab,
GitOps k8s home cluster, golangci strictness, semver tooling, macOS BSD-vs-GNU pain, Vue 3 frontends,
agent orchestration). No hello-worlds. Exact prompt wording: sonnet authors it, but each prompt must
be fully self-contained (all needed data inline) and pinned to one deterministic expected answer.

**Ground-truth rule (non-negotiable): every constant expected value (numbers, sizes, orderings) must
carry a `// ground truth:` comment showing the derivation, and where cheaply possible the unit test
must recompute it (e.g. compute expected cosine similarity in the Go unit test rather than hardcode).**

### programming/golang
1. `go-struct-align` — give a Go struct with wasteful field order (mix of bool/int64/int32/string/byte);
   ask for reordered struct minimizing padding on 64-bit. Eval: Regex/ContainsAll on required field order.
   Ground truth: derive by alignment rules in comment; unit test may verify with unsafe.Sizeof on both variants.
2. `go-worker-pool` — implement `Pool[T, R any](in []T, workers int, f func(T) R) []R` preserving input order.
   Eval: GoRun harness feeding 100 items with a slow f, checks order + concurrency actually used (duration bound or counter).
3. `go-semver-classify` — given 5 conventional-commit messages (incl. a body containing the word "breaking"
   in a non-breaking sense trap, a `feat:`, a `fix:`, a `chore:`), ask for the resulting semver bump as JSON
   `{"bump":"major|minor|patch"}`. Eval: JSONField.

### programming/python
4. `py-log-triage` — inline ~15 lines of kubelet/pod log output (mixed OOMKilled, ImagePullBackOff, normal);
   ask for a python3 script printing counts per failure type as `type=count` sorted. Eval: PyRun compare stdout.
5. `py-cosine` — two 8-dim float vectors inline; ask for cosine similarity to 4dp (plain number only).
   Eval: Numeric with tol 0.0005. Ground truth computed in unit test with math.

### programming/typescript
6. `ts-debounce-composable` — Vue 3 composable `useDebouncedRef<T>(source: Ref<T>, ms: number): Ref<T>`
   with proper cleanup. Eval: ContainsAll(`export function useDebouncedRef`, `watch`, `clearTimeout`/`onScopeDispose` any, generic `<T>`) + NotContains(`any`) — weighted composite.

### programming/c
7. `c-struct-size` — C struct with char/double/int/short members inline; ask `sizeof` on LP64 (answer: number only).
   Eval: Numeric exact. Ground truth: derivation comment + optional CRun cross-check in unit test guarded by cc presence.

### operations/macos
8. `macos-timeout-portability` — script fails with `timeout: command not found` on stock macOS; why + two portable fixes.
   Eval: composite ContainsAny(coreutils|gtimeout) + ContainsAny(perl -e 'alarm'|background & kill|builtin) + ContainsAll(GNU).
9. `macos-launchd-cron` — persistent job on macOS that survives reboot and runs every 5 min: which mechanism + minimal plist keys.
   Eval: ContainsAll(launchd/launchctl, StartInterval or StartCalendarInterval, LaunchAgents/LaunchDaemons).

### operations/linux
10. `linux-pct-exec` — CT 251 lives on Proxmox host 10.0.0.100, not directly reachable from laptop; one-line command
    to check `systemctl status myservice` inside it. Eval: Regex requiring `ssh` + `10.0.0.100` + `pct exec 251` + command.
11. `linux-systemd-oneshot` — write a systemd unit that runs a script once at boot after network-online, no restart loops.
    Eval: ContainsAll(`Type=oneshot`, `network-online.target`, `WantedBy=multi-user.target`, `RemainAfterExit` optional credit).

### operations/kubernetes
12. `k8s-crashloop-gitops` — inline `kubectl describe` excerpt: CrashLoopBackOff, last state OOMKilled exit 137,
    limits 128Mi. Cluster is ArgoCD-managed. Ask: root cause + correct fix procedure. Eval: composite
    ContainsAll(OOM/memory, limit raise) + ContainsAny(git commit|ArgoCD|GitOps) + NotContains(`kubectl edit`, `kubectl patch`).

### research/web
13. `web-robots-ai-crawlers` — inline a robots.txt with mixed rules (GPTBot disallowed, ClaudeBot allowed,
    PerplexityBot disallowed, Googlebot allowed); ask which AI crawlers can fetch `/docs/`. JSON array answer.
    Eval: JSONField/exact set compare.

### research/whitepapers
14. `paper-hnsw-params` — inline a 150-word excerpt describing HNSW parameters (M, efConstruction, efSearch —
    sonnet writes a technically correct excerpt); ask three precise extraction questions answered as JSON.
    Eval: JSONField ×3 composite.

### research/codebase
15. `code-trace-go` — inline a ~20-line Go function (bit manipulation + recursion, deterministic); ask for exact
    output for a given input, number only. Eval: Numeric exact. Ground truth: unit test RUNS the same function.

### agents (subcategories: tool-routing, planning)
16. `agent-tool-routing` — inline roster of 6 tools with descriptions (search_web, read_file, run_shell,
    query_db, send_email, none) + 4 mini-tasks; ask JSON mapping task→tool. Eval: JSONField ×4.
17. `agent-plan-ordering` — deployment scenario with dependencies (build→test→backup→deploy→verify→rollback-on-fail);
    ask for ordered JSON array of step ids. Eval: exact array compare (order matters).

(16–17: both under `agents`; subcategories `tool-routing` and `planning`. Total = 17 tests.)

## Quality gates (must pass before you report done)

1. `go build ./...`
2. `go vet ./...`
3. `go test ./...` — offline only, mock Client; evaluator tests cover good/bad/edge; exec-evaluator tests
   guarded by toolchain detection.
4. `golangci-lint run` if the binary exists — must be clean incl. fieldalignment (fix orders, do not disable linters).
5. `gofmt -l .` empty.

## Deliverables

- Full source per layout above, `go.mod`, `config.example.yaml`, `.gitignore` (binaries, config.yaml), `README.md`
  (usage, adding a test, adding an evaluator).
- Do NOT `git commit` — orchestrator commits after opus verification.
- Do NOT call the live endpoint anywhere in code paths exercised by `go test`.

## Report back

Files created; output of each quality gate (pass/fail + failure detail); any deviation from this plan with one-line reason; open concerns.

# llm-testbench

llm-testbench runs a catalog of realistic tests against multiple language
models through an OpenAI-compatible endpoint, scores each response with a
deterministic evaluator, and renders a comparison table. It does not use an
LLM as a judge.

## Requirements

- Go 1.25 or later.
- `python3` and `cc` on PATH, only if you want the Python and C
  exec-evaluator tests to run instead of skip.
- An OpenAI-compatible chat completions endpoint to test against.

**Warning: the `go-worker-pool`, `py-log-triage`, and `c-struct-size`
exec-evaluator tests compile and run model-generated code** (`internal/eval`'s
`GoRun`/`PyRun`/`CRun`), **with the same privileges as the `llmtest`
process itself.** Each call runs in its own temp directory with a minimal
environment (`PATH`, an isolated `HOME`/`TMPDIR`, and `GOCACHE`/`GOMODCACHE`
for `GoRun`), not the operator's full environment - but this is not a
sandbox, and it does not stop a malicious payload from reading files the
operating-system user can read or making outbound network calls. Run
`llmtest run` in a throwaway VM or container if you test models you do not
already trust.

## Setup

Copy the example config and set your endpoint and models:

```sh
cp config.example.yaml config.yaml
```

Set an API key through the config file's `api_key` field, or through the
`LLMTB_API_KEY` environment variable. The environment variable always wins
over the file value.

## Usage

List the test catalog:

```sh
go run ./cmd/llmtest list
go run ./cmd/llmtest list --category programming --subcategory golang
```

Run the catalog against the configured models:

```sh
go run ./cmd/llmtest run
go run ./cmd/llmtest run --category operations --format markdown
go run ./cmd/llmtest run --models uni/deepseek-v4-flash-0731,uni/qwen3.6-27b
```

### Flags

Both `list` and `run` accept:

- `--config` - path to the YAML config file. Default: `config.yaml`.
- `--category` - filter to one category (for example `programming`).
- `--subcategory` - filter to one subcategory (for example `golang`).

`run` also accepts:

- `--models` - comma-separated model list, overriding the config file.
- `--format` - `table`, `markdown`, or `json`. Default: `table`.
- `--tests` - comma-separated exact test IDs (no globs), overriding
  `--category`/`--subcategory` - for probing individual tests cheaply.
- `--repeat N` - sample each (model, test) pair N times. Sampling
  temperature is pinned to 0, so disagreement between repeats
  exposes response instability (server-side batching, MoE routing) rather
  than decoding randomness. Default: 1.
- `--concurrency` - override the config file's `concurrency`.
- `--timeout` - override the config file's `request_timeout` (for example
  `60s`).
- `--out FILE` - write the run as a JSON artifact (raw per-attempt results
  plus the per-test discrimination rollup) for later `llmtest compare`.
- `--quiet` - suppress progress output on stderr. Progress is on by
  default: a run can issue hundreds of requests at 30-50s each, and a
  silent CLI reads as a hang.

By default, `run` prints one line to stderr at the start ("running N tests
x M models = K requests, concurrency C") and one line per completed
request ("[done/total] model=... test=... score=... tokens=... (latency)",
with a `TRUNCATED` marker when a response was cut off by the token
budget). stdout carries only the table/markdown/json report, so `llmtest
run --format json > results.json` stays pipeable regardless of `--quiet`.

With `--repeat N>1`, each (model, test) cell reports the mean over
attempts; when attempts disagreed, the cell appends the `[min-max]` range
and the discrimination rollup flags the test as unstable.

### Comparing two runs

`compare` diffs two saved artifacts (from `--out`) per (model, test) pair.
It re-aggregates each side with the same truncation policy the report
uses, so a comparison stays honest even when only some attempts were cut
off. A mean drop of >= 0.01 is a regression, an equal-size rise an
improvement; tests present on only one side are listed as
`baseline-only`/`current-only`. The exit code reflects only diff
failures (unreadable artifacts), not regressions:

```sh
go run ./cmd/llmtest run --out baseline.json
# ... change config, upgrade the endpoint, swap model ...
go run ./cmd/llmtest run --out current.json
go run ./cmd/llmtest compare baseline.json current.json
```

### Auditing suite health

`health` reads one or more saved artifacts and rolls them up per
subcategory: what share of (model, test) cells score perfect, how many
tests are saturated (perfect in >= 90% of their cells, so they cannot
separate models), and which tests still carry signal. Cells pool across
artifacts taking each (model, test) pair's WORST artifact, so a lucky
re-run cannot hide a failure; subcategories at 100% perfect across every
model contribute zero ranking information and need harder tests:

```sh
go run ./cmd/llmtest run --out run1.json
go run ./cmd/llmtest health run1.json              # one artifact
go run ./cmd/llmtest health run1.json run2.json    # pooled audit
```

### Reading the discrimination rollup

`table` and `markdown` output end with a `Discrimination` section. Its
headline counts the tests whose cross-model score spread reaches
`DiscriminationSpread` (0.05) - the tests that actually separate models -
plus the tests whose repeated attempts disagreed. A suite where nearly
every test scores 1.00 for every model measures nothing about relative
capability, and this section makes that visible per test instead of
averaging it away; with a single model there is no spread, so the section
reports instability only.

If `config.yaml` omits `concurrency`, `request_timeout`, or
`max_tokens_default`, `internal/config` fills in `8`, `300s`, and `12000`.
The effective per-call token budget is `max(test.MaxTokens,
max_tokens_default)`: a test's own `MaxTokens` can only raise the budget
above the config default, never lower it below it. This matters for
reasoning models, which can spend thousands of completion tokens on
`reasoning_content` before writing an answer to `content`; a low budget
truncates the answer before the model reaches it.

### Output

`table` and `markdown` render three views: per-test scores grouped by
category and subcategory, a category rollup with mean score per model, and
a model summary with pass/partial/fail/error counts, mean latency, total
tokens, and average tokens per second. `json` dumps the raw per-test
results, joined with each test's category and subcategory.

A scored cell in the per-test table reads `score (Ntok)`, where `Ntok` is
the prompt plus completion tokens that test burned on that model (for
example `0.85 (312tok)`). The model summary's `TOK/S` column is an
aggregate ratio, sum of completion tokens over sum of elapsed seconds
across all non-skipped, non-error calls, not a mean of each test's own
ratio; this keeps a handful of very short responses from skewing the rate.

A cell reads `ERR` when the model call itself failed (a transport error or
a non-2xx response after retries), and `skip` when the test's evaluator
needs a local toolchain that is not installed (for example, a Python
exec-evaluator when `python3` is missing). Skipped tests are excluded from
every mean.

A cell carries a trailing `!` when at least one of its attempts was cut
off by the token budget (`finish_reason=length`) - for example
`1.00 (420tok)!`. Truncation policy: a truncated attempt was scored on
partial text, so its score measures the token budget as much as the
model. Attempts that finished normally are therefore the only ones
averaged into the cell's mean and its `[min-max]` range; the truncated
sample is discarded and marked by the `!`. Only when every scored attempt
was truncated does the mean fall back to the partial-text scores - then
the `!` means the whole cell is unreliable and `max_tokens_default` needs
rising before the number can be read as capability. `compare` reuses this
policy, so a truncated sample in one artifact cannot fake a regression.
`table` and `markdown` print a one-line legend explaining `!` whenever at
least one cell has it. `json` carries the signal per attempt as
`"truncated": true`, plus `"finish_reason"` and `"response_text"` (the
exact normalized text that was scored) so a truncated or unexpectedly
low-scoring result can be debugged without re-running the model.

## Test catalog

The catalog lives in `internal/tests`, one file per category. Every test
has a fully self-contained prompt (all data needed to answer is inline) and
a single deterministic evaluator. Every hardcoded expected value carries a
`// ground truth:` comment showing its derivation, and where the derivation
is cheap, the test file's `_test.go` recomputes it independently (for
example, `internal/tests/python_test.go` recomputes the cosine similarity
with `math.Sqrt` rather than trusting the constant).

1063 tests across 105 subcategories (each holds at least 10, enforced by
`internal/tests/catalog_test.go`). Subcategory minimums and the exact ID
list are the source of truth; `llmtest list` prints them live.

| Category / subcategory | Tests |
| --- | --- |
| programming / golang (10) | `go-bounded-fanout`, `go-channel-deadlock`, `go-context-cancellation-trace`, `go-defer-recover-trace`, `go-generics-constraint-fix`, `go-method-set-interface`, `go-semver-classify`, `go-slice-append-aliasing`, `go-struct-align`, `go-worker-pool` |
| programming / python (10) | `py-asyncio-gather-trace`, `py-cosine`, `py-dict-comprehension-trace`, `py-generator-exhaustion-trace`, `py-json-transform`, `py-log-triage`, `py-mutable-default-arg`, `py-pathlib-rewrite`, `py-regex-log-extraction`, `py-softmax` |
| programming / typescript (10) | `ts-array-method-chain`, `ts-computed-vs-watch`, `ts-debounce-composable`, `ts-discriminated-union-narrowing`, `ts-eslint-flat-config`, `ts-generic-utility-type`, `ts-optional-chaining-nullish`, `ts-promise-allsettled-trace`, `ts-tsconfig-strict-flag`, `ts-vue-ref-reactive-unwrap` |
| programming / c (10) | `c-array-decay-sizeof`, `c-bitmask-ops`, `c-integer-promotion-overflow`, `c-macro-expansion-pitfall`, `c-pointer-arithmetic`, `c-string-function-output`, `c-struct-bitfield-packing`, `c-struct-size`, `c-undefined-behavior-spot`, `c-union-size-lp64` |
| programming / hard (12) | `hard-context-timeout`, `hard-defer-named-return`, `hard-float-binadd`, `hard-go-paren-balance`, `hard-goroutine-race`, `hard-int-division`, `hard-json-unmarshal-null`, `hard-nullish-over-or`, `hard-promise-allsettled`, `hard-py-version-sort`, `hard-regex-greedy`, `hard-var-shadow` |
| operations / macos (10) | `macos-apfs-snapshot`, `macos-caffeinate`, `macos-launchctl-bootstrap`, `macos-launchd-cron`, `macos-mdfind-spotlight`, `macos-plutil-plist-read`, `macos-sed-inplace`, `macos-ssh-identity-agent`, `macos-stat-bsd-gnu`, `macos-timeout-portability` |
| operations / linux (10) | `linux-cgroup-oom-diagnosis`, `linux-cron-expression`, `linux-journalctl-filter`, `linux-lvm-extend-order`, `linux-nftables-dnat`, `linux-pct-exec`, `linux-proc-meminfo-available`, `linux-ssh-port-forward`, `linux-systemd-oneshot`, `linux-systemd-timer` |
| operations / kubernetes (10) | `k8s-argocd-outofsync`, `k8s-cnpg-pdb`, `k8s-crashloop-gitops`, `k8s-imagepullbackoff`, `k8s-networkpolicy-dns-block`, `k8s-pending-taints`, `k8s-pvc-storageclass-mismatch`, `k8s-qos-class`, `k8s-service-selector-mismatch`, `k8s-traefik-ingressroute-host` |
| operations / scenario (11) | `scen-backup-restore-order`, `scen-certexpiry-triage`, `scen-crashloop-order`, `scen-diskpressure-firstcmd`, `scen-inode-exhaustion`, `scen-log-rootcause-mcq`, `scen-lvm-resize-order`, `scen-net-layer-isolate`, `scen-portbind-fail`, `scen-service-degrade`, `scen-systemd-failreason` |
| research / web (12) | `web-canonical-vs-redirect`, `web-cors-missing-header`, `web-dns-mx-record`, `web-hreflang-reciprocal`, `web-hreflang-self-reference-trap`, `web-html-product-extract`, `web-http-status-scenarios`, `web-robots-ai-crawlers`, `web-robots-ua-scope-trap`, `web-security-headers-audit`, `web-sitemap-max-urls`, `web-url-parse-components` |
| research / whitepapers (10) | `paper-attention-scale-factor`, `paper-bloom-fp-rate`, `paper-btree-height-levels`, `paper-cap-availability-choice`, `paper-hnsw-params`, `paper-lsm-write-amplification`, `paper-pq-compression-ratio`, `paper-raft-quorum-failures`, `paper-recall-at-k`, `paper-tfidf-term-score` |
| research / codebase (10) | `code-bigo-dedup`, `code-bug-line-diff`, `code-commit-bisect`, `code-dead-functions`, `code-generic-return-type`, `code-import-cycle`, `code-race-variable`, `code-trace-go`, `code-trace-python`, `code-trace-ts` |
| agents / tool-routing (10) | `agent-tool-routing`, `agent-tool-routing-ambiguous`, `agent-tool-routing-chaining`, `agent-tool-routing-cheapest`, `agent-tool-routing-distractors`, `agent-tool-routing-missing-param`, `agent-tool-routing-multistep`, `agent-tool-routing-no-tool-needed`, `agent-tool-routing-parallel`, `agent-tool-routing-safety` |
| agents / planning (10) | `agent-plan-critical-path`, `agent-plan-cycle-detect`, `agent-plan-idempotency`, `agent-plan-milestone-date`, `agent-plan-minimal-replan`, `agent-plan-ordering`, `agent-plan-parallel-steps`, `agent-plan-repair`, `agent-plan-resource-constrained`, `agent-plan-rollback-trigger` |
| agents / delegation (12) | `agent-deleg-batch-vs-separate`, `agent-deleg-build-then-verify`, `agent-deleg-capacity-trap`, `agent-deleg-escalation-to-human`, `agent-deleg-handoff-context`, `agent-deleg-main-thread-vs-delegate`, `agent-deleg-minimal-privilege`, `agent-deleg-near-miss-roster`, `agent-deleg-parallel-dispatch-safety`, `agent-deleg-reviewer-independence`, `agent-deleg-task-to-specialist`, `agent-deleg-verify-vs-trust` |
| databases / postgres (12) | `pg-bloat-vacuum`, `pg-count-star-vs-join`, `pg-deadlock-lock-order`, `pg-explain-seq-vs-index`, `pg-index-choice`, `pg-isolation-anomaly`, `pg-listen-notify`, `pg-n-plus-one`, `pg-partial-index`, `pg-pool-sizing`, `pg-replica-failover`, `pg-timeout-trap` |
| databases / redis (10) | `redis-cache-stampede`, `redis-incr-atomicity`, `redis-keyspace-anti-pattern`, `redis-lua-atomicity`, `redis-memory-estimate`, `redis-multi-vs-pipeline`, `redis-noeviction-write`, `redis-pubsub-delivery`, `redis-scan-vs-keys`, `redis-structure-choice` |
| databases / sql-tuning (10) | `sql-aggregate-sum`, `sql-composite-index-skip-column`, `sql-covering-index`, `sql-equivalent-rewrite`, `sql-groupby-having`, `sql-join-counts`, `sql-keyset-pagination`, `sql-null-in-where`, `sql-orderby-index-serving`, `sql-window-rank` |
| security / appsec (12) | `sec-csrf-post-trap`, `sec-csrf-requirement`, `sec-idor-spot`, `sec-input-validation-boundary`, `sec-open-redirect-spot`, `sec-orm-interpolation-trap`, `sec-path-traversal-spot`, `sec-rate-limit-placement`, `sec-secret-log-spot`, `sec-sqli-spot`, `sec-ssrf-spot`, `sec-xss-sink-spot` |
| security / crypto (10) | `sec-aes-gcm-nonce-reuse`, `sec-cert-chain-validation`, `sec-constant-time-compare`, `sec-hash-vs-encrypt-pii`, `sec-hmac-vs-signature`, `sec-jwt-alg-none`, `sec-password-hash-choice`, `sec-rand-source-choice`, `sec-rotate-order`, `sec-tls-floor-version` |
| security / secrets (12) | `sec-diff-secret-spot`, `sec-error-path-leak-trap`, `sec-fork-pr-ci-exposure`, `sec-gitignore-history-nuance`, `sec-hardcoded-secret-spot`, `sec-k8s-secret-base64`, `sec-least-privilege-scope`, `sec-remediation-order`, `sec-rotation-no-downtime`, `sec-rotation-window-math`, `sec-ssh-agent-identity`, `sec-vault-tradeoff` |
| delivery / git (10) | `git-bisect-steps`, `git-cherry-pick-vs-revert`, `git-conventional-commit-classify`, `git-detached-head-recovery`, `git-force-with-lease-vs-force`, `git-gitignore-negation-trace`, `git-hook-choice`, `git-rebase-vs-merge`, `git-stash-pop-conflict`, `git-worktree-use-case` |
| delivery / containers (10) | `docker-build-arg-vs-env`, `docker-copy-vs-add`, `docker-dockerignore-effect`, `docker-entrypoint-cmd-trace`, `docker-healthcheck-semantics`, `docker-image-size-math`, `docker-layer-cache-bust`, `docker-layer-count`, `docker-multistage-size-benefit`, `docker-nonroot-user-caps` |
| delivery / release-engineering (10) | `rel-canary-vs-bluegreen`, `rel-checksums-signing-purpose`, `rel-clean-tree-requirement`, `rel-commit-changelog-mapping`, `rel-hotfix-flow-ordering`, `rel-pin-vs-range-policy`, `rel-pipeline-stage-ordering`, `rel-rollback-ordering`, `rel-semver-bump-changelog`, `rel-tag-to-release-sequence` |
| ai / vector-search (10) | `vec-cosine-vs-dot`, `vec-distance-to-similarity`, `vec-embedding-dimension-tradeoff`, `vec-hnsw-efsearch-tradeoff`, `vec-index-build-query-tradeoff`, `vec-near-duplicate-threshold`, `vec-pq-memory-math`, `vec-pre-vs-post-filtering`, `vec-recall-at-k`, `vec-rrf-fusion` |
| ai / llm-integration (10) | `llm-429-retry-backoff`, `llm-context-overflow-strategy`, `llm-embedding-batch-math`, `llm-field-semantics`, `llm-role-placement`, `llm-sse-stream-done`, `llm-stop-sequence-trace`, `llm-temperature-zero-caveat`, `llm-token-budget-reasoning`, `llm-tool-call-handling` |
| ai / rag (10) | `rag-chunk-size-tradeoff`, `rag-citation-grounding`, `rag-context-selection-vs-stuffing`, `rag-eval-metric-choice`, `rag-hallucination-mitigation-ordering`, `rag-index-staleness`, `rag-multihop-decomposition`, `rag-preassembly-dedup`, `rag-reranker-placement`, `rag-retrieval-failure-mode` |
| daily / text-processing (10) | `dtext-cron-dst-skip`, `dtext-csv-revenue`, `dtext-duration-sum`, `dtext-invoice-vat-rounding`, `dtext-json-merge-patch`, `dtext-meeting-zones`, `dtext-percent-symmetry-trap`, `dtext-sla-date-math`, `dtext-url-components`, `dtext-utf8-bytes-vs-chars` |
| daily / terminal (10) | `dterm-cp-n-versus-mv`, `dterm-du-total-max`, `dterm-find-mtime-plus7`, `dterm-grep-lines-vs-matches`, `dterm-log-top-2xx-path`, `dterm-pipefail-exit-code`, `dterm-ps-memory-column`, `dterm-single-quote-expansion`, `dterm-strip-port-uniq-count`, `dterm-tail-grep-buffering` |
| daily / office-math (10) | `doffice-discount-tax-order`, `doffice-file-quota-gib`, `doffice-fx-wire-fee`, `doffice-mileage-threshold`, `doffice-overtime-pay`, `doffice-payroll-loaded`, `doffice-prorated-fee`, `doffice-reams-ceiling`, `doffice-shift-roster`, `doffice-uptime-budget` |
| daily / policy-rules (10) | `dpol-approval-boundaries`, `dpol-cert-expiry-window`, `dpol-expense-per-night-cap`, `dpol-holiday-premium`, `dpol-marginal-brackets`, `dpol-oncall-rotation`, `dpol-retention-window`, `dpol-shipping-business-days`, `dpol-sla-tiered-credit`, `dpol-vacation-accrual-cap` |
| daily / scheduling (10) | `dsched-billing-anchor-clamp`, `dsched-contract-notice-window`, `dsched-deadline-countdown`, `dsched-dst-backup-hours`, `dsched-dst-meeting-overlap`, `dsched-flight-cross-tz`, `dsched-payroll-period-walk`, `dsched-rota-mod7`, `dsched-utc-cross-date`, `dsched-weekend-shift-deadline` |
| daily / shopping (10) | `dshop-bill-split-penny`, `dshop-buy-one-free`, `dshop-coupon-threshold`, `dshop-free-shipping-threshold`, `dshop-fuel-range-check`, `dshop-fuel-trip-cost`, `dshop-pack-mix-optimum`, `dshop-stacked-discount`, `dshop-unit-price-per-litre`, `dshop-vat-cashback` |
| daily / finance (10) | `dfin-commute-season-ticket`, `dfin-compound-vs-simple`, `dfin-deposit-cap-weekly`, `dfin-fx-round-trip`, `dfin-income-tax-single-band`, `dfin-installment-vs-lump`, `dfin-min-payment-interest`, `dfin-prorata-rent`, `dfin-raise-compounding`, `dfin-simple-interest-18mo` |
| daily / travel (10) | `dtrv-baggage-started-kg`, `dtrv-car-hire-80h`, `dtrv-hotel-3-nights`, `dtrv-layover-share`, `dtrv-mileage-tiers`, `dtrv-taxi-split-surge`, `dtrv-three-city-meeting`, `dtrv-toll-vs-detour`, `dtrv-train-connection`, `dtrv-visa-window` |
| daily / cooking (10) | `dck-bakers-percentages`, `dck-coffee-unit-price`, `dck-cordial-dilution`, `dck-dinner-parallel-starts`, `dck-granola-label-math`, `dck-pan-area-scaling`, `dck-roast-start-timing`, `dck-scale-recipe-4-to-10`, `dck-starter-feed-ratio`, `dck-temp-convert-probe` |
| daily / fitness (10) | `dft-acsm-incline-walk`, `dft-cadence-step-goal`, `dft-energy-balance-weeks`, `dft-ftp-interval-work`, `dft-karvonen-zones`, `dft-lean-mass-target`, `dft-pace-from-5k`, `dft-plank-compound-weeks`, `dft-swim-set-totals`, `dft-tenk-decimal-pace` |
| daily / gardening (10) | `dgd-bed-plant-spacing`, `dgd-compost-cn-alligation`, `dgd-fertiliser-season-bags`, `dgd-hedge-compound-prune`, `dgd-irrigation-zone-cycle`, `dgd-mower-throughput-overlap`, `dgd-mulch-volume-bags`, `dgd-rain-equivalent-watering`, `dgd-raised-bed-soil-bags`, `dgd-sowing-transplant-dates` |
| daily / home-repair (10) | `drp-bath-mix-fill-time`, `drp-concrete-post-hole-bags`, `drp-door-refix-timeline`, `drp-ladder-pythagoras-rule`, `drp-laminate-planks-with-waste`, `drp-led-swap-energy-savings`, `drp-paint-two-coat-cans`, `drp-shelf-bracket-fencepost`, `drp-thermostat-setback-savings`, `drp-tile-count-breakage` |
| daily / car (10) | `dcar-average-speed-restart`, `dcar-detailing-overlap`, `dcar-dilution-ratio`, `dcar-hire-mileage-charge`, `dcar-mpg-to-litres`, `dcar-parking-blocks`, `dcar-service-whichever-first`, `dcar-tank-range-reserve`, `dcar-tread-life`, `dcar-trip-fuel-cost` |
| daily / events (10) | `dev-catering-crate-wastage`, `dev-coaches-per-started`, `dev-deposit-after-discount`, `dev-flower-breakage-budget`, `dev-keg-pint-count`, `dev-place-card-sheets`, `dev-setup-working-days`, `dev-shift-block-overshoot`, `dev-table-grid-vs-area`, `dev-wedding-timeline` |
| daily / medication (10) | `dmed-antibiotic-course-volume`, `dmed-bioavailability-tablets`, `dmed-combo-analgesic-cap`, `dmed-insulin-daily-total`, `dmed-iv-drip-rate`, `dmed-pack-refill-count`, `dmed-suspension-dose-volume`, `dmed-tablet-cost-per-pack`, `dmed-unit-conversion-excretion`, `dmed-weight-based-dose` |
| daily / hiking (10) | `dhike-average-speed-halves`, `dhike-descent-time-allowance`, `dhike-drive-ferry-arrival`, `dhike-food-resupply-days`, `dhike-heat-water-fuel`, `dhike-map-scale-ground-time`, `dhike-naismith-summit-time`, `dhike-required-split-pace`, `dhike-tent-count-weight`, `dhike-water-gap-refill` |
| daily / energy (10) | `den-battery-arbitrage-losses`, `den-carbon-tier-bands`, `den-draught-payback-share`, `den-ev-overnight-charge`, `den-heat-pump-vs-gas-cost`, `den-load-profile-weighted-mean`, `den-oil-litres-efficiency`, `den-solar-self-consumption`, `den-standing-charge-days`, `den-tou-bill-supply-charge` |
| daily / childcare (10) | `dch-age-in-days`, `dch-daycare-funded-hours`, `dch-formula-annual-cost`, `dch-formula-can-count`, `dch-milk-bank-net-days`, `dch-night-sleep-minutes`, `dch-paracetamol-suspension-dose`, `dch-twin-feed-total`, `dch-volume-per-kg`, `dch-wake-window-clock` |
| daily / insurance (10) | `dins-annual-premium-discount`, `dins-contents-depreciation`, `dins-deductible-coinsurance`, `dins-excess-per-claim`, `dins-family-deductible-met`, `dins-income-protection-gap`, `dins-out-of-pocket-cap`, `dins-pet-annual-limit`, `dins-rental-damage-waiver`, `dins-waiting-period-date` |
| daily / nutrition (10) | `dnut-alcohol-units-abv`, `dnut-bmi-weight-band`, `dnut-caffeine-daily-cap`, `dnut-drink-sugar-averages`, `dnut-hydration-bottles`, `dnut-macro-energy-split`, `dnut-percent-dv-two-servings`, `dnut-protein-scoop-rounding`, `dnut-serving-scale-pack`, `dnut-tdee-deficit-target` |
| daily / pets (10) | `dpets-cat-calories`, `dpets-dog-food-percent`, `dpets-dog-walk-km`, `dpets-dog-water`, `dpets-flea-boxes`, `dpets-grooming-cycle`, `dpets-med-mg-kg`, `dpets-puppy-vaccines`, `dpets-rabbit-hay`, `dpets-tank-volume` |
| daily / commuting (10) | `dcomm-bus-interval`, `dcomm-carpool-rebate`, `dcomm-cycle-arrival`, `dcomm-fuel-share`, `dcomm-metro-platform`, `dcomm-office-cost`, `dcomm-parking-blocks`, `dcomm-rail-payg-vs-pass`, `dcomm-railcard-breakeven`, `dcomm-toll-vs-free` |
| daily / laundry (10) | `dlau-concentrate-dose`, `dlau-damp-dry-clock`, `dlau-delicates-loads`, `dlau-detergent-pricing`, `dlau-fleece-blanket`, `dlau-home-vs-laundrette`, `dlau-mixed-load-cycles`, `dlau-pod-schedule`, `dlau-sanitiser-dilution`, `dlau-washer-energy` |
| daily / woodwork (10) | `dwood-decking-boards`, `dwood-mitre-posts`, `dwood-mortar-waste`, `dwood-offcut-kerf`, `dwood-oil-coats`, `dwood-post-holes`, `dwood-price-break`, `dwood-screw-boxes`, `dwood-timber-invoice`, `dwood-varnish-tins` |
| daily / grow (10) | `dgrow-fert-tank`, `dgrow-germination`, `dgrow-heat-pump`, `dgrow-light-bill`, `dgrow-mix-ratio`, `dgrow-nutrient-dose`, `dgrow-reservoir-bottles`, `dgrow-seed-cost`, `dgrow-seed-trays`, `dgrow-watering-cans` |
| daily / home-maintenance (10) | `dhr-caulking-tubes`, `dhr-dryer-annual`, `dhr-filter-packs`, `dhr-insulation-payback`, `dhr-mower-runtime`, `dhr-pool-shock`, `dhr-render-time`, `dhr-service-plan`, `dhr-tank-ibc`, `dhr-tank-top-up` |
| daily / bathroom-plumbing (10) | `dbp-anti-slip-mats`, `dbp-bath-fill`, `dbp-descaling`, `dbp-drip-line`, `dbp-gutter-run`, `dbp-heater-energy`, `dbp-leak-waste`, `dbp-shower-annual`, `dbp-sump-pump`, `dbp-wall-tiles` |
| daily / cleaning-supplies (10) | `dcs-bin-liners`, `dcs-bleach-sprayers`, `dcs-cart-discount`, `dcs-cleaner-hours`, `dcs-detergent-packs`, `dcs-floor-polish`, `dcs-mop-buckets`, `dcs-tablets-vs-powder`, `dcs-vacuum-annual`, `dcs-window-sprayer` |
| daily / painting-decorating (10) | `dpaint-booth-energy`, `dpaint-crew-hours`, `dpaint-gloss-doors`, `dpaint-pot-sizes`, `dpaint-scaffold`, `dpaint-skirting`, `dpaint-sprayer-amort`, `dpaint-tint`, `dpaint-two-coats`, `dpaint-wallpaper` |
| daily / moving-house (10) | `dmov-box-packs`, `dmov-crate-trips`, `dmov-deposit-order`, `dmov-lift-tonnage`, `dmov-piano-surcharge`, `dmov-route-fuel`, `dmov-transit-insurance`, `dmov-two-quotes`, `dmov-utility-final`, `dmov-van-trips` |
| daily / diy-electrical (10) | `delec-breaker`, `delec-cable`, `delec-downlights`, `delec-duty-cycle`, `delec-festoon`, `delec-heat-mat`, `delec-kettle`, `delec-router-sla`, `delec-sleeves`, `delec-surge` |
| daily / waste-recycling (10) | `dwaste-bin-days`, `dwaste-bottles`, `dwaste-bulbs`, `dwaste-compost`, `dwaste-ewaste`, `dwaste-green-bags`, `dwaste-landfill`, `dwaste-oil`, `dwaste-shredder`, `dwaste-skips` |
| daily / diy-pest-control (10) | `dpest-ant-bait-mix`, `dpest-bait-stations`, `dpest-flea-fogger`, `dpest-mosquito-traps`, `dpest-pest-contract`, `dpest-rodent-blocks`, `dpest-snake-fence`, `dpest-spray-perimeter`, `dpest-termite-warranty`, `dpest-wasp-plan` |
| daily / home-safety (10) | `dsafe-cctv-archive`, `dsafe-extinguisher-service`, `dsafe-first-aid-stock`, `dsafe-handrail-fixings`, `dsafe-inspection-penalty`, `dsafe-ladder-hire`, `dsafe-monitoring-exit`, `dsafe-nightlight-annual`, `dsafe-smoke-batteries`, `dsafe-vent-fan` |
| daily / water-usage (10) | `dwat-bath-heat`, `dwat-filter-vs-jug`, `dwat-hose-vs-bucket`, `dwat-leak-trace`, `dwat-meter-bill`, `dwat-pool-fill`, `dwat-shower-vs-bath`, `dwat-softener-salt`, `dwat-sprinkler`, `dwat-tank-payback` |
| daily / home-security (10) | `dsec-doorbell-battery`, `dsec-dummy-camera`, `dsec-fake-presence`, `dsec-grille-bars`, `dsec-hasp-screws`, `dsec-monitoring-plan`, `dsec-motion-sensors`, `dsec-photocell`, `dsec-siren-cells`, `dsec-window-film` |
| daily / cooling-load (10) | `dcool-ac-size`, `dcool-car-ac`, `dcool-dehumidifier`, `dcool-evap-vs-ac`, `dcool-fan-energy`, `dcool-fan-vs-ac`, `dcool-filter-cleans`, `dcool-ice-blocks`, `dcool-thermostat`, `dcool-window-shading` |
| daily / telecom-plans (10) | `dtel-broadband-speed`, `dtel-contract-uplift`, `dtel-data-overage`, `dtel-family-plan`, `dtel-modem-power`, `dtel-payg-vs-bundle`, `dtel-prepaid-card`, `dtel-roaming-cap`, `dtel-sim-plan`, `dtel-tether-data` |
| daily / heating-load (10) | `dheat-boiler-kwh`, `dheat-cylinder`, `dheat-duty-cycle`, `dheat-heat-pump`, `dheat-insulation`, `dheat-oil-tank`, `dheat-radiator`, `dheat-tariff`, `dheat-thermostat`, `dheat-zoning` |
| daily / timesheet-pay (10) | `dpay-bonus-tax`, `dpay-clock-rounding`, `dpay-contractor-day`, `dpay-leave-accrual`, `dpay-mileage`, `dpay-overtime`, `dpay-parttime-prorata`, `dpay-salary-raise`, `dpay-shift-differential`, `dpay-tax-band` |
| daily / fuel-trip (10) | `dfuel-boat-range`, `dfuel-economy-mixed`, `dfuel-ev-charge`, `dfuel-flight-uplift`, `dfuel-generator-kwh`, `dfuel-hedge-mix`, `dfuel-jerrycan-trip`, `dfuel-lpg-payback`, `dfuel-moto-range`, `dfuel-truck-legs` |
| daily / pool-chemicals (10) | `dpool-algae-shock`, `dpool-backwash`, `dpool-floc`, `dpool-heat-pump`, `dpool-ph-adjust`, `dpool-ppm-dose`, `dpool-salt-cell`, `dpool-turnover`, `dpool-uv`, `dpool-volume-m3` |
| daily / garden-irrigation (10) | `dirr-drip-emitters`, `dirr-drip-flow`, `dirr-fert-spread`, `dirr-greenhouse-mist`, `dirr-hose-fill`, `dirr-pond-topup`, `dirr-soil-compost`, `dirr-sprinkler`, `dirr-timer-days`, `dirr-water-tank` |
| daily / aquarium-water (10) | `daqu-chiller`, `daqu-co2`, `daqu-filter-flow`, `daqu-lighting`, `daqu-medication`, `daqu-nitrification`, `daqu-potash`, `daqu-salt-mix`, `daqu-tank-litres`, `daqu-waterchange` |
| daily / ev-charging (10) | `devch-ac-session`, `devch-battery-value`, `devch-cable-loss`, `devch-dc-fast`, `devch-degradation`, `devch-fleet-balancer`, `devch-range-added`, `devch-solar-surplus`, `devch-toll-pass`, `devch-tou-window` |
| daily / tile-flooring (10) | `dtile-adhesive-coverage`, `dtile-border-run`, `dtile-budget`, `dtile-cut-waste`, `dtile-drain-slope`, `dtile-grid-count`, `dtile-grout-volume`, `dtile-levelling`, `dtile-room-area`, `dtile-skirting-cost` |
| daily / decking-fencing (10) | `ddeck-board-area`, `ddeck-concrete-footings`, `ddeck-fence-panel-run`, `ddeck-gate-hinges`, `ddeck-joist-spacing`, `ddeck-oil-restoration`, `ddeck-railing-load`, `ddeck-slope-steps`, `ddeck-stair-stringer`, `ddeck-subframe-clamps` |
| daily / loft-insulation (10) | `dloft-blown-depth`, `dloft-cistern-insul`, `dloft-downlight-covers`, `dloft-hatch-draft`, `dloft-heat-loss`, `dloft-knee-wall`, `dloft-pipe-jacket`, `dloft-spray-foam`, `dloft-target-u`, `dloft-vapour-barrier` |
| daily / coffee-brewing (10) | `dbrew-caffeine-budget`, `dbrew-cold-brew-yield`, `dbrew-cup-cost`, `dbrew-descale-tablets`, `dbrew-espresso-shots`, `dbrew-extraction-yield`, `dbrew-french-press`, `dbrew-grind-throughput`, `dbrew-mineral-dose`, `dbrew-syrup-ratio` |
| daily / laundry-care (10) | `dlc-bleach-dilution`, `dlc-detergent-cost`, `dlc-drum-load`, `dlc-dryclean-vs-home`, `dlc-dryer-energy`, `dlc-load-split`, `dlc-softener-vs-vinegar`, `dlc-spin-extraction`, `dlc-stain-soak-supply`, `dlc-water-use` |
| daily / bike-care (10) | `dbike-battery-range`, `dbike-brake-pad-cost`, `dbike-chain-lifetime`, `dbike-chain-wear`, `dbike-energy-gels`, `dbike-gear-development`, `dbike-hire-mileage`, `dbike-patch-box`, `dbike-tuneup-vs-parts`, `dbike-tyre-tread` |
| daily / houseplant-care (10) | `dhpl-cutting-trays`, `dhpl-fertilizer-bottle`, `dhpl-foot-candle-convert`, `dhpl-grow-light-bill`, `dhpl-neem-spray-rounds`, `dhpl-repot-soil-volume`, `dhpl-reservoir-top-up`, `dhpl-shelf-bundle-deal`, `dhpl-soil-mix-split`, `dhpl-watering-schedule` |
| daily / lawn-care (10) | `dlawn-clippings`, `dlawn-fence-posts`, `dlawn-fertilizer-season`, `dlawn-irrigation`, `dlawn-mowing-time`, `dlawn-reseed-bags`, `dlawn-robot-wire`, `dlawn-spreader-fills`, `dlawn-topdress`, `dlawn-two-stroke` |
| daily / data-backup (10) | `dbak-321-plan`, `dbak-archive-drives`, `dbak-cloud-bill`, `dbak-lto-tapes`, `dbak-raid-usable`, `dbak-ransomware-anomaly`, `dbak-restore-time`, `dbak-snapshot-retention`, `dbak-upload-throughput`, `dbak-upload-window` |
| daily / pet-nutrition (10) | `dpn-bag-days`, `dpn-cat-water`, `dpn-joint-jar`, `dpn-kibble-cups`, `dpn-multi-cat-bags`, `dpn-puppy-meals`, `dpn-raw-mix`, `dpn-split-wet`, `dpn-treat-budget`, `dpn-wet-can-boxes` |
| daily / bread-baking (10) | `dbake-annual-flour`, `dbake-hydration`, `dbake-pan-volume`, `dbake-proof-rate`, `dbake-roll-division`, `dbake-scale`, `dbake-schedule`, `dbake-starter-hydration`, `dbake-water-temp`, `dbake-yeast-convert` |
| daily / sewing-tailoring (10) | `dsew-batting`, `dsew-bias-binding`, `dsew-button-spacing`, `dsew-elastic`, `dsew-fusible`, `dsew-layout`, `dsew-seam-allowance`, `dsew-shrinkage`, `dsew-thread-cone`, `dsew-zip-roll` |
| daily / bbq-grilling (10) | `dbbq-brine`, `dbbq-burger-packs`, `dbbq-coal-indirect`, `dbbq-low-slow`, `dbbq-propane`, `dbbq-rub-batch`, `dbbq-sauce-bottles`, `dbbq-service-queue`, `dbbq-thaw-schedule`, `dbbq-veg-trays` |
| daily / candle-making (10) | `dcand-batch-cost`, `dcand-blend-ratio`, `dcand-double-pour`, `dcand-jar-fill`, `dcand-market-margin`, `dcand-pool-ring`, `dcand-shared-batch`, `dcand-timeline`, `dcand-topup-bags`, `dcand-wick-spools` |
| daily / freshwater-fishkeeping (10) | `dfish-boat-current`, `dfish-fillet-yield`, `dfish-fry-hatch`, `dfish-grow-out`, `dfish-koi-ration`, `dfish-net-coverage`, `dfish-pond-liner`, `dfish-pond-volume`, `dfish-transport-crates`, `dfish-worm-harvest` |
| daily / home-canning (10) | `dcann-batch-cost`, `dcann-blanch-altitude`, `dcann-hard-boil`, `dcann-jam-pectin`, `dcann-jam-yield`, `dcann-kraut-salt`, `dcann-lid-packs`, `dcann-pickle-brine`, `dcann-pressure-session`, `dcann-sugar-per-jar` |
| daily / beekeeping (10) | `dbee-extraction-yield`, `dbee-fence-perimeter`, `dbee-hive-gain`, `dbee-mite-wash`, `dbee-nuc-split`, `dbee-queen-graft`, `dbee-season-revenue`, `dbee-super-fill`, `dbee-syrup-mix`, `dbee-winter-feed` |
| daily / knitting (10) | `dknit-blanket-squares`, `dknit-dye-wool`, `dknit-gauge-cast-on`, `dknit-hat-decrease`, `dknit-mitten-commission`, `dknit-rib-cuff`, `dknit-scarf-time`, `dknit-sock-pairs`, `dknit-sweater-yardage`, `dknit-yarn-skeins` |
| daily / soap-making (10) | `dsoap-curing-sales`, `dsoap-fragrance-load`, `dsoap-loaf-cut`, `dsoap-lye-bags`, `dsoap-lye-solution`, `dsoap-lye-strength`, `dsoap-market-profit`, `dsoap-mold-loaves`, `dsoap-oil-blend`, `dsoap-superfat` |
| daily / pottery-ceramics (10) | dpot-clay-bags`, `dpot-commission-discount`, `dpot-course-profit`, `dpot-drying-rate`, `dpot-glaze-batch`, `dpot-grog-share`, `dpot-kiln-energy`, `dpot-mug-margin`, `dpot-shrinkage`, `dpot-throwing-shift |
| daily / cheese-making (10) | dcheese-affinage-profit`, `dcheese-brine-saturation`, `dcheese-calcium-dose`, `dcheese-drying-loft`, `dcheese-milk-yield`, `dcheese-mother-stir`, `dcheese-mozzarella-sales`, `dcheese-press-force`, `dcheese-press-schedule`, `dcheese-rennet-dose |
| daily / home-brewing (10) | `dbeer-abv`, `dbeer-boil-energy`, `dbeer-chiller-time`, `dbeer-efficiency`, `dbeer-growler-margin`, `dbeer-hops-ibu`, `dbeer-mash-sparge`, `dbeer-priming-sugar`, `dbeer-strike-temp`, `dbeer-yeast-pitch` |
| daily / embroidery (10) | `demb-class-net`, `demb-floss-length`, `demb-hoop-area`, `demb-kit-profit`, `demb-pattern-scale`, `demb-quilt-edge`, `demb-ribbon-round`, `demb-runner-fabric`, `demb-stitch-time`, `demb-tassel-cut` |
| daily / jewelry-making (10) | `djew-bead-strand`, `djew-casting-sprue`, `djew-chain-price`, `djew-clay-shrink`, `djew-earring-wire`, `djew-jump-ring`, `djew-pendant-weight`, `djew-ring-blank`, `djew-wire-gauge`, `djew-wire-loop` |
| daily / snow-removal (10) | `dsnow-brine-mix`, `dsnow-ice-melt`, `dsnow-job-price`, `dsnow-plow-shifts`, `dsnow-roof-load`, `dsnow-salt-driveway`, `dsnow-salting-rounds`, `dsnow-sand-salt-stock`, `dsnow-shovel-rate`, `dsnow-snow-haul` |
| daily / camping (10) | `dcamp-tent-capacity`, `dcamp-pack-weight`, `dcamp-water-tablets`, `dcamp-gas-canister`, `dcamp-cooler-ice`, `dcamp-clothesline`, `dcamp-rope-load`, `dcamp-headlamp-battery`, `dcamp-firewood-split`, `dcamp-fee-per-night` |
| daily / photography (10) | `dphot-exposure-stops`, `dphot-hyperfocal`, `dphot-memory-card`, `dphot-golden-hour`, `dphot-print-size`, `dphot-film-contact`, `dphot-sandbag-packs`, `dphot-stock-royalty`, `dphot-lens-rental`, `dphot-timelapse-clock` |
| daily / records (10) | `dvrc-shipping-boxes`, `dvrc-sleeve-packs`, `dvrc-bulk-price`, `dvrc-play-hours`, `dvrc-flip-clock`, `dvrc-crate-load`, `dvrc-clean-fluid`, `dvrc-grading-score`, `dvrc-belt-life`, `dvrc-shelf-space` |
| daily / skiing (10) | `dski-lift-pass`, `dski-slope-grade`, `dski-wax-season`, `dski-lift-queue`, `dski-snowmaking`, `dski-boot-shell`, `dski-storage-fee`, `dski-kids-camp`, `dski-gear-value`, `dski-club-minibus` |
| daily / vanlife (10) | `dvlf-solar-array`, `dvlf-water-tank`, `dvlf-fuel-budget`, `dvlf-propane`, `dvlf-mileage`, `dvlf-plywood-cuts`, `dvlf-grey-water`, `dvlf-fridge-amps`, `dvlf-drive-time`, `dvlf-campsite-bill` |
| daily / arcade (10) | `darc-token-packs`, `darc-claw-net`, `darc-hoops-tickets`, `darc-dance-session`, `darc-hockey-league`, `darc-redemption-hours`, `darc-pinball-games`, `darc-ticket-dispenser`, `darc-photo-booth`, `darc-prize-wall` |
| daily / 3d-printing (10) | `d3dp-cost-per-print`, `d3dp-print-time`, `d3dp-bed-layout`, `d3dp-resin-batch`, `d3dp-spool-swaps`, `d3dp-support-share`, `d3dp-post-cure`, `d3dp-flow-limit`, `d3dp-power-bill`, `d3dp-stock-order` |
| daily / car-wash (10) | `dcw-foam-dilution`, `dcw-bay-throughput`, `dcw-water-reclaim`, `dcw-package-margin`, `dcw-clay-bars`, `dcw-vac-bags`, `dcw-tyre-dressing`, `dcw-queue-length`, `dcw-wash-plan`, `dcw-tip-pool` |

## Adding a test

1. Add a `testkit.Test` value to the relevant file in `internal/tests` (or a
   new file, for a new category). Give it a unique, kebab-case `ID`, a
   `Category` and `Subcategory`, a self-contained `Prompt`, and an
   `eval.Evaluator`.
2. Register it in that file's `register<Category>Tests` function, called
   from `internal/tests/catalog.go`'s `All`.
3. Add a `// ground truth:` comment above any hardcoded expected value,
   showing how you derived it. Recompute it in the test file when that is
   cheap (arithmetic, a struct's `unsafe.Sizeof`, running the traced
   function).
4. Add table-driven `_test.go` cases covering a correct answer, a wrong
   answer, and at least one edge case (a common trap, a partial match, or a
   malformed response).
5. If you introduced a new subcategory, add it to `wantSubcats` in
   `internal/tests/catalog_test.go`; every subcategory needs at least ten
   tests, so a one-test subcategory will not pass the catalog gate.

## Adding an evaluator

Evaluators live in `internal/eval` and implement:

```go
type Evaluator interface {
    Evaluate(ctx context.Context, response string) Score
}
```

`Score.Value` is in the range 0 to 1; set `Score.Skipped` only when a
required toolchain is missing, not for a wrong answer. Compose existing
evaluators with `eval.All` (weighted mean) or `eval.Mean` (equal-weighted
mean) rather than writing a one-off composite by hand. For a check that the
built-in evaluators cannot express, wrap a closure with
`eval.EvaluatorFunc`. Add table-driven tests in `internal/eval` covering a
match, a non-match, and an edge case (empty input, malformed JSON, and so
on).

## Quality gates

```sh
go build ./...
go vet ./...
go test ./...
gofmt -l .              # must print nothing
golangci-lint run       # if the binary is installed
```

Every unit test runs offline against a mock `llm.Client` or a local
`httptest.Server`; none of them call the live endpoint. The exec-evaluator
tests (`GoRun`, `PyRun`, `CRun`) detect a missing `go`, `python3`, or `cc`
toolchain and skip rather than fail.
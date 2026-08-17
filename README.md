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
- `--concurrency` - override the config file's `concurrency`.
- `--timeout` - override the config file's `request_timeout` (for example
  `60s`).
- `--verbose` - print one progress line per completed test to stderr.

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

## Test catalog

The catalog lives in `internal/tests`, one file per category. Every test
has a fully self-contained prompt (all data needed to answer is inline) and
a single deterministic evaluator. Every hardcoded expected value carries a
`// ground truth:` comment showing its derivation, and where the derivation
is cheap, the test file's `_test.go` recomputes it independently (for
example, `internal/tests/python_test.go` recomputes the cosine similarity
with `math.Sqrt` rather than trusting the constant).

| Category / subcategory | Tests |
| --- | --- |
| programming / golang | `go-struct-align`, `go-worker-pool`, `go-semver-classify` |
| programming / python | `py-log-triage`, `py-cosine` |
| programming / typescript | `ts-debounce-composable` |
| programming / c | `c-struct-size` |
| operations / macos | `macos-timeout-portability`, `macos-launchd-cron` |
| operations / linux | `linux-pct-exec`, `linux-systemd-oneshot` |
| operations / kubernetes | `k8s-crashloop-gitops` |
| research / web | `web-robots-ai-crawlers` |
| research / whitepapers | `paper-hnsw-params` |
| research / codebase | `code-trace-go` |
| agents / tool-routing | `agent-tool-routing` |
| agents / planning | `agent-plan-ordering` |

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
5. Add the test's ID to the `wantCatalog` map in
   `internal/tests/catalog_test.go`.

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

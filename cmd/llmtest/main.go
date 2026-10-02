// Command llmtest runs the llm-testbench catalog against one or more
// OpenAI-compatible models and reports deterministic scores.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/lukaszraczylo/llm-testbench/internal/config"
	"github.com/lukaszraczylo/llm-testbench/internal/llm"
	"github.com/lukaszraczylo/llm-testbench/internal/report"
	"github.com/lukaszraczylo/llm-testbench/internal/runner"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
	"github.com/lukaszraczylo/llm-testbench/internal/tests"
)

// requestTemperature is pinned to 0 for deterministic scoring, per PLAN.md;
// it is not exposed as a flag.
const requestTemperature = 0.0

// version is stamped by the release build via
// -ldflags "-X main.version=vX.Y.Z"; a source build reports "dev".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "run":
		err = runCommand(os.Args[2:])
	case "list":
		err = listCommand(os.Args[2:])
	case "compare":
		err = compareCommand(os.Args[2:])
	case "health":
		err = healthCommand(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println("llmtest " + version)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "llmtest: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "llmtest: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `llmtest - LLM accuracy testing framework

Usage:
  llmtest run     [flags]   Run the test catalog against configured models.
  llmtest list    [flags]   List the test catalog.
  llmtest compare baseline.json current.json
                            Diff two saved run artifacts (--out files).
  llmtest health  file.json [more.json ...]
                            Audit suite health: which subcategories are
                            saturated (every model passes everything), and
                            which tests still carry signal. Pools multiple
                            artifacts.
  llmtest version           Print the build version.

Run "llmtest run -h" or "llmtest list -h" for flag details.
`)
}

// sharedFlags are accepted by both run and list to filter the catalog.
type sharedFlags struct {
	configPath  string
	category    string
	subcategory string
}

func bindSharedFlags(fs *flag.FlagSet) *sharedFlags {
	f := &sharedFlags{}
	fs.StringVar(&f.configPath, "config", "config.yaml", "path to config.yaml")
	fs.StringVar(&f.category, "category", "", "filter tests by category")
	fs.StringVar(&f.subcategory, "subcategory", "", "filter tests by subcategory")
	return f
}

func listCommand(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	shared := bindSharedFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	registry := tests.All()
	filtered := registry.Filter(shared.category, shared.subcategory)
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID < filtered[j].ID })

	for _, t := range filtered {
		fmt.Printf("%-28s %-14s %-14s %s\n", t.ID, t.Category, t.Subcategory, t.Description)
	}
	return nil
}

func runCommand(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	shared := bindSharedFlags(fs)
	modelsCSV := fs.String("models", "", "comma-separated model override (defaults to config's models)")
	testsCSV := fs.String("tests", "", "comma-separated exact test IDs (overrides --category/--subcategory)")
	format := fs.String("format", "table", "output format: table|markdown|json")
	concurrency := fs.Int("concurrency", 0, "override config's concurrency (0 = use config)")
	timeout := fs.Duration("timeout", 0, "override config's request_timeout (0 = use config)")
	repeat := fs.Int("repeat", 1, "samples per (model, test); >1 exposes response instability at temperature 0")
	out := fs.String("out", "", "write the run as a JSON artifact to this file (for llmtest compare)")
	quiet := fs.Bool("quiet", false, "suppress progress output on stderr")
	seedFlag := fs.Int("seed", 0, "send this seed with every request (overrides config's seed)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(shared.configPath)
	if err != nil {
		return err
	}

	models := cfg.Models
	if *modelsCSV != "" {
		models = dedupe(splitCSV(*modelsCSV))
	}

	reqTimeout := cfg.RequestTimeout
	if *timeout > 0 {
		reqTimeout = *timeout
	}

	conc := cfg.Concurrency
	if *concurrency > 0 {
		conc = *concurrency
	}

	seed := cfg.Seed
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "seed" {
			seed = seedFlag
		}
	})

	client := llm.NewOpenAIClient(cfg.Endpoint, cfg.APIKey, reqTimeout).
		WithRetries(cfg.MaxRetries).
		WithRetryTimeouts(cfg.RetryTimeouts).
		WithModelTimeouts(cfg.ModelTimeouts).
		WithStreaming(cfg.Stream, cfg.IdleTimeout)

	registry := tests.All()
	selected, err := selectTests(registry, *testsCSV, shared.category, shared.subcategory)
	if err != nil {
		return err
	}

	var reporter runner.ProgressReporter = runner.NoopProgressReporter{}
	if !*quiet {
		reporter = newStderrProgressReporter(os.Stderr)
	}

	r := runner.New(client, runner.Config{
		Concurrency:      conc,
		Temperature:      requestTemperature,
		MaxTokensDefault: cfg.MaxTokensDefault,
		Reporter:         reporter,
		Repeat:           *repeat,
		Seed:             seed,
		ModelConcurrency: cfg.ModelConcurrency,
	})

	// First Ctrl-C stops dispatching and lets in-flight calls drain so the
	// partial run can still be saved; stop() restores default handling, so
	// a second Ctrl-C kills the process.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startedAt := time.Now().UTC()
	results := r.Run(ctx, models, selected)
	interrupted := ctx.Err() != nil
	if interrupted {
		stop()
		results = dropCancelled(results)
		fmt.Fprintf(os.Stderr, "llmtest: interrupted; reporting %d completed results\n", len(results))
	}

	f, err := validateFormat(*format)
	if err != nil {
		return err
	}

	if *out != "" {
		meta := report.RunMeta{
			Version:          version,
			StartedAt:        startedAt.Format(time.RFC3339),
			Endpoint:         cfg.Endpoint,
			Seed:             seed,
			Temperature:      requestTemperature,
			MaxTokensDefault: cfg.MaxTokensDefault,
			Repeat:           max(*repeat, 1),
			Interrupted:      interrupted,
		}
		if err := report.WriteArtifact(*out, meta, selected, models, results); err != nil {
			return fmt.Errorf("write --out: %w", err)
		}
	}

	return report.Render(os.Stdout, f, selected, models, results)
}

// dropCancelled removes results that never ran because the run was
// interrupted, so they are not saved as failures.
func dropCancelled(results []runner.Result) []runner.Result {
	kept := results[:0:0]
	for _, r := range results {
		if r.Err != nil && errors.Is(r.Err, context.Canceled) {
			continue
		}
		kept = append(kept, r)
	}
	return kept
}

// selectTests resolves the test subset: --tests (exact IDs, error on any
// unknown ID) overrides --category/--subcategory filtering.
func selectTests(registry *testkit.Registry, testsCSV, category, subcategory string) ([]testkit.Test, error) {
	if testsCSV == "" {
		selected := registry.Filter(category, subcategory)
		if len(selected) == 0 {
			return nil, fmt.Errorf("no tests matched category=%q subcategory=%q", category, subcategory)
		}
		return selected, nil
	}

	ids := splitCSV(testsCSV)
	out := make([]testkit.Test, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		t, ok := registry.Get(id)
		if !ok {
			return nil, fmt.Errorf("unknown test id %q (see llmtest list)", id)
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--tests parsed to zero ids")
	}
	return out, nil
}

// compareCommand diffs two saved artifacts from `llmtest run --out`.
func compareCommand(args []string) error {
	fs := flag.NewFlagSet("compare", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("usage: llmtest compare baseline.json current.json")
	}

	baseline, err := report.LoadArtifact(fs.Arg(0))
	if err != nil {
		return err
	}

	current, err := report.LoadArtifact(fs.Arg(1))
	if err != nil {
		return err
	}

	return report.RenderCompare(os.Stdout, report.CompareArtifacts(baseline, current), report.CompareWarnings(baseline, current)...)
}

// healthCommand audits one or more saved artifacts for suite health:
// per-subcategory saturation and the tests that still discriminate.
func healthCommand(args []string) error {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: llmtest health artifact.json [more.json ...]")
	}
	artifacts := make([]report.Artifact, 0, fs.NArg())
	for _, p := range fs.Args() {
		a, err := report.LoadArtifact(p)
		if err != nil {
			return fmt.Errorf("load %s: %w", p, err)
		}
		artifacts = append(artifacts, a)
	}
	return report.RenderHealth(os.Stdout, report.AuditHealth(artifacts))
}

// validateFormat parses and validates the --format flag value, pulled out
// of runCommand as its own function so it is unit-testable without going
// through flag parsing / a live run (N8).
func validateFormat(format string) (report.Format, error) {
	f := report.Format(format)
	switch f {
	case report.FormatTable, report.FormatMarkdown, report.FormatJSON:
		return f, nil
	default:
		return "", fmt.Errorf("unknown format %q (want table|markdown|json)", format)
	}
}

// dedupe drops repeated entries, keeping first-seen order; a model listed
// twice would otherwise run, and be averaged, twice.
func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

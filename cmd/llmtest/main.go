// Command llmtest runs the llm-testbench catalog against one or more
// OpenAI-compatible models and reports deterministic scores.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/config"
	"github.com/lukaszraczylo/llm-testbench/internal/llm"
	"github.com/lukaszraczylo/llm-testbench/internal/report"
	"github.com/lukaszraczylo/llm-testbench/internal/runner"
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
	case "version", "-v", "--version":
		fmt.Println("llmtest " + version)
		return
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
  llmtest run  [flags]   Run the test catalog against configured models.
  llmtest list [flags]   List the test catalog.
  llmtest version        Print the build version.

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
	format := fs.String("format", "table", "output format: table|markdown|json")
	concurrency := fs.Int("concurrency", 0, "override config's concurrency (0 = use config)")
	timeout := fs.Duration("timeout", 0, "override config's request_timeout (0 = use config)")
	quiet := fs.Bool("quiet", false, "suppress progress output on stderr")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(shared.configPath)
	if err != nil {
		return err
	}

	models := cfg.Models
	if *modelsCSV != "" {
		models = splitCSV(*modelsCSV)
	}

	reqTimeout := cfg.RequestTimeout
	if *timeout > 0 {
		reqTimeout = *timeout
	}

	conc := cfg.Concurrency
	if *concurrency > 0 {
		conc = *concurrency
	}

	client := llm.NewOpenAIClient(cfg.Endpoint, cfg.APIKey, reqTimeout)

	registry := tests.All()
	selected := registry.Filter(shared.category, shared.subcategory)
	if len(selected) == 0 {
		return fmt.Errorf("no tests matched category=%q subcategory=%q", shared.category, shared.subcategory)
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
	})

	ctx := context.Background()
	results := r.Run(ctx, models, selected)

	f, err := validateFormat(*format)
	if err != nil {
		return err
	}

	return report.Render(os.Stdout, f, selected, models, results)
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

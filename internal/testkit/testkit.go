// Package testkit defines the Test type shared by the catalog in
// internal/tests, a Registry to collect them, and response normalization
// applied before evaluation.
package testkit

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/llm"
)

// Test is one catalog entry: a fully self-contained prompt scored by a
// deterministic Evaluator.
//
// When Tools is non-empty, the test exercises function-calling: the runner
// advertises the tools (tool_choice auto), then serializes the model's tool
// calls into the canonical JSON envelope the eval.ToolCalled/
// ToolCallWithArgs/ToolSequence/NoToolCalled evaluators parse. The Eval for
// such a test scores that envelope, not the model's free-text answer.
type Test struct {
	Eval        eval.Evaluator
	ID          string
	Category    string
	Subcategory string
	Description string
	System      string
	Prompt      string
	Tools       []llm.Tool
	MaxTokens   int
}

// thinkBlockPattern strips a leading <think>...</think> or
// <reasoning>...</reasoning> block, case-insensitive, before the real
// answer. Models observed on this endpoint prepend one.
var thinkBlockPattern = regexp.MustCompile(`(?is)^\s*<(think|reasoning)>.*?</(think|reasoning)>\s*`)

// thinkCloseTagPattern and thinkOpenTagPattern locate a closing/opening
// <think>/<reasoning> tag anywhere in the text, used to detect an orphan
// closing tag (S7): some models on this endpoint truncate the opening tag
// (e.g. it falls outside a context/streaming window) but still emit the
// closing tag, leaving stray reasoning text with no opener in front of it.
var thinkCloseTagPattern = regexp.MustCompile(`(?i)</(?:think|reasoning)>`)
var thinkOpenTagPattern = regexp.MustCompile(`(?i)<(?:think|reasoning)>`)

// stripOrphanThinkClose removes everything up to and including a closing
// </think>/</reasoning> tag that has no matching opening tag earlier in s.
// A tag pair with a real opener is left alone (thinkBlockPattern already
// handles the well-formed, anchored-at-start case).
func stripOrphanThinkClose(s string) string {
	closeLoc := thinkCloseTagPattern.FindStringIndex(s)
	if closeLoc == nil {
		return s
	}
	if thinkOpenTagPattern.MatchString(s[:closeLoc[0]]) {
		return s
	}
	return s[closeLoc[1]:]
}

// Normalize prepares a raw model response for evaluation: it strips a
// leading <think>/<reasoning> block (or, failing that, an orphan closing
// tag with no opener), collapses Windows line endings, and trims
// surrounding whitespace.
func Normalize(response string) string {
	s := thinkBlockPattern.ReplaceAllString(response, "")
	s = stripOrphanThinkClose(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimSpace(s)
}

// Registry collects Tests and provides lookup/filtering for the runner and
// CLI.
type Registry struct {
	byID  map[string]Test
	tests []Test
}

// NewRegistry builds an empty Registry.
func NewRegistry() *Registry {
	return &Registry{byID: make(map[string]Test)}
}

// Register adds t to the registry. It panics on a duplicate ID since the
// catalog is assembled once at program startup via init()-time registration,
// making a duplicate a programming error, not a runtime condition.
func (r *Registry) Register(t Test) {
	if t.ID == "" {
		panic("testkit: Test.ID must not be empty")
	}
	if _, exists := r.byID[t.ID]; exists {
		panic(fmt.Sprintf("testkit: duplicate test ID %q", t.ID))
	}
	if t.Eval == nil {
		panic(fmt.Sprintf("testkit: test %q has a nil Evaluator", t.ID))
	}
	r.tests = append(r.tests, t)
	r.byID[t.ID] = t
}

// All returns every registered test, sorted by ID for deterministic output.
func (r *Registry) All() []Test {
	out := make([]Test, len(r.tests))
	copy(out, r.tests)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Filter returns registered tests whose Category/Subcategory match the
// given filters. Empty filters match everything.
func (r *Registry) Filter(category, subcategory string) []Test {
	all := r.All()
	if category == "" && subcategory == "" {
		return all
	}
	var out []Test
	for _, t := range all {
		if category != "" && !strings.EqualFold(t.Category, category) {
			continue
		}
		if subcategory != "" && !strings.EqualFold(t.Subcategory, subcategory) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// Get returns the test with the given ID.
func (r *Registry) Get(id string) (Test, bool) {
	t, ok := r.byID[id]
	return t, ok
}

// Len returns the number of registered tests.
func (r *Registry) Len() int {
	return len(r.tests)
}

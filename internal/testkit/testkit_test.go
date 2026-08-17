package testkit

import (
	"context"
	"testing"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"trims whitespace", "  hello  ", "hello"},
		{"leading whitespace before answer", "\n\nOK", "OK"},
		{"strips think block", "<think>reasoning here</think>OK", "OK"},
		{"strips think block with whitespace after", "<think>reasoning here</think>\n\nOK", "OK"},
		{"strips think block case insensitive", "<THINK>x</THINK>OK", "OK"},
		{"strips reasoning block", "<reasoning>plan</reasoning>final answer", "final answer"},
		{"collapses windows line endings", "line1\r\nline2", "line1\nline2"},
		{"multiline think block", "<think>\nstep 1\nstep 2\n</think>\nanswer", "answer"},
		{"no think block passthrough", "just a plain answer", "just a plain answer"},
		{"think tag not at start is left alone", "prefix <think>x</think> suffix", "prefix <think>x</think> suffix"},
		// S7: an orphan closing tag (opening tag truncated by the gateway)
		// must still be stripped.
		{"orphan think close tag with no opener", "leftover reasoning residue</think>OK", "OK"},
		{"orphan reasoning close tag with no opener", "leftover plan</reasoning>final answer", "final answer"},
		{"orphan close tag case insensitive", "residue</THINK>OK", "OK"},
		{"orphan close tag with whitespace after", "residue</think>\n\nOK", "OK"},
		{"no close tag at all passthrough", "just a plain answer with no tags", "just a plain answer with no tags"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.in)
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func newDummyEval(value float64) eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, _ string) eval.Score {
		return eval.Score{Value: value}
	})
}

func TestRegistry_RegisterAndAll(t *testing.T) {
	r := NewRegistry()
	r.Register(Test{ID: "b-test", Category: "cat", Subcategory: "sub", Prompt: "p", Eval: newDummyEval(1)})
	r.Register(Test{ID: "a-test", Category: "cat", Subcategory: "sub", Prompt: "p", Eval: newDummyEval(1)})

	all := r.All()
	if len(all) != 2 {
		t.Fatalf("All() len = %d, want 2", len(all))
	}
	if all[0].ID != "a-test" || all[1].ID != "b-test" {
		t.Errorf("All() not sorted by ID: got %q, %q", all[0].ID, all[1].ID)
	}
}

func TestRegistry_RegisterDuplicateIDPanics(t *testing.T) {
	r := NewRegistry()
	r.Register(Test{ID: "dup", Category: "c", Prompt: "p", Eval: newDummyEval(1)})

	defer func() {
		if recover() == nil {
			t.Fatal("Register() with duplicate ID did not panic")
		}
	}()
	r.Register(Test{ID: "dup", Category: "c", Prompt: "p", Eval: newDummyEval(1)})
}

func TestRegistry_RegisterEmptyIDPanics(t *testing.T) {
	r := NewRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("Register() with empty ID did not panic")
		}
	}()
	r.Register(Test{Category: "c", Prompt: "p", Eval: newDummyEval(1)})
}

func TestRegistry_RegisterNilEvaluatorPanics(t *testing.T) {
	r := NewRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("Register() with nil Evaluator did not panic")
		}
	}()
	r.Register(Test{ID: "x", Category: "c", Prompt: "p"})
}

func TestRegistry_Filter(t *testing.T) {
	r := NewRegistry()
	r.Register(Test{ID: "go-1", Category: "programming", Subcategory: "golang", Prompt: "p", Eval: newDummyEval(1)})
	r.Register(Test{ID: "py-1", Category: "programming", Subcategory: "python", Prompt: "p", Eval: newDummyEval(1)})
	r.Register(Test{ID: "k8s-1", Category: "operations", Subcategory: "kubernetes", Prompt: "p", Eval: newDummyEval(1)})

	tests := []struct {
		name        string
		category    string
		subcategory string
		wantIDs     []string
	}{
		{"no filter", "", "", []string{"go-1", "k8s-1", "py-1"}},
		{"by category", "programming", "", []string{"go-1", "py-1"}},
		{"by category case insensitive", "PROGRAMMING", "", []string{"go-1", "py-1"}},
		{"by subcategory", "", "golang", []string{"go-1"}},
		{"by category and subcategory", "programming", "python", []string{"py-1"}},
		{"no match", "nonexistent", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Filter(tt.category, tt.subcategory)
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("Filter(%q, %q) len = %d, want %d", tt.category, tt.subcategory, len(got), len(tt.wantIDs))
			}
			for i, want := range tt.wantIDs {
				if got[i].ID != want {
					t.Errorf("Filter(%q, %q)[%d].ID = %q, want %q", tt.category, tt.subcategory, i, got[i].ID, want)
				}
			}
		})
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()
	r.Register(Test{ID: "x", Category: "c", Prompt: "p", Eval: newDummyEval(1)})

	got, ok := r.Get("x")
	if !ok || got.ID != "x" {
		t.Errorf("Get(%q) = %+v, %v, want ID x, true", "x", got, ok)
	}
	_, ok = r.Get("missing")
	if ok {
		t.Error("Get() for missing ID returned ok=true")
	}
}

func TestRegistry_Len(t *testing.T) {
	r := NewRegistry()
	if r.Len() != 0 {
		t.Errorf("Len() = %d, want 0", r.Len())
	}
	r.Register(Test{ID: "x", Category: "c", Prompt: "p", Eval: newDummyEval(1)})
	if r.Len() != 1 {
		t.Errorf("Len() = %d, want 1", r.Len())
	}
}

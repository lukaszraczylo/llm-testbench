package main

import (
	"strings"
	"testing"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/report"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"single model", "uni/deepseek-v4-flash-0731", []string{"uni/deepseek-v4-flash-0731"}},
		{"multiple models", "a,b,c", []string{"a", "b", "c"}},
		{"trims whitespace around entries", " a , b ,c ", []string{"a", "b", "c"}},
		{"drops empty entries from trailing comma", "a,b,", []string{"a", "b"}},
		{"drops empty entries from doubled comma", "a,,b", []string{"a", "b"}},
		{"empty string yields no entries", "", nil},
		{"whitespace only yields no entries", "   ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitCSV(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("splitCSV(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitCSV(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestValidateFormat(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    report.Format
		wantErr bool
	}{
		{"table", "table", report.FormatTable, false},
		{"markdown", "markdown", report.FormatMarkdown, false},
		{"json", "json", report.FormatJSON, false},
		{"unknown format", "yaml", "", true},
		{"empty format", "", "", true},
		{"case sensitive: uppercase rejected", "TABLE", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateFormat(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateFormat(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("validateFormat(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSelectTests(t *testing.T) {
	reg := testkit.NewRegistry()
	reg.Register(testkit.Test{ID: "a", Category: "x", Subcategory: "s", Eval: eval.ContainsAny("a")})
	reg.Register(testkit.Test{ID: "b", Category: "y", Subcategory: "s", Eval: eval.ContainsAny("b")})

	got, err := selectTests(reg, "b,a", "", "")
	if err != nil {
		t.Fatalf("selectTests() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" {
		t.Errorf("selection = %+v, want [b a] in flag order", got)
	}

	if _, err := selectTests(reg, "a,nope", "", ""); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("unknown id error = %v, want mention of %q", err, "nope")
	}

	got, err = selectTests(reg, "", "y", "")
	if err != nil || len(got) != 1 || got[0].ID != "b" {
		t.Errorf("category filter = %+v, %v; want [b], nil", got, err)
	}

	if _, err := selectTests(reg, "", "zzz", ""); err == nil {
		t.Error("empty category filter: error = nil, want no-match error")
	}
}

func TestHealthCommand_RequiresArtifacts(t *testing.T) {
	if err := healthCommand(nil); err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Errorf("healthCommand(nil) error = %v, want usage error", err)
	}
	if err := healthCommand([]string{"/nonexistent-artifact.json"}); err == nil ||
		!strings.Contains(err.Error(), "load /nonexistent-artifact.json") {
		t.Errorf("healthCommand(missing file) error = %v, want load error naming the file", err)
	}
}

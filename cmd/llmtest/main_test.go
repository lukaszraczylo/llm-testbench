package main

import (
	"testing"

	"github.com/lukaszraczylo/llm-testbench/internal/report"
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

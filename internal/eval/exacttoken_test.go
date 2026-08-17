package eval

import (
	"context"
	"testing"
)

func TestNormalizeExactToken(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare", "GET", "GET"},
		{"surrounding whitespace", "  GET  \n", "GET"},
		{"single-quoted", "'GET'", "GET"},
		{"double-quoted", "\"GET\"", "GET"},
		{"backtick-quoted", "`GET`", "GET"},
		{"bolded", "**GET**", "GET"},
		{"trailing period", "GET.", "GET"},
		{"quoted with trailing period", "\"GET\".", "GET"},
		{"fenced, untagged", "```\nGET\n```", "GET"},
		{"fenced, tagged", "```text\nGET\n```", "GET"},
		{"fenced with surrounding prose", "The answer is:\n```\nGET\n```", "GET"},
		{"multi-word phrase", "Access-Control-Allow-Headers", "Access-Control-Allow-Headers"},
		{"multi-word phrase bolded with period", "**Access-Control-Allow-Headers**.", "Access-Control-Allow-Headers"},
		// Regression: an unpaired leading asterisk is meaningful content
		// (a Go pointer type), not markdown decoration, and must survive.
		{"unpaired leading asterisk is content, not decoration", "*string", "*string"},
		{"unpaired trailing asterisk is content, not decoration", "string*", "string*"},
		{"nested quote-then-bold with trailing period", "**\"GET\"**.", "GET"},
		{"italic single-asterisk wrap", "*GET*", "GET"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeExactToken(tt.in)
			if got != tt.want {
				t.Errorf("NormalizeExactToken(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExactToken(t *testing.T) {
	tests := []struct {
		name     string
		want     string
		response string
		wantVal  float64
	}{
		{"bare exact match", "MX", "MX", 1},
		{"case insensitive", "MX", "mx", 1},
		{"quoted", "MX", "'MX'", 1},
		{"bolded", "MX", "**MX**", 1},
		{"trailing period", "MX", "MX.", 1},
		{"fenced", "MX", "```\nMX\n```", 1},
		{"restates a longer option list scores 0", "MX", "A, AAAA, CNAME, TXT, NS, MX", 0},
		{"wrong answer", "MX", "TXT", 0},
		{"empty response", "MX", "", 0},
		{"phrase exact match", "Access-Control-Allow-Headers", "Access-Control-Allow-Headers", 1},
		{"phrase restated with prose scores 0", "Access-Control-Allow-Headers", "The missing header is Access-Control-Allow-Headers.", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExactToken(tt.want).Evaluate(context.Background(), tt.response)
			if got.Value != tt.wantVal {
				t.Errorf("ExactToken(%q).Evaluate(%q) = %v, want %v (detail: %s)", tt.want, tt.response, got.Value, tt.wantVal, got.Detail)
			}
		})
	}
}

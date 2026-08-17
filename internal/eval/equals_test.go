package eval

import (
	"context"
	"testing"
)

func TestEquals(t *testing.T) {
	tests := []struct {
		name     string
		want     string
		response string
		wantVal  float64
	}{
		{"exact match", "patch", "patch", 1},
		{"case insensitive", "PATCH", "patch", 1},
		{"trimmed whitespace", "patch", "  patch\n", 1},
		{"mismatch", "patch", "minor", 0},
		{"substring is not equal", "patch", "patch bump", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Equals(tt.want).Evaluate(context.Background(), tt.response)
			if got.Value != tt.wantVal {
				t.Errorf("Equals(%q).Evaluate(%q) = %v, want %v", tt.want, tt.response, got.Value, tt.wantVal)
			}
		})
	}
}

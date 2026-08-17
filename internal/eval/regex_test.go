package eval

import (
	"context"
	"testing"
)

func TestRegex(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		response string
		want     float64
	}{
		{"matches", `ssh\s+root@10\.0\.0\.100`, "run: ssh root@10.0.0.100 'pct exec 251 -- systemctl status myservice'", 1},
		{"no match", `ssh\s+root@10\.0\.0\.100`, "ssh root@10.0.0.200", 0},
		{"anchored word boundary", `\bpct exec 251\b`, "use pct exec 251 -- cmd", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Regex(tt.pattern).Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Regex(%q).Evaluate(%q) = %v, want %v", tt.pattern, tt.response, got.Value, tt.want)
			}
		})
	}
}

func TestRegex_PanicsOnInvalidPattern(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Regex() with invalid pattern did not panic")
		}
	}()
	Regex(`(unclosed`)
}

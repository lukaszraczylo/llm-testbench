package eval

import (
	"context"
	"testing"
)

func TestContainsAll(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		substrings []string
		want       float64
	}{
		{"all present", "use launchd with StartInterval", []string{"launchd", "StartInterval"}, 1},
		{"case insensitive", "use Launchd here", []string{"LAUNCHD"}, 1},
		{"partial credit", "alpha and beta only", []string{"alpha", "beta", "gamma", "delta"}, 0.5},
		{"none present", "nothing matches", []string{"x", "y"}, 0},
		{"empty list", "anything", nil, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainsAll(tt.substrings...).Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("ContainsAll(%v).Evaluate(%q) = %v, want %v (detail: %s)", tt.substrings, tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		substrings []string
		want       float64
	}{
		{"one matches", "install coreutils via brew", []string{"coreutils", "gtimeout"}, 1},
		{"case insensitive match", "use gtimeout instead", []string{"GTIMEOUT"}, 1},
		{"none match", "totally unrelated text", []string{"foo", "bar"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainsAny(tt.substrings...).Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("ContainsAny(%v).Evaluate(%q) = %v, want %v", tt.substrings, tt.response, got.Value, tt.want)
			}
		})
	}
}

func TestNotContains(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		substrings []string
		want       float64
	}{
		{"forbidden absent", "git commit the manifest change", []string{"kubectl edit"}, 1},
		{"forbidden present", "just run kubectl edit deployment", []string{"kubectl edit"}, 0},
		{"case insensitive forbidden", "run kubectl patch now", []string{"KUBECTL PATCH"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NotContains(tt.substrings...).Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("NotContains(%v).Evaluate(%q) = %v, want %v", tt.substrings, tt.response, got.Value, tt.want)
			}
		})
	}
}

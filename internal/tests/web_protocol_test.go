package tests

import (
	"context"
	"testing"
)

func TestWebSitemapMaxURLsTest_Eval(t *testing.T) {
	tc := webSitemapMaxURLsTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "50000", 1},
		{"prose wrapped", "The limit is 50000 URLs per sitemap file.", 1},
		{"wrong: confused with the uncompressed size limit in MB", "50", 0},
		{"wrong entirely", "10000", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestWebHTTPStatusScenariosTest_Eval(t *testing.T) {
	tc := webHTTPStatusScenariosTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"all correct", `{"scenario1":400,"scenario2":401,"scenario3":403,"scenario4":301}`, 1},
		{"swapped 401 and 403", `{"scenario1":400,"scenario2":403,"scenario3":401,"scenario4":301}`, 0.5},
		{"used 500 for a client error", `{"scenario1":500,"scenario2":401,"scenario3":403,"scenario4":301}`, 0.75},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestWebDNSMXRecordTest_Eval(t *testing.T) {
	tc := webDNSMXRecordTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"bare", "MX", 1},
		{"lowercase", "mx", 1},
		{"with the word record", "MX record", 1},
		{"wrong: CNAME", "CNAME", 0},
		{"wrong: A record", "A", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestWebCanonicalVsRedirectTest_Eval(t *testing.T) {
	tc := webCanonicalVsRedirectTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct lowercase", "canonical", 1},
		{"correct capitalized with period", "Canonical.", 1},
		{"wrong: redirect", "redirect", 0},
		{"wrong: extra words break the forced one-word format", "Use a canonical tag", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

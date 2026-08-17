package tests

import (
	"context"
	"net/url"
	"testing"
)

func TestWebHTMLProductExtractTest_Eval(t *testing.T) {
	tc := webHTMLProductExtractTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"all correct", `{"name":"Wireless Mechanical Keyboard","price":129.99,"sku":"WBH-2201"}`, 1},
		{"one field wrong", `{"name":"Wireless Mechanical Keyboard","price":99.99,"sku":"WBH-2201"}`, 2.0 / 3.0},
		{"all wrong", `{"name":"Wrong Product","price":1,"sku":"XXX"}`, 0},
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

func TestWebHreflangReciprocalTest_Eval(t *testing.T) {
	tc := webHreflangReciprocalTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"page_missing_link":"https://example.com/fr/","missing_target":"https://example.com/en/"}`, 1},
		{"correct page, wrong target", `{"page_missing_link":"https://example.com/fr/","missing_target":"https://example.com/es/"}`, 0.5},
		{"wrong page entirely", `{"page_missing_link":"https://example.com/es/","missing_target":"https://example.com/en/"}`, 0.5},
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

func TestWebParseURL_GroundTruth(t *testing.T) {
	// Independent re-derivation via net/url.Parse, not hand-derived, per
	// the ground-truth discipline.
	u, err := url.Parse(webParseURL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", webParseURL, err)
	}

	if got, want := u.Scheme, "https"; got != want {
		t.Errorf("Scheme = %q, want %q", got, want)
	}
	if got, want := u.Hostname(), "api.example.com"; got != want {
		t.Errorf("Hostname() = %q, want %q", got, want)
	}
	if got, want := u.Port(), "8443"; got != want {
		t.Errorf("Port() = %q, want %q", got, want)
	}
	if got, want := u.Path, "/v2/search"; got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
	if got, want := u.RawQuery, "q=hello+world&sort=desc"; got != want {
		t.Errorf("RawQuery = %q, want %q", got, want)
	}
	if got, want := u.Fragment, "results"; got != want {
		t.Errorf("Fragment = %q, want %q", got, want)
	}
}

func TestWebURLParseComponentsTest_Eval(t *testing.T) {
	tc := webURLParseComponentsTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			"all correct",
			`{"scheme":"https","host":"api.example.com","port":8443,"path":"/v2/search","query":"q=hello+world&sort=desc","fragment":"results"}`,
			1,
		},
		{
			"port included in host field is still wrong for host",
			`{"scheme":"https","host":"api.example.com:8443","port":8443,"path":"/v2/search","query":"q=hello+world&sort=desc","fragment":"results"}`,
			5.0 / 6.0,
		},
		{
			"all wrong",
			`{"scheme":"http","host":"wrong.example.com","port":80,"path":"/","query":"","fragment":""}`,
			0,
		},
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

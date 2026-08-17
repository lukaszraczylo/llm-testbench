package eval

import (
	"context"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
		wantErr  bool
	}{
		{"plain object", `{"bump":"minor"}`, `{"bump":"minor"}`, false},
		{"fenced object", "Here you go:\n```json\n{\"bump\":\"minor\"}\n```", `{"bump":"minor"}`, false},
		{"object with prose around", `The answer is {"bump":"minor"} as computed.`, `{"bump":"minor"}`, false},
		{"nested braces ignored inside strings", `{"note":"a { b } c","bump":"patch"}`, `{"note":"a { b } c","bump":"patch"}`, false},
		{"array", `["ClaudeBot","Googlebot"]`, `["ClaudeBot","Googlebot"]`, false},
		{"no json", "no json here at all", "", true},
		// S7: a candidate JSON considered during reasoning, followed by the
		// real final JSON, must resolve to the final one.
		{
			name:     "reasoning with a candidate JSON then a final JSON: final wins",
			response: `Let me think. One option would be {"bump":"major"} but that is not right. The actual answer is {"bump":"minor"}.`,
			want:     `{"bump":"minor"}`,
			wantErr:  false,
		},
		{
			name:     "fenced json block preferred over any other json-looking text before it",
			response: "Draft: {\"bump\":\"major\"}\n\nFinal answer:\n```json\n{\"bump\":\"minor\"}\n```",
			want:     `{"bump":"minor"}`,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractJSON(tt.response)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ExtractJSON(%q) error = %v, wantErr %v", tt.response, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("ExtractJSON(%q) = %q, want %q", tt.response, got, tt.want)
			}
		})
	}
}

func TestJSONField_String(t *testing.T) {
	e := JSONField("bump", "minor")
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact match", `{"bump":"minor"}`, 1},
		{"case insensitive", `{"bump":"MINOR"}`, 1},
		{"wrong value", `{"bump":"patch"}`, 0},
		{"missing field", `{"other":"x"}`, 0},
		{"fenced json", "```json\n{\"bump\": \"minor\"}\n```", 1},
		{"invalid json", "not json at all", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestJSONField_Int(t *testing.T) {
	e := JSONField("M", 16)
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact match", `{"M":16,"efConstruction":200,"efSearch":50}`, 1},
		{"wrong value", `{"M":32}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestJSONField_Int_NumericStringCoercion is the B9 regression: a model
// answering {"line":"8"} instead of {"line":8} must still score 1 - the
// prompt asked for a JSON field with the right value, not a specific JSON
// type.
func TestJSONField_Int_NumericStringCoercion(t *testing.T) {
	e := JSONField("line", 8)
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"numeric string coerces", `{"line":"8"}`, 1},
		{"numeric string with whitespace coerces", `{"line":" 8 "}`, 1},
		{"wrong numeric string", `{"line":"9"}`, 0},
		{"non-numeric string does not coerce", `{"line":"eight"}`, 0},
		{"native number still works", `{"line":8}`, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestJSONField_Float_NumericStringCoercion mirrors the int case for a
// float64-typed field.
func TestJSONField_Float_NumericStringCoercion(t *testing.T) {
	e := JSONField("port", float64(8080))
	got := e.Evaluate(context.Background(), `{"port":"8080"}`)
	if got.Value != 1 {
		t.Errorf("Evaluate() = %v, want 1 (detail: %s)", got.Value, got.Detail)
	}
}

// TestJSONField_Bool_StringCoercion is the DC2 regression, mirroring B9's
// numeric-string coercion: a model answering {"guaranteed_visible":"false"}
// instead of {"guaranteed_visible":false} must still score 1.
func TestJSONField_Bool_StringCoercion(t *testing.T) {
	e := JSONField("guaranteed_visible", false)
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"bool string coerces (lowercase)", `{"guaranteed_visible":"false"}`, 1},
		{"bool string coerces with whitespace", `{"guaranteed_visible":" false "}`, 1},
		{"bool string coerces, different case", `{"guaranteed_visible":"False"}`, 1},
		{"wrong bool string", `{"guaranteed_visible":"true"}`, 0},
		{"non-boolean string does not coerce", `{"guaranteed_visible":"nope"}`, 0},
		{"native bool still works", `{"guaranteed_visible":false}`, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestJSONField_EnumHyphenSpaceFold is the DC4 regression: a multi-word
// enum answer spelled with spaces instead of hyphens (or vice versa) must
// still score 1.
func TestJSONField_EnumHyphenSpaceFold(t *testing.T) {
	e := JSONField("anomaly", "non-repeatable-read")
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact hyphenated form", `{"anomaly":"non-repeatable-read"}`, 1},
		{"space-separated form folds equal (DC4 bug probe)", `{"anomaly":"non repeatable read"}`, 1},
		{"mixed hyphen/space form folds equal (DC4 bug probe)", `{"anomaly":"non repeatable-read"}`, 1},
		{"a genuinely different enum value still fails", `{"anomaly":"dirty-read"}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestJSONField_NestedPath(t *testing.T) {
	e := JSONField("task1", "search_web")
	got := e.Evaluate(context.Background(), `{"task1":"search_web","task2":"read_file"}`)
	if got.Value != 1 {
		t.Errorf("Evaluate() = %v, want 1 (detail: %s)", got.Value, got.Detail)
	}
}

func TestJSONField_IndexedPath(t *testing.T) {
	e := JSONField("steps[0]", "build")
	got := e.Evaluate(context.Background(), `{"steps":["build","test","deploy"]}`)
	if got.Value != 1 {
		t.Errorf("Evaluate() = %v, want 1 (detail: %s)", got.Value, got.Detail)
	}
	got = e.Evaluate(context.Background(), `{"steps":["test","build","deploy"]}`)
	if got.Value != 0 {
		t.Errorf("Evaluate() = %v, want 0 (detail: %s)", got.Value, got.Detail)
	}
}

func TestJSONStringSet(t *testing.T) {
	e := JSONStringSet([]string{"ClaudeBot", "Googlebot"})
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact set", `["ClaudeBot","Googlebot"]`, 1},
		{"exact set different order", `["Googlebot","ClaudeBot"]`, 1},
		{"case insensitive", `["claudebot","googlebot"]`, 1},
		{"missing one", `["ClaudeBot"]`, 0.5},
		{"extra one", `["ClaudeBot","Googlebot","GPTBot"]`, float64(2) / 3},
		{"completely wrong", `["GPTBot","PerplexityBot"]`, 0},
		{"invalid json", "no array here", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestJSONStringArrayEquals(t *testing.T) {
	want := []string{"build", "test", "backup", "deploy", "verify"}
	e := JSONStringArrayEquals(want)

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact order", `["build","test","backup","deploy","verify"]`, 1},
		{"case insensitive exact order", `["BUILD","TEST","BACKUP","DEPLOY","VERIFY"]`, 1},
		{"wrong order", `["build","backup","test","deploy","verify"]`, 0},
		{"wrong length", `["build","test"]`, 0},
		{"invalid json", "not an array", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

package eval

import (
	"context"
	"testing"
)

func TestExtractLastNumber(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     float64
		wantErr  bool
	}{
		{"plain number", "0.8785", 0.8785, false},
		{"trailing number after prose", "The cosine similarity is 0.8785", 0.8785, false},
		{"last of multiple numbers", "step 1 then step 2 result is 42", 42, false},
		{"negative number", "the delta is -3.5", -3.5, false},
		{"no number", "no digits here", 0, true},
		// S6 regressions.
		{"unit qualifier after the answer is skipped", "24 bytes on a 64-bit system", 24, false},
		{"leading-dot decimal", "= .9896", 0.9896, false},
		{"answer on last line, reasoning with numbers on earlier lines", "step 1 uses 8 items\nstep 2 uses 16 items\nfinal answer: 42", 42, false},
		{"last line has only a unit-qualified number, falls back to whole text", "the computation used 100 iterations\nresult on a 64-bit system", 100, false},
		// 5b regressions: opus round 2's failing set.
		{"platform qualifier with parenthetical acronym", "24 bytes on a 64-bit (LP64) platform.", 24, false},
		{"digits glued into an identifier via underscore", "sizeof = 24 bytes on x86_64", 24, false},
		{"digits glued into an identifier via hyphen (bogus sign)", "sizeof = 24 bytes on x86-64", 24, false},
		{"multiplier suffix accepted", "The compression ratio is 64x.", 64, false},
		{"multiplier suffix at end of text", "speedup: 8x", 8, false},
		{"hex literal never a multiplier", "the flag is 0x2A so the answer is 42", 42, false},
		{"hyphen-compound number accepted as a last resort", "It is a 24-byte struct.", 24, false},
		// Documented limitation, not a regression: the heuristic has no
		// notion of "the total" vs. "a breakdown component" and returns
		// the last standalone number, which here is not the answer a
		// human would pick. Prompts should ask for the number alone.
		{"ambiguous parenthetical breakdown returns the wrong number by design", "Total: 24 bytes (22 bytes of members plus 2 tail padding)", 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractLastNumber[float64](tt.response)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ExtractLastNumber(%q) error = %v, wantErr %v", tt.response, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("ExtractLastNumber(%q) = %v, want %v", tt.response, got, tt.want)
			}
		})
	}
}

func TestNumeric_Float(t *testing.T) {
	e := Numeric(ExtractLastNumber[float64], 0.8785, 0.0005)

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "0.8785", 1},
		{"within tolerance above", "0.8789", 1},
		{"within tolerance below", "0.8781", 1},
		{"outside tolerance", "0.9000", 0},
		{"unparsable", "no number here", 0},
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

func TestNumeric_IntExact(t *testing.T) {
	e := Numeric(ExtractLastNumber[int], 32, 0)

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact match", "32", 1},
		{"off by one", "33", 0},
		{"exact match with prose", "The sizeof value is 32 bytes", 1},
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

func TestNumeric_UnsignedNoUnderflow(t *testing.T) {
	// Regression: unsigned subtraction must not wrap when got < want.
	e := Numeric(ExtractLastNumber[uint], 10, 2)
	got := e.Evaluate(context.Background(), "9")
	if got.Value != 1 {
		t.Errorf("Evaluate(9) with want=10 tol=2 = %v, want 1 (detail: %s)", got.Value, got.Detail)
	}
	got = e.Evaluate(context.Background(), "5")
	if got.Value != 0 {
		t.Errorf("Evaluate(5) with want=10 tol=2 = %v, want 0 (detail: %s)", got.Value, got.Detail)
	}
}

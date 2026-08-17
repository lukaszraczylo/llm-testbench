package eval

import "testing"

func TestExtractCodeBlock(t *testing.T) {
	tests := []struct {
		name     string
		response string
		lang     string
		want     string
	}{
		{
			name:     "tagged block matches lang",
			response: "Here:\n```go\nfunc F() {}\n```\nDone.",
			lang:     "go",
			want:     "func F() {}",
		},
		{
			name:     "case insensitive lang tag",
			response: "```GO\nfunc F() {}\n```",
			lang:     "go",
			want:     "func F() {}",
		},
		{
			name:     "prefers matching lang over other tagged blocks",
			response: "```json\n{\"x\":1}\n```\n```go\nfunc F() {}\n```",
			lang:     "go",
			want:     "func F() {}",
		},
		{
			name:     "falls back to untagged block",
			response: "```\nfunc F() {}\n```",
			lang:     "go",
			want:     "func F() {}",
		},
		{
			name:     "no fences falls back to whole response",
			response: "  func F() {}  ",
			lang:     "go",
			want:     "func F() {}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractCodeBlock(tt.response, tt.lang)
			if got != tt.want {
				t.Errorf("ExtractCodeBlock(%q, %q) = %q, want %q", tt.response, tt.lang, got, tt.want)
			}
		})
	}
}

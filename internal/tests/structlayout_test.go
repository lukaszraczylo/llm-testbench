package tests

import (
	"context"
	"testing"
	"unsafe"
)

// The four field orders below all reach the 32-byte minimum: the two
// 8-byte-aligned fields (Name, ID) may go in either order at the front, and
// the two 1-byte fields (Active, Flag) may go in either order at the back,
// independent of each other. optimalOrderNameIDPriorityActiveFlag mirrors
// optimalTask in golang_test.go; the other three are its remaining
// permutations.
type optimalOrderNameIDPriorityFlagActive struct {
	Name     string
	ID       int64
	Priority int32
	Flag     byte
	Active   bool
}

// The govet suppressions below are intentional: each of these types
// encodes one specific, named field permutation under test (its own name
// documents the order); fieldalignment's suggested reorder (grouping the
// pointer-containing string field first, for GC-scan cost, not total size)
// would silently make two of these permutations identical to each other
// and defeat the point of testing them as distinct orderings.

//nolint:govet // fieldalignment: intentional, see comment above
type optimalOrderIDNamePriorityActiveFlag struct {
	ID       int64
	Name     string
	Priority int32
	Active   bool
	Flag     byte
}

//nolint:govet // fieldalignment: intentional, see comment above
type optimalOrderIDNamePriorityFlagActive struct {
	ID       int64
	Name     string
	Priority int32
	Flag     byte
	Active   bool
}

func TestComputeLP64StructSize_CrossCheckUnsafeSizeof(t *testing.T) {
	tests := []struct {
		name       string
		fieldTypes []string
		want       uintptr
	}{
		{"wasteful order (matches wastefulTask)", []string{"bool", "int64", "int32", "string", "byte"}, unsafe.Sizeof(wastefulTask{})},
		{"Name,ID,Priority,Active,Flag (matches optimalTask)", []string{"string", "int64", "int32", "bool", "byte"}, unsafe.Sizeof(optimalTask{})},
		{"Name,ID,Priority,Flag,Active", []string{"string", "int64", "int32", "byte", "bool"}, unsafe.Sizeof(optimalOrderNameIDPriorityFlagActive{})},
		{"ID,Name,Priority,Active,Flag", []string{"int64", "string", "int32", "bool", "byte"}, unsafe.Sizeof(optimalOrderIDNamePriorityActiveFlag{})},
		{"ID,Name,Priority,Flag,Active", []string{"int64", "string", "int32", "byte", "bool"}, unsafe.Sizeof(optimalOrderIDNamePriorityFlagActive{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := computeLP64StructSize(tt.fieldTypes)
			if err != nil {
				t.Fatalf("computeLP64StructSize(%v) error = %v", tt.fieldTypes, err)
			}
			if got != tt.want {
				t.Errorf("computeLP64StructSize(%v) = %d, want %d (unsafe.Sizeof)", tt.fieldTypes, got, tt.want)
			}
		})
	}
}

func TestComputeLP64StructSize_UnknownType(t *testing.T) {
	_, err := computeLP64StructSize([]string{"bool", "float64"})
	if err == nil {
		t.Fatal("computeLP64StructSize() with an unknown type: error = nil, want error")
	}
}

func TestGoStructAlignEval_AllFourMinimalOrdersScoreFull(t *testing.T) {
	e := goStructAlignEval()

	orders := []string{
		"Name     string\n\tID       int64\n\tPriority int32\n\tActive   bool\n\tFlag     byte",
		"Name     string\n\tID       int64\n\tPriority int32\n\tFlag     byte\n\tActive   bool",
		"ID       int64\n\tName     string\n\tPriority int32\n\tActive   bool\n\tFlag     byte",
		"ID       int64\n\tName     string\n\tPriority int32\n\tFlag     byte\n\tActive   bool",
	}
	for i, order := range orders {
		response := "```go\ntype Task struct {\n\t" + order + "\n}\n```"
		got := e.Evaluate(context.Background(), response)
		if got.Value != 1 {
			t.Errorf("order %d: Evaluate() = %v, want 1 (detail: %s)", i, got.Value, got.Detail)
		}
	}
}

// TestGoStructAlignEval_TrailingCommentsAndTags is the 5c regression: a
// field line may carry a trailing "// comment" explaining the model's
// reasoning, and/or a Go struct tag, without breaking field extraction.
func TestGoStructAlignEval_TrailingCommentsAndTags(t *testing.T) {
	e := goStructAlignEval()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "trailing comment on every field",
			response: "```go\ntype Task struct {\n\tName     string // 16 bytes\n\tID       int64  // 8 bytes\n\tPriority int32  // 4 bytes\n\tActive   bool   // 1 byte\n\tFlag     byte   // 1 byte\n}\n```",
			want:     1,
		},
		{
			name:     "struct tag with no comment",
			response: "```go\ntype Task struct {\n\tName     string `json:\"name\"`\n\tID       int64  `json:\"id\"`\n\tPriority int32  `json:\"priority\"`\n\tActive   bool   `json:\"active\"`\n\tFlag     byte   `json:\"flag\"`\n}\n```",
			want:     1,
		},
		{
			name:     "struct tag followed by a comment",
			response: "```go\ntype Task struct {\n\tName     string `json:\"name\"` // 16 bytes\n\tID       int64  `json:\"id\"`\n\tPriority int32  `json:\"priority\"`\n\tActive   bool   `json:\"active\"`\n\tFlag     byte   `json:\"flag\"`\n}\n```",
			want:     1,
		},
		{
			name:     "wasteful order with comments still scores 0",
			response: "```go\ntype Task struct {\n\tActive   bool   // 1 byte\n\tID       int64  // 8 bytes\n\tPriority int32  // 4 bytes\n\tName     string // 16 bytes\n\tFlag     byte   // 1 byte\n}\n```",
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestGoStructAlignEval_RejectsBadResponses(t *testing.T) {
	e := goStructAlignEval()

	tests := []struct {
		name     string
		response string
	}{
		{
			name:     "original wasteful order",
			response: "```go\ntype Task struct {\n\tActive   bool\n\tID       int64\n\tPriority int32\n\tName     string\n\tFlag     byte\n}\n```",
		},
		{
			name:     "int32 not grouped after the align-8 fields",
			response: "```go\ntype Task struct {\n\tPriority int32\n\tName     string\n\tID       int64\n\tActive   bool\n\tFlag     byte\n}\n```",
		},
		{
			name:     "missing a field",
			response: "```go\ntype Task struct {\n\tName     string\n\tID       int64\n\tPriority int32\n\tActive   bool\n}\n```",
		},
		{
			name:     "wrong type for a field",
			response: "```go\ntype Task struct {\n\tName     string\n\tID       int32\n\tPriority int32\n\tActive   bool\n\tFlag     byte\n}\n```",
		},
		{
			name:     "duplicated field",
			response: "```go\ntype Task struct {\n\tName     string\n\tID       int64\n\tPriority int32\n\tActive   bool\n\tActive   bool\n}\n```",
		},
		{
			name:     "no recognizable fields",
			response: "I'm not sure how to reorder this.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Evaluate(context.Background(), tt.response)
			if got.Value != 0 {
				t.Errorf("Evaluate(%q) = %v, want 0 (detail: %s)", tt.response, got.Value, got.Detail)
			}
		})
	}
}

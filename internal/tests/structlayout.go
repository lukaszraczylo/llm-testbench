package tests

import (
	"context"
	"fmt"
	"regexp"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
)

// lp64TypeLayout is one Go type's size and alignment, in bytes, on a 64-bit
// (LP64) system - the same values golang_test.go cross-checks against
// unsafe.Sizeof/unsafe.Alignof.
type lp64TypeLayout struct {
	size, align uintptr
}

// lp64FieldLayout covers the field types used by goStructAlignTest's
// prompt (bool, int64, int32, string, byte). string is a two-word header
// (an 8-byte pointer plus an 8-byte length) on a 64-bit system.
var lp64FieldLayout = map[string]lp64TypeLayout{
	"bool":   {size: 1, align: 1},
	"byte":   {size: 1, align: 1},
	"int32":  {size: 4, align: 4},
	"int64":  {size: 8, align: 8},
	"string": {size: 16, align: 8},
}

// computeLP64StructSize simulates the Go compiler's struct layout
// algorithm on a 64-bit system for a sequence of field types, in
// declaration order: each field is padded up to its own alignment, then
// the total is padded up to the widest field's alignment. It is general
// over any sequence of the types in lp64FieldLayout, not just the specific
// field set goStructAlignTest asks about.
func computeLP64StructSize(fieldTypes []string) (uintptr, error) {
	var offset, maxAlign uintptr = 0, 1
	for _, ft := range fieldTypes {
		layout, ok := lp64FieldLayout[ft]
		if !ok {
			return 0, fmt.Errorf("unknown field type %q", ft)
		}
		if layout.align > maxAlign {
			maxAlign = layout.align
		}
		if rem := offset % layout.align; rem != 0 {
			offset += layout.align - rem
		}
		offset += layout.size
	}
	if rem := offset % maxAlign; rem != 0 {
		offset += maxAlign - rem
	}
	return offset, nil
}

// structAlignFieldPattern matches one "Name Type" struct field declaration
// line for the specific 5 fields goStructAlignTest's prompt gives (Active
// bool, ID int64, Priority int32, Name string, Flag byte), in whatever
// order the response declares them. An optional Go struct tag
// ("`json:\"name\"`") and/or an optional trailing "// comment" (e.g. "Name
// string // 16 bytes", a model explaining its layout reasoning inline) are
// both tolerated between the type and the end of the line (5c).
var structAlignFieldPattern = regexp.MustCompile("(?m)^\\s*(Active|ID|Priority|Name|Flag)\\s+(bool|int64|int32|string|byte)(?:\\s+`[^`]*`)?\\s*(?://.*)?$")

// structAlignWantFields is the required field name -> type mapping;
// goStructAlignEval rejects a response that renames, retypes, drops, or
// duplicates any of them.
var structAlignWantFields = map[string]string{
	"Active":   "bool",
	"ID":       "int64",
	"Priority": "int32",
	"Name":     "string",
	"Flag":     "byte",
}

// goStructAlignMinimalSize is the smallest possible layout size for these 5
// fields on LP64 (see the ground-truth derivation in golang.go's doc
// comment); golang_test.go recomputes this via computeLP64StructSize
// independently of the hardcoded constant.
const goStructAlignMinimalSize = 32

// goStructAlignEval extracts the field order from the response's struct
// definition and computes its actual LP64 layout size, rather than
// matching one hardcoded field order: any of the several field orderings
// that reach the 32-byte minimum scores full credit (S3).
func goStructAlignEval() eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		matches := structAlignFieldPattern.FindAllStringSubmatch(response, -1)

		seen := make(map[string]bool, len(structAlignWantFields))
		types := make([]string, 0, len(matches))
		for _, m := range matches {
			name, typ := m[1], m[2]
			wantType := structAlignWantFields[name]
			if typ != wantType {
				return eval.Score{Value: 0, Detail: fmt.Sprintf("field %s has type %s, want %s", name, typ, wantType)}
			}
			if seen[name] {
				return eval.Score{Value: 0, Detail: fmt.Sprintf("field %s appears more than once", name)}
			}
			seen[name] = true
			types = append(types, typ)
		}

		if len(seen) != len(structAlignWantFields) {
			return eval.Score{Value: 0, Detail: fmt.Sprintf("found %d/%d required fields", len(seen), len(structAlignWantFields))}
		}

		size, err := computeLP64StructSize(types)
		if err != nil {
			return eval.Score{Value: 0, Detail: err.Error()}
		}
		if size == goStructAlignMinimalSize {
			return eval.Score{Value: 1, Detail: fmt.Sprintf("computed layout size = %d bytes (minimal)", size)}
		}
		return eval.Score{Value: 0, Detail: fmt.Sprintf("computed layout size = %d bytes, want %d (minimal)", size, goStructAlignMinimalSize)}
	})
}

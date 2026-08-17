package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerCTests(r *testkit.Registry) {
	r.Register(cStructSizeTest())
}

// cStructSizeWant is sizeof(struct Config) on LP64 (64-bit System V / Apple
// ARM64 ABI: natural alignment, no packing).
//
// ground truth: char flag(align 1, size 1) at offset 0; double ratio
// (align 8, size 8) needs offset divisible by 8, so it starts at offset 8
// (7 bytes padding after flag), ending at 16; int count (align 4, size 4)
// at offset 16 (already aligned), ending at 20; short mode (align 2, size
// 2) at offset 20, ending at 22. The struct's own alignment is the widest
// member alignment (8, from double), so the total size rounds up from 22
// to the next multiple of 8: 24. c_test.go cross-checks this with cc via
// sizeof and offsetof when the cc toolchain is available.
const cStructSizeWant = 24

func cStructSizeTest() testkit.Test {
	prompt := `Here is a C struct:

` + "```c" + `
struct Config {
	char flag;
	double ratio;
	int count;
	short mode;
};
` + "```" + `

Assuming the LP64 data model (64-bit, no #pragma pack, no
non-default alignment attributes), what is sizeof(struct Config) in bytes?
Respond with only the number, nothing else.`

	return testkit.Test{
		ID:          "c-struct-size",
		Category:    "programming",
		Subcategory: "c",
		Description: "Compute sizeof() for a C struct on LP64 given its member types and default alignment.",
		Prompt:      prompt,
		MaxTokens:   200,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], cStructSizeWant, 0),
	}
}

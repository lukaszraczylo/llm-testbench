package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerCTests(r *testkit.Registry) {
	r.Register(cStructSizeTest())
	r.Register(cPointerArithmeticTest())
	r.Register(cBitmaskOpsTest())
	r.Register(cUnionSizeLP64Test())
	r.Register(cStringFunctionOutputTest())
	r.Register(cIntegerPromotionOverflowTest())
	r.Register(cArrayDecaySizeofTest())
	r.Register(cMacroExpansionPitfallTest())
	r.Register(cUndefinedBehaviorSpotTest())
	r.Register(cStructBitfieldPackingTest())
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

// cPointerArithmeticCode is the inline snippet for cPointerArithmeticTest.
const cPointerArithmeticCode = `int arr[6] = {10, 20, 30, 40, 50, 60};
int *p = arr;
p += 2;
int *q = &arr[5];
long diff = q - p;
int val = *(p + 1) + diff;`

// cPointerArithmeticWant is the value of val.
//
// ground truth: p starts at &arr[0], then p += 2 advances it by 2 ints
// (pointer arithmetic scales by the pointee's size), landing on &arr[2]
// (value 30). q = &arr[5] (value 60). Pointer subtraction (q - p) yields the
// number of elements between them, not bytes: (&arr[5] - &arr[2]) = 3. *(p +
// 1) dereferences arr[3] (value 40). val = 40 + 3 = 43. c_test.go
// cross-checks this by compiling and running the identical snippet with cc.
const cPointerArithmeticWant = 43

func cPointerArithmeticTest() testkit.Test {
	prompt := `Here is C code:

` + "```c" + `
` + cPointerArithmeticCode + `
` + "```" + `

Assuming a 6-element int array laid out contiguously with no padding
between elements, what is the value of val? Respond with only the number,
nothing else.`

	return testkit.Test{
		ID:          "c-pointer-arithmetic",
		Category:    "programming",
		Subcategory: "c",
		Description: "Trace the exact result of pointer arithmetic and pointer subtraction over an inline int array.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], cPointerArithmeticWant, 0),
	}
}

// cBitmaskOpsCode is the inline snippet for cBitmaskOpsTest.
const cBitmaskOpsCode = `unsigned int flags = 0;
flags |= (1 << 2);
flags |= (1 << 5);
flags &= ~(1 << 2);
flags ^= (1 << 0);`

// cBitmaskOpsWant is the final value of flags.
//
// ground truth: starting from 0: |= (1<<2) sets bit 2 -> 0b000100 (4); |=
// (1<<5) sets bit 5 -> 0b100100 (36); &= ~(1<<2) clears bit 2 -> 0b100000
// (32); ^= (1<<0) toggles bit 0, which is currently 0, so it becomes 1 ->
// 0b100001 (33). c_test.go cross-checks this by compiling and running the
// identical snippet with cc.
const cBitmaskOpsWant = 33

func cBitmaskOpsTest() testkit.Test {
	prompt := `Here is C code:

` + "```c" + `
` + cBitmaskOpsCode + `
` + "```" + `

What is the final value of flags, as a decimal number? Respond with only
the number, nothing else.`

	return testkit.Test{
		ID:          "c-bitmask-ops",
		Category:    "programming",
		Subcategory: "c",
		Description: "Trace the final decimal value of a chain of bit set/clear/toggle operations.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], cBitmaskOpsWant, 0),
	}
}

// cUnionSizeLP64Code is the inline snippet for cUnionSizeLP64Test.
const cUnionSizeLP64Code = `union Packet {
	double timestamp;
	char raw[3];
	struct { int a; short b; } fields;
};`

// cUnionSizeLP64Want is sizeof(union Packet) on LP64.
//
// ground truth: a union's size is at least the largest member's size,
// rounded up to the union's own alignment (the largest member alignment).
// timestamp: size 8, align 8. raw: size 3, align 1. fields (struct{int a;
// short b;}): a at offset 0 (size 4), b at offset 4 (size 2, already
// aligned), giving 6 bytes of payload padded up to the struct's own
// alignment (4, from int) -> struct size 8, align 4. The union's size is
// max(8, 3, 8) = 8, and its alignment is max(8, 1, 4) = 8; 8 is already a
// multiple of 8, so sizeof(union Packet) = 8. c_test.go cross-checks this
// with cc.
const cUnionSizeLP64Want = 8

func cUnionSizeLP64Test() testkit.Test {
	prompt := `Here is a C union:

` + "```c" + `
` + cUnionSizeLP64Code + `
` + "```" + `

Assuming the LP64 data model (64-bit, no #pragma pack, no non-default
alignment attributes), what is sizeof(union Packet) in bytes? Respond with
only the number, nothing else.`

	return testkit.Test{
		ID:          "c-union-size-lp64",
		Category:    "programming",
		Subcategory: "c",
		Description: "Compute sizeof() for a C union on LP64, accounting for its largest member and its alignment.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], cUnionSizeLP64Want, 0),
	}
}

// cStringFunctionOutputKeys is the inline dataset for
// cStringFunctionOutputTest.
// ground truth: keys starting with "srv_" (checked with strncmp(key,
// "srv_", 4) == 0): srv_web, srv_db, srv_api, srv_auth = 4 matches (also
// re-derived in c_test.go by scanning the same literal data with Go's
// strings.HasPrefix).
const cStringFunctionOutputKeys = `"srv_web", "srv_db", "cache_1", "srv_api", "queue_2", "srv_auth"`

// cStringFunctionOutputWant is the expected stdout: the count of keys with
// the "srv_" prefix.
const cStringFunctionOutputWant = "4"

func cStringFunctionOutputTest() testkit.Test {
	prompt := `Here are 6 inline C string literals (configuration key names):

` + "```c" + `
const char *keys[] = {` + cStringFunctionOutputKeys + `};
` + "```" + `

Write a complete, self-contained C program (embed the array above exactly
as shown; do not read stdin or a file) that uses strncmp from <string.h> to
count how many of these 6 keys start with the prefix "srv_", and prints
just that count as a bare integer via printf, followed by a newline, and
nothing else.`

	return testkit.Test{
		ID:          "c-string-function-output",
		Category:    "programming",
		Subcategory: "c",
		Description: "Write a C program using strncmp to count inline strings matching a prefix.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        eval.CRun(eval.PassthroughHarness, cStringFunctionOutputWant),
	}
}

// cIntegerPromotionOverflowCode is the inline snippet for
// cIntegerPromotionOverflowTest.
const cIntegerPromotionOverflowCode = `unsigned char a = 200;
unsigned char b = 100;
unsigned char sum = a + b;`

// cIntegerPromotionOverflowWant is the value of sum.
//
// ground truth: in a + b, both unsigned char operands undergo the usual
// arithmetic conversions ("integer promotion"): each is promoted to int
// before the addition, so the addition computes 200 + 100 = 300 as a full
// int, with no overflow at that step. Assigning that int back to the
// unsigned char sum truncates it: unsigned integer types always wrap modulo
// 2^n (n=8 for unsigned char), which is well-defined behavior (not
// undefined, unlike signed overflow) - 300 mod 256 = 44. c_test.go
// cross-checks this by compiling and running the identical snippet with cc.
const cIntegerPromotionOverflowWant = 44

func cIntegerPromotionOverflowTest() testkit.Test {
	prompt := `Here is C code:

` + "```c" + `
` + cIntegerPromotionOverflowCode + `
` + "```" + `

What is the value of sum, printed as a decimal int (%d)? Respond with only
the number, nothing else.`

	return testkit.Test{
		ID:          "c-integer-promotion-overflow",
		Category:    "programming",
		Subcategory: "c",
		Description: "Trace unsigned char addition through integer promotion to int and truncation back to 8 bits.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], cIntegerPromotionOverflowWant, 0),
	}
}

// cArrayDecaySizeofCode is the inline snippet for cArrayDecaySizeofTest.
const cArrayDecaySizeofCode = `void check(int arr[10]) {
	printf("%zu\n", sizeof(arr));
}

int main(void) {
	int arr[10];
	printf("%zu\n", sizeof(arr));
	check(arr);
	return 0;
}`

// cArrayDecaySizeofWant is the sum of the two printed sizeof values.
//
// ground truth: in main, arr is a genuine 10-element int array in scope, so
// sizeof(arr) is the full array size: 10 * sizeof(int) = 40 (int is 4 bytes
// on LP64). Inside check, the parameter "int arr[10]" is adjusted by the
// compiler to "int *arr" (array-to-pointer decay in a function parameter
// declaration, per the C standard) - sizeof(arr) there measures the
// pointer, not the array: sizeof(int *) = 8 on LP64. Sum = 40 + 8 = 48.
// c_test.go cross-checks both individual values (and their sum) by
// compiling and running the identical snippet with cc.
const cArrayDecaySizeofWant = 48

func cArrayDecaySizeofTest() testkit.Test {
	prompt := `Here is a C program:

` + "```c" + `
#include <stdio.h>

` + cArrayDecaySizeofCode + `
` + "```" + `

Assuming LP64 (int is 4 bytes, pointers are 8 bytes), what is the sum of
the two numbers this program prints? Respond with only the number, nothing
else.`

	return testkit.Test{
		ID:          "c-array-decay-sizeof",
		Category:    "programming",
		Subcategory: "c",
		Description: "Compute the sum of sizeof(array) in its defining scope versus sizeof(array-parameter) after decay to a pointer.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], cArrayDecaySizeofWant, 0),
	}
}

// cMacroExpansionPitfallWant is the expected stdout: the result of the
// classic unparenthesized function-like macro pitfall.
//
// ground truth: #define SQUARE(x) x * x is a pure textual substitution with
// no parentheses around x or around the whole expansion, so
// SQUARE(2 + 3) expands to literally 2 + 3 * 2 + 3, not (2 + 3) * (2 + 3).
// Operator precedence then evaluates * before +: 2 + 3 * 2 + 3
// = 2 + 6 + 3 = 11, not the "squared" answer 25 the macro's name suggests.
// c_test.go cross-checks this by compiling and running the identical macro
// and call with cc.
const cMacroExpansionPitfallWant = "11"

func cMacroExpansionPitfallTest() testkit.Test {
	prompt := `Here is a C macro definition:

` + "```c" + `
#define SQUARE(x) x * x
` + "```" + `

Write a complete, self-contained C program that defines SQUARE exactly as
given above, then prints the result of SQUARE(2 + 3) as a bare integer via
printf("%d\n", ...), and nothing else.`

	return testkit.Test{
		ID:          "c-macro-expansion-pitfall",
		Category:    "programming",
		Subcategory: "c",
		Description: "Write a C program exposing the classic unparenthesized-macro operator-precedence pitfall.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        eval.CRun(eval.PassthroughHarness, cMacroExpansionPitfallWant),
	}
}

// cUndefinedBehaviorSpotCode is the inline, line-numbered snippet for
// cUndefinedBehaviorSpotTest.
const cUndefinedBehaviorSpotCode = `1: #include <limits.h>
2: #include <stdio.h>
3:
4: int main(void) {
5: 	int x = INT_MAX;
6: 	int y = x + 1;
7: 	printf("%d\n", y);
8: 	return 0;
9: }`

// cUndefinedBehaviorSpotKindSignedOverflow is the fixed literal
// cUndefinedBehaviorSpotTest requires for the "kind" JSON field.
const cUndefinedBehaviorSpotKindSignedOverflow = "signed-integer-overflow"

// cUndefinedBehaviorSpotTest: identify the line and kind of undefined
// behavior in an inline snippet.
//
// ground truth: x holds INT_MAX (the largest representable int). Per the C
// standard (C11 6.5p5), an operation whose mathematical result is not
// representable in its result type is undefined behavior for signed
// integer types (unlike unsigned types, which are defined to wrap). Line 6
// (x + 1) computes INT_MAX + 1, which overflows int - undefined behavior,
// not a well-defined wraparound. This cannot be "verified by execution":
// UB means the standard imposes no requirement on the observed result (in
// practice compilers may wrap, trap, or optimize the check away entirely),
// so c_test.go documents this ground truth from the standard rather than
// asserting a specific runtime output, the same approach golang.go's
// go-channel-deadlock test uses for its non-recomputable deadlock claim.
func cUndefinedBehaviorSpotTest() testkit.Test {
	prompt := `Here is C code, with line numbers prefixed:

` + "```c" + `
` + cUndefinedBehaviorSpotCode + `
` + "```" + `

Which line number contains undefined behavior? Pick exactly one kind from
this list: "signed-integer-overflow", "use-after-free", "uninitialized-read",
"buffer-overflow". Respond with only a JSON object:
{"line": <line number as an integer>, "kind": "<one of the four kinds above, exactly as written>"}`

	evaluator := eval.All(
		eval.W(eval.JSONField("line", 6), 1),
		eval.W(eval.JSONField("kind", cUndefinedBehaviorSpotKindSignedOverflow), 1),
	)

	return testkit.Test{
		ID:          "c-undefined-behavior-spot",
		Category:    "programming",
		Subcategory: "c",
		Description: "Identify the line and kind of undefined behavior in an inline C snippet (signed integer overflow).",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// cStructBitfieldPackingCode is the inline snippet for
// cStructBitfieldPackingTest.
const cStructBitfieldPackingCode = `struct Flags {
	unsigned int a : 3;
	unsigned int b : 5;
	unsigned int c : 24;
};`

// cStructBitfieldPackingWant is sizeof(struct Flags) on a typical LP64
// System V / Apple ARM64 ABI.
//
// ground truth: consecutive bitfields declared with the same base type
// (unsigned int, a 4-byte/32-bit storage unit) are packed into shared
// storage units by mainstream compilers (gcc/clang on both x86-64 System V
// and Apple's ARM64 ABI) when they fit: a (3 bits) + b (5 bits) + c (24
// bits) = 32 bits exactly, filling one 4-byte unsigned int storage unit
// with no padding needed. So sizeof(struct Flags) = 4. Exact bitfield
// packing is implementation-defined by the C standard (not mandated), so
// c_test.go cross-checks this empirically against the real cc toolchain
// rather than relying on the derivation alone.
const cStructBitfieldPackingWant = 4

func cStructBitfieldPackingTest() testkit.Test {
	prompt := `Here is a C struct with bitfields:

` + "```c" + `
` + cStructBitfieldPackingCode + `
` + "```" + `

Assuming a typical LP64 System V or Apple ARM64 ABI (gcc/clang, no
#pragma pack), what is sizeof(struct Flags) in bytes? Respond with only the
number, nothing else.`

	return testkit.Test{
		ID:          "c-struct-bitfield-packing",
		Category:    "programming",
		Subcategory: "c",
		Description: "Compute sizeof() for a C struct whose bitfields exactly fill one storage unit with no padding.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], cStructBitfieldPackingWant, 0),
	}
}

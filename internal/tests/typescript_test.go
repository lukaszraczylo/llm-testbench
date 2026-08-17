package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runNodeJS writes js (plain JavaScript - TypeScript syntax already erased
// by the caller) to a temp file and runs it with `node`, returning trimmed
// stdout. It skips the calling test (does not fail it) when node is not on
// PATH, mirroring the toolchain-detection contract eval.PyRun/GoRun/CRun
// use, since node is a ground-truth verification tool here, not a catalog
// exec.Evaluator (typescript.go owns no eval.Evaluator that shells out).
func runNodeJS(t *testing.T, js string) string {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found on PATH, skipping ground-truth cross-check")
	}

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "check.js")
	if err := os.WriteFile(scriptPath, []byte(js), 0o600); err != nil {
		t.Fatalf("write check.js: %v", err)
	}

	// #nosec G204 -- scriptPath is a fixed filename this test just wrote
	// into its own t.TempDir(); not external input.
	out, err := exec.Command("node", scriptPath).Output() //nolint:gosec // see comment above
	if err != nil {
		t.Fatalf("running check.js failed: %v", err)
	}
	return strings.TrimSpace(string(out))
}

const goodDebounceComposable = "```ts\n" + `import { ref, watch, onScopeDispose, type Ref } from 'vue'

export function useDebouncedRef<T>(source: Ref<T>, ms: number): Ref<T> {
	const debounced = ref(source.value) as Ref<T>
	let timer: ReturnType<typeof setTimeout> | undefined

	const stop = watch(source, (value) => {
		if (timer) clearTimeout(timer)
		timer = setTimeout(() => {
			debounced.value = value
		}, ms)
	})

	onScopeDispose(() => {
		if (timer) clearTimeout(timer)
		stop()
	})

	return debounced
}
` + "```"

func TestNoAnyEscapeHatch(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"clean typed code", goodDebounceComposable, 1},
		{"type annotation any", "function f(x: any) {}", 0},
		{"generic any", "const r: Ref<any> = ref(1)", 0},
		{"cast as any", "const x = value as any", 0},
		{"false positive guard: many", "there are many refs here", 1},
		{"false positive guard: Company", "Company.example()", 1},
		// N3 additions.
		{"non-first generic argument any", "const m: Map<string, any> = new Map()", 0},
		{"array of any", "const values: any[] = []", 0},
		{"false positive guard: many[] is not any[]", "const values: many[] = []", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := noAnyEscapeHatch().Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("noAnyEscapeHatch().Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

// tsDiscriminatedUnionAreaJS is a plain-JavaScript mirror of
// tsDiscriminatedUnionAreaCode (TypeScript's type annotations erased, since
// they have no runtime effect), used to cross-check the ground truth with
// `node` rather than trusting the doc comment's arithmetic alone.
const tsDiscriminatedUnionAreaJS = `function area(s) {
  switch (s.kind) {
    case "circle":
      return Math.PI * s.radius ** 2;
    case "rectangle":
      return s.width * s.height;
  }
}

const shapes = [
  { kind: "rectangle", width: 4, height: 5 },
  { kind: "circle", radius: 3 },
];

const total = shapes.map(area).reduce((a, b) => a + b, 0);
console.log(Math.round(total * 100) / 100);`

func TestTsDiscriminatedUnionArea_GroundTruth(t *testing.T) {
	got := runNodeJS(t, tsDiscriminatedUnionAreaJS)
	if want := "48.27"; got != want {
		t.Errorf("node output = %q, want %q", got, want)
	}
}

func TestTsDiscriminatedUnionAreaTest_Eval(t *testing.T) {
	tc := tsDiscriminatedUnionAreaTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact", "48.27", 1},
		{"within tolerance", "48.272", 1},
		{"prose wrapped", "The total area is 48.27.", 1},
		{"wrong: circle area only", "28.27", 0},
		{"wrong: unrounded-ish but off", "48.3", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestTsGenericUtilityTypeTest_Eval(t *testing.T) {
	tc := tsGenericUtilityTypeTest()

	goodDeepPartial := "```ts\n" + `type DeepPartial<T> = {
  [K in keyof T]?: T[K] extends object ? DeepPartial<T[K]> : T[K];
};
` + "```"

	missingRecursion := "```ts\n" + `type DeepPartial<T> = {
  [K in keyof T]?: T[K];
};
` + "```"

	usesAny := "```ts\n" + `type DeepPartial<T> = {
  [K in keyof T]?: any;
};
` + "```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct recursive DeepPartial", goodDeepPartial, 1},
		{"wrong: shallow only, no recursion into nested objects", missingRecursion, 0.75}, // (2*1 + 1*0 + 1*1)/4: mapped type ok, no DeepPartial<T[K]> reference, no any
		{"wrong: uses any, defeating the point", usesAny, 0.5},                            // (2*1 + 1*0 + 1*0)/4: mapped type ok, no recursion, uses any
		{
			// A9: "P" (TypeScript's own Partial<T> convention) is an
			// equally valid mapped-type variable name to "K" - the
			// recursive-reference check must not hardcode the literal
			// identifier.
			name: "correct recursive DeepPartial using P instead of K as the mapped-type variable (A9 bug probe)",
			response: "```ts\n" + `type DeepPartial<T> = {
  [P in keyof T]?: T[P] extends object ? DeepPartial<T[P]> : T[P];
};
` + "```",
			want: 1,
		},
		{
			// A9: "keyof T" with extra internal whitespace is the same
			// requirement, just formatted differently.
			name: "correct recursive DeepPartial with extra whitespace around keyof T (A9 bug probe)",
			response: "```ts\n" + `type DeepPartial<T> = {
  [K in keyof   T]?: T[K] extends object ? DeepPartial<T[K]> : T[K];
};
` + "```",
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

// tsPromiseAllSettledTraceJS is a plain-JavaScript mirror of
// tsPromiseAllSettledTraceCode.
const tsPromiseAllSettledTraceJS = `async function main() {
  const results = await Promise.allSettled([
    Promise.resolve(1),
    Promise.reject("fail"),
    Promise.resolve(3),
  ]);
  console.log(JSON.stringify(results.map((r) => r.status)));
}

main();`

func TestTsPromiseAllSettledTrace_GroundTruth(t *testing.T) {
	got := runNodeJS(t, tsPromiseAllSettledTraceJS)
	if want := `["fulfilled","rejected","fulfilled"]`; got != want {
		t.Errorf("node output = %q, want %q", got, want)
	}
}

func TestTsPromiseAllSettledTraceTest_Eval(t *testing.T) {
	tc := tsPromiseAllSettledTraceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", `["fulfilled","rejected","fulfilled"]`, 1},
		{"correct fenced json", "```json\n[\"fulfilled\", \"rejected\", \"fulfilled\"]\n```", 1},
		{"correct with prose wrapper", `The statuses are: ["fulfilled","rejected","fulfilled"]`, 1},
		{"wrong: assumes allSettled short-circuits like Promise.all", `["fulfilled"]`, 0},
		{"wrong: assumes rejection order shifts", `["rejected","fulfilled","fulfilled"]`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestTsVueRefReactiveUnwrapTest_Eval(t *testing.T) {
	tc := tsVueRefReactiveUnwrapTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", `{"object_property_unwraps": true, "array_element_unwraps": false}`, 1},
		{"correct fenced json", "```json\n{\"object_property_unwraps\": true, \"array_element_unwraps\": false}\n```", 1},
		{"correct with prose wrapper", `Answer: {"object_property_unwraps": true, "array_element_unwraps": false}`, 1},
		{"wrong: assumes both unwrap", `{"object_property_unwraps": true, "array_element_unwraps": true}`, 0.5},
		{"wrong: assumes neither unwraps", `{"object_property_unwraps": false, "array_element_unwraps": false}`, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestTsComputedVsWatchTest_Eval(t *testing.T) {
	tc := tsComputedVsWatchTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", `{"scenario_a": "computed", "scenario_b": "watch"}`, 1},
		{"correct fenced json", "```json\n{\"scenario_a\": \"computed\", \"scenario_b\": \"watch\"}\n```", 1},
		{"correct with prose wrapper", `Answer: {"scenario_a": "computed", "scenario_b": "watch"}`, 1},
		{"wrong: both watch", `{"scenario_a": "watch", "scenario_b": "watch"}`, 0.5},
		{"wrong: swapped", `{"scenario_a": "watch", "scenario_b": "computed"}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestTsESLintFlatConfigTest_Eval(t *testing.T) {
	tc := tsESLintFlatConfigTest()

	goodFlatConfig := "```js\n" + `export default [
  {
    languageOptions: { ecmaVersion: "latest", sourceType: "module" },
    rules: { semi: "error", "prefer-const": "error" },
  },
];
` + "```"

	goodFlatConfigWithHelper := "```js\n" + `import { defineConfig } from "eslint/config";

export default defineConfig([
  {
    languageOptions: { ecmaVersion: "latest", sourceType: "module" },
    rules: { semi: "error", "prefer-const": "error" },
  },
]);
` + "```"

	goodFlatConfigCommonJS := "```js\n" + `module.exports = [
  {
    languageOptions: { ecmaVersion: "latest", sourceType: "module" },
    rules: { semi: "error", "prefer-const": "error" },
  },
];
` + "```"

	stillLegacyFormat := "```json\n" + tsOldEslintrcConfig + "\n```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"good: plain array export default", goodFlatConfig, 1},
		{"good: defineConfig helper", goodFlatConfigWithHelper, 1},
		{"good: CommonJS module.exports form is equally valid flat config (A8 bug probe)", goodFlatConfigCommonJS, 1},
		{"wrong: left as legacy .eslintrc JSON", stillLegacyFormat, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestTsTsconfigStrictFlagTest_Eval(t *testing.T) {
	tc := tsTsconfigStrictFlagTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", `{"flag": "noImplicitAny"}`, 1},
		{"correct fenced json", "```json\n{\"flag\": \"noImplicitAny\"}\n```", 1},
		{"correct with prose wrapper", `The answer is: {"flag": "noImplicitAny"}`, 1},
		{"wrong: unrelated strict flag", `{"flag": "strictNullChecks"}`, 0},
		{"wrong: not a real tsconfig flag", `{"flag": "strict-types"}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

// tsArrayMethodChainJS is a plain-JavaScript mirror of
// tsArrayMethodChainCode.
const tsArrayMethodChainJS = `const nums = [3, 8, 1, 9, 4, 6, 2, 7];
const result = nums
  .filter((n) => n % 2 === 0)
  .map((n) => n * n)
  .reduce((acc, n) => acc + n, 0);
console.log(result);`

func TestTsArrayMethodChain_GroundTruth(t *testing.T) {
	got := runNodeJS(t, tsArrayMethodChainJS)
	if want := "120"; got != want {
		t.Errorf("node output = %q, want %q", got, want)
	}
}

func TestTsArrayMethodChainTraceTest_Eval(t *testing.T) {
	tc := tsArrayMethodChainTraceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "120", 1},
		{"correct with prose", "The result is 120.", 1},
		{"correct with label", "Output: 120", 1},
		{"wrong: filters odd instead of even", "30", 0},
		{"wrong: forgets to square before summing", "20", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

// tsOptionalChainingNullishJS is a plain-JavaScript mirror of
// tsOptionalChainingNullishCode (TypeScript's type annotations erased).
const tsOptionalChainingNullishJS = `function describe(c) {
  const host = c.server?.host ?? "localhost";
  const port = c.server?.port ?? 8080;
  return ` + "`${host}:${port}`" + `;
}

console.log(describe({ server: { port: 0 } }));`

func TestTsOptionalChainingNullishTrace_GroundTruth(t *testing.T) {
	got := runNodeJS(t, tsOptionalChainingNullishJS)
	if want := "localhost:0"; got != want {
		t.Errorf("node output = %q, want %q", got, want)
	}
}

func TestTsOptionalChainingNullishTraceTest_Eval(t *testing.T) {
	tc := tsOptionalChainingNullishTraceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact correct", "localhost:0", 1},
		{"correct with surrounding whitespace", "  localhost:0  ", 1},
		{"correct case-insensitive", "LOCALHOST:0", 1},
		// A7 regression: fenced/quoted decoration on the correct answer
		// must still score full credit.
		{"fenced", "```\nlocalhost:0\n```", 1},
		{"quoted with trailing period", `"localhost:0".`, 1},
		{"wrong: treats ?? like || and replaces the falsy 0", "localhost:8080", 0},
		{"wrong: assumes host is present", "example.com:0", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestTsDebounceComposableTest_Eval(t *testing.T) {
	tc := tsDebounceComposableTest()

	badMissingCleanup := "```ts\n" + `export function useDebouncedRef<T>(source: Ref<T>, ms: number): Ref<T> {
	const debounced = ref(source.value)
	watch(source, (value) => {
		setTimeout(() => { debounced.value = value }, ms)
	})
	return debounced
}
` + "```"

	badUsesAny := "```ts\n" + `export function useDebouncedRef<T>(source: any, ms: number): any {
	const debounced = ref(source.value)
	let timer: ReturnType<typeof setTimeout> | undefined
	watch(source, (value: any) => {
		if (timer) clearTimeout(timer)
		timer = setTimeout(() => { debounced.value = value }, ms)
	})
	return debounced
}
` + "```"

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct composable scores 1", goodDebounceComposable, 1},
		{"missing cleanup loses cleanup weight", badMissingCleanup, 0.75}, // (2*1 + 1*0 + 1*1)/4
		{"uses any loses guard weight", badUsesAny, 0.75},                 // (2*1 + 1*1 + 1*0)/4
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate() = %v, want %v (detail: %s)", got.Value, tt.want, got.Detail)
			}
		})
	}
}

package tests

import (
	"context"
	"regexp"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerTypeScriptTests(r *testkit.Registry) {
	r.Register(tsDebounceComposableTest())
	r.Register(tsDiscriminatedUnionAreaTest())
	r.Register(tsGenericUtilityTypeTest())
	r.Register(tsPromiseAllSettledTraceTest())
	r.Register(tsVueRefReactiveUnwrapTest())
	r.Register(tsComputedVsWatchTest())
	r.Register(tsESLintFlatConfigTest())
	r.Register(tsTsconfigStrictFlagTest())
	r.Register(tsArrayMethodChainTraceTest())
	r.Register(tsOptionalChainingNullishTraceTest())
}

// tsAnyEscapeHatchPattern matches TypeScript's `any` used as a type: a type
// annotation (": any"), a sole generic argument ("<any>"), a non-first
// generic argument ("Map<string, any>"), a cast ("as any"), or an array
// type ("any[]"). A plain \bany\b substring check would false-positive on
// prose words like "many" or "Company", so this is intentionally more
// specific than eval.NotContains's plain substring matching.
var tsAnyEscapeHatchPattern = regexp.MustCompile(`(?i):\s*any\b|<\s*any\s*>|,\s*any\s*>|\bas\s+any\b|\bany\s*\[\]`)

// noAnyEscapeHatch scores full credit unless the response uses TypeScript's
// `any` type as an escape hatch.
func noAnyEscapeHatch() eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		if tsAnyEscapeHatchPattern.MatchString(response) {
			return eval.Score{Value: 0, Detail: "uses the `any` escape hatch"}
		}
		return eval.Score{Value: 1, Detail: "properly typed, no `any`"}
	})
}

// tsDebounceComposableTest: write a properly typed, generic, cleaned-up
// Vue 3 debounce composable.
func tsDebounceComposableTest() testkit.Test {
	prompt := `Write a Vue 3 composable with this exact signature:

` + "```ts" + `
export function useDebouncedRef<T>(source: Ref<T>, ms: number): Ref<T>
` + "```" + `

Requirements:
- Returns a new ref whose value updates to match source, but only after ms
  milliseconds have passed with no further changes to source (debounced).
- Watches source for changes (use Vue's watch API).
- Cleans up any pending timer, both between debounce cycles and when the
  owning component scope is torn down (e.g. via onScopeDispose).
- Fully typed: do not use the "any" type anywhere.
- Import anything you need from "vue".`

	evaluator := eval.All(
		eval.W(eval.ContainsAll("export function useDebouncedRef", "watch", "<T>"), 2),
		eval.W(eval.ContainsAny("clearTimeout", "onScopeDispose"), 1),
		eval.W(noAnyEscapeHatch(), 1),
	)

	return testkit.Test{
		ID:          "ts-debounce-composable",
		Category:    "programming",
		Subcategory: "typescript",
		Description: "Write a properly typed, generic Vue 3 debounced-ref composable with cleanup.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		MaxTokens:   600,
		Eval:        evaluator,
	}
}

// tsDiscriminatedUnionAreaCode is the inline snippet for
// tsDiscriminatedUnionAreaTest: a discriminated union narrowed with a
// switch on its "kind" tag.
const tsDiscriminatedUnionAreaCode = `type Shape =
  | { kind: "circle"; radius: number }
  | { kind: "rectangle"; width: number; height: number };

function area(s: Shape): number {
  switch (s.kind) {
    case "circle":
      return Math.PI * s.radius ** 2;
    case "rectangle":
      return s.width * s.height;
  }
}

const shapes: Shape[] = [
  { kind: "rectangle", width: 4, height: 5 },
  { kind: "circle", radius: 3 },
];

const total = shapes.map(area).reduce((a, b) => a + b, 0);
console.log(Math.round(total * 100) / 100);`

// tsDiscriminatedUnionAreaTest: trace the exact output of code that narrows
// a discriminated union in a switch and sums the per-branch results.
//
// ground truth: switching on the "kind" tag narrows each branch to its
// specific member of the Shape union, so s.radius is only accessible in the
// "circle" case and s.width/s.height only in "rectangle". area({kind:
// "rectangle", width: 4, height: 5}) = 4*5 = 20. area({kind: "circle",
// radius: 3}) = pi*3^2 = 28.274333882308138. Their sum, rounded to 2
// decimal places (Math.round(total*100)/100), is 48.27. typescript_test.go
// cross-checks this by running the equivalent JavaScript (the same
// arithmetic, with TypeScript's type annotations erased, since switch-based
// narrowing is a compile-time-only construct with no runtime effect on
// these values) via `node`, guarded by node's presence.
func tsDiscriminatedUnionAreaTest() testkit.Test {
	prompt := `Here is a TypeScript program:

` + "```ts" + `
` + tsDiscriminatedUnionAreaCode + `
` + "```" + `

What number does this program print? Respond with only the number, nothing
else.`

	return testkit.Test{
		ID:          "ts-discriminated-union-narrowing",
		Category:    "programming",
		Subcategory: "typescript",
		Description: "Trace the exact output of a discriminated union narrowed with a switch, summed across variants.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[float64], 48.27, 0.005),
	}
}

// tsGenericUtilityTypeTest: author a recursive generic utility type
// (DeepPartial<T>) using a mapped type and a conditional type, without any.
func tsGenericUtilityTypeTest() testkit.Test {
	prompt := `Write a generic TypeScript utility type ` + "`DeepPartial<T>`" + ` that
makes T and all of its nested object properties optional, recursively (so
given an interface with a nested object property, that nested object's own
properties are also optional). Do not use the "any" type anywhere. Respond
with only the type definition.`

	evaluator := eval.All(
		eval.W(eval.ContainsAll("DeepPartial<T>", "keyof T"), 2),
		eval.W(eval.ContainsAny("DeepPartial<T[K]>"), 1),
		eval.W(noAnyEscapeHatch(), 1),
	)

	return testkit.Test{
		ID:          "ts-generic-utility-type",
		Category:    "programming",
		Subcategory: "typescript",
		Description: "Author a recursive generic DeepPartial<T> utility type using a mapped and conditional type, without any.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// tsPromiseAllSettledTraceCode is the inline snippet for
// tsPromiseAllSettledTraceTest.
const tsPromiseAllSettledTraceCode = `async function main() {
  const results = await Promise.allSettled([
    Promise.resolve(1),
    Promise.reject("fail"),
    Promise.resolve(3),
  ]);
  console.log(results.map((r) => r.status));
}

main();`

// tsPromiseAllSettledTraceTest: trace the exact "status" values
// Promise.allSettled reports for a mix of resolved and rejected promises.
//
// ground truth: unlike Promise.all, Promise.allSettled never short-circuits
// on a rejection - it waits for every input promise to settle and reports
// each one's outcome as {status: "fulfilled", value} or {status: "rejected",
// reason}, in the same order as the input array. The three inputs settle as
// fulfilled, rejected, fulfilled respectively (the middle one rejects with
// "fail"), so results.map(r => r.status) is
// ["fulfilled", "rejected", "fulfilled"]. typescript_test.go cross-checks
// this by running the equivalent JavaScript via `node`.
func tsPromiseAllSettledTraceTest() testkit.Test {
	prompt := `Here is a TypeScript program:

` + "```ts" + `
` + tsPromiseAllSettledTraceCode + `
` + "```" + `

What array of statuses does this program log? Respond with only a JSON
array of the three status strings in order, for example
["fulfilled","fulfilled","fulfilled"]`

	return testkit.Test{
		ID:          "ts-promise-allsettled-trace",
		Category:    "programming",
		Subcategory: "typescript",
		Description: "Trace the exact per-promise status array Promise.allSettled reports for a mixed resolve/reject batch.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals([]string{"fulfilled", "rejected", "fulfilled"}),
	}
}

// tsVueRefReactiveUnwrapTest: decide whether Vue 3's ref-auto-unwrapping
// applies to a property access on a reactive object versus an element
// access on a reactive array.
//
// ground truth: Vue 3's reactive() unwraps a nested ref when it is accessed
// as a property of the reactive object (state.count reads/writes as a plain
// number, not a Ref), but explicitly does NOT unwrap a ref when it is
// accessed as an element of a reactive array or a native collection like
// Map (list[0] stays a Ref object). Verified empirically for this test (not
// from memory): `reactive({count: ref(0)}).count` has typeof "number" and
// isRef() false, while `reactive([ref(0)])[0]` has isRef() true, using
// vue@3.5.41's actual reactivity runtime.
func tsVueRefReactiveUnwrapTest() testkit.Test {
	prompt := `In Vue 3's Composition API:

` + "```ts" + `
const state = reactive({ count: ref(0) });
const list = reactive([ref(0)]);
` + "```" + `

Does accessing ` + "`state.count`" + ` (a ref nested as a property of a
reactive object) auto-unwrap to a plain number? Does accessing
` + "`list[0]`" + ` (a ref nested as an element of a reactive array)
auto-unwrap to a plain number? Respond with only a JSON object:
{"object_property_unwraps": true|false, "array_element_unwraps": true|false}`

	evaluator := eval.All(
		eval.W(eval.JSONField("object_property_unwraps", true), 1),
		eval.W(eval.JSONField("array_element_unwraps", false), 1),
	)

	return testkit.Test{
		ID:          "ts-vue-ref-reactive-unwrap",
		Category:    "programming",
		Subcategory: "typescript",
		Description: "Decide whether Vue 3 ref auto-unwrapping applies to a reactive object property versus a reactive array element.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// tsComputedVsWatchChoice is the fixed vocabulary tsComputedVsWatchTest
// allows for each scenario's answer.
const (
	tsComputedVsWatchChoiceComputed = "computed"
	tsComputedVsWatchChoiceWatch    = "watch"
)

// tsComputedVsWatchTest: choose computed() versus watch() for two Vue 3
// Composition API scenarios, one pure derivation and one side effect.
//
// ground truth: per Vue's own guidance, computed() is for deriving a value
// from reactive state with no side effects - Vue's docs recommend it
// "whenever possible" for exactly this - while watch() is for running a
// side effect (an API call, a DOM mutation, a storage write, logging) in
// reaction to a state change. Scenario A only derives a formatted string
// for template display (no side effect) -> computed. Scenario B persists a
// value to localStorage whenever it changes (a side effect) -> watch.
func tsComputedVsWatchTest() testkit.Test {
	prompt := `Two Vue 3 Composition API scenarios:

Scenario A: derive a formatted currency string from a "price" ref, used
only for display in the template. Deriving it has no side effects.

Scenario B: whenever a "settings" ref changes, persist the new value to
localStorage. This has a side effect (writing to storage).

For each scenario, should you use computed() or watch()? Respond with only
a JSON object: {"scenario_a": "computed"|"watch", "scenario_b": "computed"|"watch"}`

	evaluator := eval.All(
		eval.W(eval.JSONField("scenario_a", tsComputedVsWatchChoiceComputed), 1),
		eval.W(eval.JSONField("scenario_b", tsComputedVsWatchChoiceWatch), 1),
	)

	return testkit.Test{
		ID:          "ts-computed-vs-watch",
		Category:    "programming",
		Subcategory: "typescript",
		Description: "Choose computed() versus watch() for a pure-derivation scenario and a side-effect scenario.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// tsOldEslintrcConfig is the inline legacy config tsESLintFlatConfigTest
// asks the model to migrate.
const tsOldEslintrcConfig = `{
  "env": { "browser": true, "es2021": true },
  "extends": ["eslint:recommended"],
  "parserOptions": { "ecmaVersion": "latest", "sourceType": "module" },
  "rules": { "semi": "error", "prefer-const": "error" }
}`

// tsESLintFlatConfigTest: migrate a legacy .eslintrc.json to ESLint v9's
// flat config format.
func tsESLintFlatConfigTest() testkit.Test {
	prompt := `Here is a legacy .eslintrc.json:

` + "```json" + `
` + tsOldEslintrcConfig + `
` + "```" + `

ESLint v9 uses the flat config format (eslint.config.js) by default; the
legacy .eslintrc format is no longer picked up automatically. Rewrite this
configuration as an eslint.config.js flat config, preserving the same rules
(semi: error, prefer-const: error) and the ecmaVersion/sourceType settings
via languageOptions. Respond with only the eslint.config.js file content.`

	evaluator := eval.All(
		eval.W(eval.ContainsAll("export default"), 2),
		eval.W(eval.ContainsAny("defineConfig(", "languageOptions"), 1),
	)

	return testkit.Test{
		ID:          "ts-eslint-flat-config",
		Category:    "programming",
		Subcategory: "typescript",
		Description: "Migrate a legacy .eslintrc.json to ESLint v9's flat eslint.config.js format.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// tsImplicitAnyBugCode is the inline snippet for tsTsconfigStrictFlagTest:
// a function whose parameter has no type annotation.
const tsImplicitAnyBugCode = `function greet(name) {
  return "Hello, " + name.toUpperCase();
}`

// tsTsconfigStrictFlagTest: identify which tsconfig strict-family compiler
// flag would catch an implicit-any parameter at compile time.
//
// ground truth: `name` has no type annotation and TypeScript cannot infer
// one from usage, so under the "noImplicitAny" compiler option (part of the
// "strict" family, also implied by "strict": true) the compiler reports
// "Parameter 'name' implicitly has an 'any' type." Verified empirically for
// this test (not from memory): compiling this exact snippet with
// typescript@5's tsc --noImplicitAny reports that error (exit code 2);
// without the flag it compiles cleanly (exit code 0).
func tsTsconfigStrictFlagTest() testkit.Test {
	prompt := `Here is a TypeScript function:

` + "```ts" + `
` + tsImplicitAnyBugCode + `
` + "```" + `

This compiles without error under TypeScript's default (non-strict)
settings, but name has no type annotation and none can be inferred from its
usage. Which single tsconfig strict-family compiler option, when enabled,
makes the compiler reject this with an error? Respond with only a JSON
object: {"flag": "<the exact tsconfig option name>"}`

	return testkit.Test{
		ID:          "ts-tsconfig-strict-flag",
		Category:    "programming",
		Subcategory: "typescript",
		Description: "Identify the tsconfig strict-family flag that catches an implicit-any function parameter.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		Eval:        eval.JSONField("flag", "noImplicitAny"),
	}
}

// tsArrayMethodChainCode is the inline snippet for
// tsArrayMethodChainTraceTest.
const tsArrayMethodChainCode = `const nums = [3, 8, 1, 9, 4, 6, 2, 7];
const result = nums
  .filter((n) => n % 2 === 0)
  .map((n) => n * n)
  .reduce((acc, n) => acc + n, 0);
console.log(result);`

// tsArrayMethodChainTraceTest: trace the exact numeric result of a
// filter/map/reduce chain over an inline array.
//
// ground truth: filter keeps the even numbers, in original order: 8, 4, 6,
// 2. map squares each: 64, 16, 36, 4. reduce sums them with an initial
// accumulator of 0: 64+16+36+4 = 120. typescript_test.go cross-checks this
// by running the equivalent JavaScript via `node`.
func tsArrayMethodChainTraceTest() testkit.Test {
	prompt := `Here is a TypeScript program:

` + "```ts" + `
` + tsArrayMethodChainCode + `
` + "```" + `

What number does this program print? Respond with only the number, nothing
else.`

	return testkit.Test{
		ID:          "ts-array-method-chain",
		Category:    "programming",
		Subcategory: "typescript",
		Description: "Trace the exact output of a filter/map/reduce chain over an inline number array.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], 120, 0),
	}
}

// tsOptionalChainingNullishCode is the inline snippet for
// tsOptionalChainingNullishTraceTest.
const tsOptionalChainingNullishCode = `type Config = { server?: { host?: string; port?: number } };

function describe(c: Config): string {
  const host = c.server?.host ?? "localhost";
  const port = c.server?.port ?? 8080;
  return ` + "`${host}:${port}`" + `;
}

console.log(describe({ server: { port: 0 } }));`

// tsOptionalChainingNullishTraceTest: trace the exact output of code
// combining optional chaining with nullish coalescing, on an input whose
// present value is falsy but not nullish.
//
// ground truth: c.server?.host is undefined (server has no host property),
// so ?? falls back to "localhost". c.server?.port is 0 - present and not
// null/undefined - so ?? does NOT fall back (unlike ||, ?? only treats
// null/undefined as "missing", not other falsy values like 0, "", or
// false), leaving port as 0. The template literal produces "localhost:0".
// typescript_test.go cross-checks this by running the equivalent
// JavaScript via `node`.
func tsOptionalChainingNullishTraceTest() testkit.Test {
	prompt := `Here is a TypeScript program:

` + "```ts" + `
` + tsOptionalChainingNullishCode + `
` + "```" + `

What does this program print when run? Respond with only the exact text
printed, nothing else.`

	return testkit.Test{
		ID:          "ts-optional-chaining-nullish",
		Category:    "programming",
		Subcategory: "typescript",
		Description: "Trace the exact output of optional chaining combined with nullish coalescing on a falsy-but-present value.",
		Prompt:      prompt,
		Eval:        eval.Equals("localhost:0"),
	}
}

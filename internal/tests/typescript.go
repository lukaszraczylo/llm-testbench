package tests

import (
	"context"
	"regexp"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerTypeScriptTests(r *testkit.Registry) {
	r.Register(tsDebounceComposableTest())
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

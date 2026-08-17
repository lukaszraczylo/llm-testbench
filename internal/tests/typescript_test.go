package tests

import (
	"context"
	"testing"
)

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

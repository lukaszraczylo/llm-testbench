package tests

import (
	"context"
	"testing"
)

func TestDBRedisStructureChoiceTest_Eval(t *testing.T) {
	tc := dbRedisStructureChoiceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "all correct",
			response: `{"a":"sorted-set","b":"hyperloglog","c":"hash","d":"list"}`,
			want:     1,
		},
		{
			name:     "all correct fenced with prose",
			response: "Here is my answer:\n```json\n{\"a\":\"sorted-set\",\"b\":\"hyperloglog\",\"c\":\"hash\",\"d\":\"list\"}\n```",
			want:     1,
		},
		{
			name:     "one wrong",
			response: `{"a":"set","b":"hyperloglog","c":"hash","d":"list"}`,
			want:     0.75,
		},
		{
			name:     "all wrong",
			response: `{"a":"list","b":"set","c":"string","d":"stream"}`,
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDBRedisTTLEvictionTest_Eval(t *testing.T) {
	tc := dbRedisTTLEvictionTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"bare correct", "reject", 1},
		{"quoted correct", `"reject"`, 1},
		{"different case", "REJECT", 1},
		{"trailing period", "reject.", 1},
		{"wrong: evict", "evict", 0},
		{"wrong: claims silent success", "succeed", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDBRedisIncrAtomicityTest_Eval(t *testing.T) {
	tc := dbRedisIncrAtomicityTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"incr_final":100,"getset_race_safe":false}`, 1},
		{"correct fenced", "```json\n{\"incr_final\":100,\"getset_race_safe\":false}\n```", 1},
		{"wrong incr_final", `{"incr_final":99,"getset_race_safe":false}`, 0.5},
		{"wrongly claims getset is safe", `{"incr_final":100,"getset_race_safe":true}`, 0.5},
		{"both wrong", `{"incr_final":50,"getset_race_safe":true}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDBRedisMultiVsPipelineTest_Eval(t *testing.T) {
	tc := dbRedisMultiVsPipelineTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"bare correct", "no", 1},
		{"quoted correct", `"no"`, 1},
		{"different case", "No.", 1},
		{"wrong", "yes", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestDBNoBareKeysInProd exercises the negation-aware guard directly,
// mirroring TestNoLiveKubectlMutation's coverage: an unnegated instruction
// to run KEYS scores 0, but a response that mentions KEYS only to warn
// against it (however it is phrased or wrapped) scores 1.
func TestDBNoBareKeysInProd(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "no mention at all",
			response: "Use SCAN with a cursor and MATCH pattern instead, iterating in small batches.",
			want:     1,
		},
		{
			name:     "negated mid-sentence mention",
			response: "This runs against production, so do not use `KEYS` here - it blocks the whole server.",
			want:     1,
		},
		{
			name:     "imperative command at line start",
			response: "Just run this:\nKEYS *",
			want:     0,
		},
		{
			name:     "unnegated mid-sentence mention",
			response: "One quick option is to just run KEYS * to list them all.",
			want:     0,
		},
		{
			name:     "negation cue too far away to count",
			response: "Never mind the migration for now, but you could casually run KEYS * to check this quickly.",
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dbNoBareKeysInProd().Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("dbNoBareKeysInProd().Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDBRedisScanVsKeysTest_Eval(t *testing.T) {
	tc := dbRedisScanVsKeysTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: warns against KEYS, recommends SCAN",
			response: "Avoid running `KEYS *` in production - it blocks the entire server while it scans the whole keyspace, causing latency spikes for every other client. Use SCAN with a cursor and small COUNT instead, iterating incrementally.",
			want:     1,
		},
		{
			name:     "wrong: recommends running KEYS unnegated",
			response: "Just run KEYS * - it will return every matching key.",
			want:     0,
		},
		{
			name:     "partial: correctly avoids KEYS but never names SCAN",
			response: "Do not run that command against production; iterate the keyspace incrementally instead.",
			want:     0.5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDBRedisPubSubDeliveryTest_Eval(t *testing.T) {
	tc := dbRedisPubSubDeliveryTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"bare correct", "no", 1},
		{"quoted correct", `"no"`, 1},
		{"wrong", "yes", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDBRedisLuaAtomicityTest_Eval(t *testing.T) {
	tc := dbRedisLuaAtomicityTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"bare correct", "no", 1},
		{"trailing period", "no.", 1},
		{"wrong", "yes", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

// TestDBRedisMemoryEstimateWant_GroundTruth independently re-derives the
// total-bytes estimate with plain arithmetic, not via the
// dbRedisMemoryEstimateWant constant's own expression.
func TestDBRedisMemoryEstimateWant_GroundTruth(t *testing.T) {
	const count, keyBytes, valueBytes, overheadBytes = 200000, 20, 100, 56
	want := count * (keyBytes + valueBytes + overheadBytes)

	if want != 35200000 {
		t.Fatalf("independently recomputed total bytes = %d, want 35200000", want)
	}
	if dbRedisMemoryEstimateWant != want {
		t.Errorf("dbRedisMemoryEstimateWant = %d, independently recomputed = %d", dbRedisMemoryEstimateWant, want)
	}
}

func TestDBRedisMemoryEstimateTest_Eval(t *testing.T) {
	tc := dbRedisMemoryEstimateTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "35200000", 1},
		{"prose wrapped", "The estimate is about 35200000 bytes.", 1},
		{"wrong: forgot overhead term", "24000000", 0},
		{"wrong: used wrong overhead", "32000000", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDBRedisCacheStampedeTest_Eval(t *testing.T) {
	tc := dbRedisCacheStampedeTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "exact correct set",
			response: `["mutex_lock_on_regenerate", "probabilistic_early_expiration", "stale_while_revalidate"]`,
			want:     1,
		},
		{
			name:     "correct set, different order, fenced",
			response: "```json\n[\"stale_while_revalidate\", \"mutex_lock_on_regenerate\", \"probabilistic_early_expiration\"]\n```",
			want:     1,
		},
		{
			name:     "includes the non-mitigation",
			response: `["mutex_lock_on_regenerate", "probabilistic_early_expiration", "stale_while_revalidate", "increase_replica_count"]`,
			want:     0.75,
		},
		{
			name:     "only the non-mitigation",
			response: `["increase_replica_count"]`,
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

func TestDBRedisKeyspaceAntiPatternTest_Eval(t *testing.T) {
	tc := dbRedisKeyspaceAntiPatternTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"problem":"monolithic-key","fix":"hash-per-user"}`, 1},
		{"correct fenced with prose", "Here's the issue:\n```json\n{\"problem\":\"monolithic-key\",\"fix\":\"hash-per-user\"}\n```", 1},
		{"wrong problem", `{"problem":"missing-ttl","fix":"hash-per-user"}`, 0.5},
		{"wrong fix", `{"problem":"monolithic-key","fix":"add-ttl"}`, 0.5},
		{"both wrong", `{"problem":"too-many-small-keys","fix":"single-key-ok"}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.Eval.Evaluate(context.Background(), tt.response)
			if got.Value != tt.want {
				t.Errorf("Eval.Evaluate(%q) = %v, want %v (detail: %s)", tt.response, got.Value, tt.want, got.Detail)
			}
		})
	}
}

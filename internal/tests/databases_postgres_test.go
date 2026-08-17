package tests

import (
	"context"
	"testing"
)

func TestDBPGExplainSeqVsIndexTest_Eval(t *testing.T) {
	tc := dbPGExplainSeqVsIndexTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"reason":"low-selectivity","would_index_help_at_rare_status":true}`, 1},
		{"correct fenced with prose", "Here you go:\n```json\n{\"reason\":\"low-selectivity\",\"would_index_help_at_rare_status\":true}\n```", 1},
		{"wrong reason: blames missing index", `{"reason":"missing-index","would_index_help_at_rare_status":true}`, 0.5},
		{"wrong: says rare-status index would not help", `{"reason":"low-selectivity","would_index_help_at_rare_status":false}`, 0.5},
		{"both wrong", `{"reason":"stale-statistics","would_index_help_at_rare_status":false}`, 0},
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

func TestDBPGIndexChoiceTest_Eval(t *testing.T) {
	tc := dbPGIndexChoiceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `["region", "order_date"]`, 1},
		{"correct fenced", "```json\n[\"region\", \"order_date\"]\n```", 1},
		{"reversed order", `["order_date", "region"]`, 0},
		{"missing a column", `["region"]`, 0},
		{"wrong column entirely", `["region", "amount"]`, 0},
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

func TestDBPGNPlusOneTest_Eval(t *testing.T) {
	tc := dbPGNPlusOneTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"pattern":"n-plus-one","fix":"single-join-query"}`, 1},
		{"correct, different key order", `{"fix":"single-join-query","pattern":"n-plus-one"}`, 1},
		{"wrong pattern", `{"pattern":"cartesian-product","fix":"single-join-query"}`, 0.5},
		{"wrong fix", `{"pattern":"n-plus-one","fix":"add-cache"}`, 0.5},
		{"both wrong", `{"pattern":"missing-index","fix":"add-index"}`, 0},
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

func TestDBPGIsolationAnomalyTest_Eval(t *testing.T) {
	tc := dbPGIsolationAnomalyTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"anomaly":"non-repeatable-read"}`, 1},
		{"correct, uppercase value", `{"anomaly":"Non-Repeatable-Read"}`, 1},
		{"wrong: dirty read", `{"anomaly":"dirty-read"}`, 0},
		{"wrong: phantom read", `{"anomaly":"phantom-read"}`, 0},
		{"wrong: lost update", `{"anomaly":"lost-update"}`, 0},
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

func TestDBPGDeadlockLockOrderTest_Eval(t *testing.T) {
	tc := dbPGDeadlockLockOrderTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"deadlock":true,"safe_order_ids":[1,2]}`, 1},
		{"correct fenced with prose", "Yes, they deadlock.\n```json\n{\"deadlock\":true,\"safe_order_ids\":[1,2]}\n```", 1},
		{"says no deadlock", `{"deadlock":false,"safe_order_ids":[1,2]}`, 2.0 / 3.0},
		{"reversed safe order", `{"deadlock":true,"safe_order_ids":[2,1]}`, 1.0 / 3.0},
		{"everything wrong", `{"deadlock":false,"safe_order_ids":[2,1]}`, 0},
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

func TestDBPGReplicaFailoverTest_Eval(t *testing.T) {
	tc := dbPGReplicaFailoverTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct: names data loss and async cause",
			response: "Because replication is asynchronous, that commit may be lost - the replica never received its WAL before promotion.",
			want:     1,
		},
		{
			name:     "correct: alternate phrasing",
			response: "No, it is not guaranteed to be visible; async replication means the promoted replica can be missing it.",
			want:     1,
		},
		{
			name:     "wrong: claims it is guaranteed durable",
			response: "Yes, Postgres guarantees every committed transaction is always visible after failover.",
			want:     0,
		},
		{
			name:     "partial: mentions loss but not the async cause",
			response: "That transaction can be lost after the failover.",
			want:     2.0 / 3.0,
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

func TestDBPGBloatVacuumTest_Eval(t *testing.T) {
	tc := dbPGBloatVacuumTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"cause":"bloat","action":"vacuum"}`, 1},
		{"correct fenced", "```json\n{\"cause\":\"bloat\",\"action\":\"vacuum\"}\n```", 1},
		{"wrong cause", `{"cause":"missing-index","action":"vacuum"}`, 0.5},
		{"wrong action", `{"cause":"bloat","action":"reindex"}`, 0.5},
		{"both wrong", `{"cause":"replication-lag","action":"add-index"}`, 0},
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

// TestDBPGPoolSizingWant_GroundTruth independently re-derives pool_size
// with plain arithmetic, not via the dbPGPoolSizingWant constant's own
// expression.
func TestDBPGPoolSizingWant_GroundTruth(t *testing.T) {
	const coreCount = 8
	const effectiveSpindleCount = 1
	want := coreCount*2 + effectiveSpindleCount

	if want != 17 {
		t.Fatalf("independently recomputed pool_size = %d, want 17", want)
	}
	if dbPGPoolSizingWant != want {
		t.Errorf("dbPGPoolSizingWant = %d, independently recomputed = %d", dbPGPoolSizingWant, want)
	}
}

func TestDBPGPoolSizingTest_Eval(t *testing.T) {
	tc := dbPGPoolSizingTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "17", 1},
		{"prose wrapped", "pool_size = 17", 1},
		{"wrong: forgot spindle term", "16", 0},
		{"wrong: doubled instead of adding", "18", 0},
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

func TestDBPGPartialIndexTest_Eval(t *testing.T) {
	tc := dbPGPartialIndexTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"partial_index_applicable":true,"reason":"small-frequently-queried-subset"}`, 1},
		{"correct fenced with prose", "Yes.\n```json\n{\"partial_index_applicable\":true,\"reason\":\"small-frequently-queried-subset\"}\n```", 1},
		{"wrong: says not applicable", `{"partial_index_applicable":false,"reason":"small-frequently-queried-subset"}`, 0.5},
		{"wrong reason", `{"partial_index_applicable":true,"reason":"subset-too-large"}`, 0.5},
		{"both wrong", `{"partial_index_applicable":false,"reason":"query-does-not-filter-on-column"}`, 0},
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

func TestDBPGListenNotifyTest_Eval(t *testing.T) {
	tc := dbPGListenNotifyTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", "Use LISTEN/NOTIFY.", 1},
		{"correct, spelled out", "Postgres's LISTEN and NOTIFY commands let a worker block until notified.", 1},
		{"only names one half", "Use LISTEN.", 0.5},
		{"wrong: recommends polling harder", "Just poll more frequently, e.g. every 100ms.", 0},
		{"wrong: recommends a trigger with no LISTEN/NOTIFY", "Add a trigger that writes to an audit table.", 0},
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

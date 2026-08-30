package tests

import (
	"context"
	"testing"

	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

// TestRegisterScenarioTests_Wiring calls registerScenarioTests into a fresh
// registry and checks it contributes a full operations/scenario layer with
// scen-* IDs, mirroring the wiring-test pattern used for round-2
// categories. It also keeps registerScenarioTests referenced so it is not
// reported as dead code while the parent still wires catalog integration.
func TestRegisterScenarioTests_Wiring(t *testing.T) {
	r := testkit.NewRegistry()
	registerScenarioTests(r)

	if n := r.Len(); n < 10 || n > 12 {
		t.Fatalf("registerScenarioTests registered %d tests, want 10-12", n)
	}
	for _, tc := range r.All() {
		if tc.Category != "operations" {
			t.Errorf("test %s has Category %q, want operations", tc.ID, tc.Category)
		}
		if tc.Subcategory != "scenario" {
			t.Errorf("test %s has Subcategory %q, want scenario", tc.ID, tc.Subcategory)
		}
		if len(tc.ID) < 5 || tc.ID[:5] != "scen-" {
			t.Errorf("test %s has an ID not prefixed scen-", tc.ID)
		}
	}
}

func TestScenDiskpressureFirstcmdTest_Eval(t *testing.T) {
	tc := scenDiskpressureFirstcmdTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct du|sort one-liner",
			response: `{"first_command":"du -xh --max-depth=1 / 2>/dev/null | sort -rh | head -20"}`,
			want:     1,
		},
		{
			name:     "correct fenced with prose",
			response: "Here is the command:\n```json\n{\"first_command\":\"du -x --max-depth=1 / 2>/dev/null | sort -h\"}\n```",
			want:     1,
		},
		{
			name:     "wrong: ls does not measure recursive usage",
			response: `{"first_command":"ls -lah /"}`,
			want:     0,
		},
		{
			name:     "wrong: destructive rm as first step",
			response: `{"first_command":"rm -rf /var/log && rm -rf /tmp"}`,
			want:     0,
		},
		{
			name:     "wrong: du but no sort",
			response: `{"first_command":"du -sh /var"}`,
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

func TestScenInodeExhaustionTest_Eval(t *testing.T) {
	tc := scenInodeExhaustionTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"cause":"inodes-exhausted","command":"df -hi"}`, 1},
		{"correct fenced", "```json\n{\"cause\":\"inodes-exhausted\",\"command\":\"df -hi\"}\n```", 1},
		{"wrong cause", `{"cause":"disk-full","command":"df -hi"}`, 0.5},
		{"wrong command", `{"cause":"inodes-exhausted","command":"df -h"}`, 0.5},
		{"both wrong", `{"cause":"quota-exceeded","command":"stat -f"}`, 0},
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

func TestScenCrashloopOrderTest_Eval(t *testing.T) {
	tc := scenCrashloopOrderTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"both steps", `["B","A"]`, 1},
		{"both steps fenced", "```json\n[\"B\",\"A\"]\n```", 1},
		{
			// The correct answer discusses the rejected options (C as an
			// unhelpful action, E as premature) at length; the JSON
			// array still governs, so the prose cannot change the score.
			name: "both steps with long discussion of rejected options",
			response: "```json\n[\"B\",\"A\"]\n```\n\n`kubectl describe pod` (B) shows the last exit reason and `kubectl logs --previous` (A) shows the actual crash output; either may come first since both are read-only. " +
				"A blind `kubectl rollout restart` (C) would not reveal the cause and `kubectl scale --replicas=0` (D) would only stop the workload; " +
				"ssh'ing to a node for journalctl (E) is node-level tooling that is premature while the container logs are local.",
			want: 1,
		},
		{"order is free", `["A","B"]`, 1},
		{"only one diagnostic step", `["A"]`, 0.5},
		// An action step in the answer costs credit: set overlap 2/3.
		{"correct plus an action step", `["B","A","D"]`, 2.0 / 3.0},
		{"all wrong", `["C","D","E"]`, 0},
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

func TestScenNetLayerIsolateTest_Eval(t *testing.T) {
	tc := scenNetLayerIsolateTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"layer":"dns"}`, 1},
		{"correct fenced", "```json\n{\"layer\":\"dns\"}\n```", 1},
		{
			name: "correct, discussing other layers at length",
			response: `{"layer":"dns"}

This is not tls or http: curl never reached the handshake because it could not resolve the name at all; the local ping to 10.0.0.7 rules out ip-routing on the segment.`,
			want: 1,
		},
		{"wrong layer", `{"layer":"tls"}`, 0},
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

func TestScenServiceDegradeTest_Eval(t *testing.T) {
	tc := scenServiceDegradeTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"action":"rollback-config"}`, 1},
		{"correct fenced", "```json\n{\"action\":\"rollback-config\"}\n```", 1},
		{
			name: "correct, discussing rejected actions at length",
			response: `{"action":"rollback-config"}

Scaling out is wrong because CPU and memory are both low - there is no resource pressure to scale for; a restart would not revert the config file; and a backup restore is wrong because there is no data loss to recover from. The regression tracks the 14:00 canary pool-size change, so reverting the config is the fix.`,
			want: 1,
		},
		{"wrong action", `{"action":"restart"}`, 0},
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

func TestScenLogRootcauseMCQTest_Eval(t *testing.T) {
	tc := scenLogRootcauseMCQTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"answer":"A"}`, 1},
		{"correct fenced", "```json\n{\"answer\":\"A\"}\n```", 1},
		{
			name: "correct, discussing every distractor at length",
			response: `{"answer":"A"}

This is not disk full (there is no out-of-space message in the log), not a slow query (no long-running query is shown), and not an authentication failure (no password-authentication-failed line) - it is the classic max_connections exhaustion signal.`,
			want: 1,
		},
		{"wrong letter", `{"answer":"C"}`, 0},
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

func TestScenBackupRestoreOrderTest_Eval(t *testing.T) {
	tc := scenBackupRestoreOrderTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct order", `["A","B","C"]`, 1},
		{"correct order fenced", "```json\n[\"A\",\"B\",\"C\"]\n```", 1},
		{
			// The correct answer is allowed to explain at length why
			// restoring last night's backup (D) first is wrong; the JSON
			// array is what is scored.
			name: "correct order, discussing rejected restore-first at length",
			response: `["A","B","C"]

Restoring last night's backup first (D) would throw away today's writes before any change is made - it is a stale snapshot, not a safety net for the migration, so I take a fresh pg_dump first.`,
			want: 1,
		},
		{"wrong: restore first", `["D","A"]`, 0},
		{"wrong: reordered", `["A","C","B"]`, 0},
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

func TestScenCertexpiryTriageTest_Eval(t *testing.T) {
	tc := scenCertexpiryTriageTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"cause":"cert-expired","check_command":"openssl-x509-enddate"}`, 1},
		{"correct fenced", "```json\n{\"cause\":\"cert-expired\",\"check_command\":\"openssl-x509-enddate\"}\n```", 1},
		{"wrong cause", `{"cause":"clock-skew","check_command":"openssl-x509-enddate"}`, 0.5},
		{"wrong command", `{"cause":"cert-expired","check_command":"date"}`, 0.5},
		{"both wrong", `{"cause":"cipher-mismatch","check_command":"openssl-cipher"}`, 0},
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

func TestScenSystemdFailreasonTest_Eval(t *testing.T) {
	tc := scenSystemdFailreasonTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"category":"missing-config"}`, 1},
		{"correct fenced", "```json\n{\"category\":\"missing-config\"}\n```", 1},
		{
			name: "correct, discussing rejected categories at length",
			response: `{"category":"missing-config"}

This is not a permission problem (no Permission denied in the journal), not a missing binary (the worker ran and printed a config-diagnostic trace), and not a port in use (no bind error) - the unit fails because config/openapi.yaml cannot be opened.`,
			want: 1,
		},
		{"wrong category", `{"category":"permission"}`, 0},
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

func TestScenPortbindFailTest_Eval(t *testing.T) {
	tc := scenPortbindFailTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct", `{"cause":"port-in-use","command":"ss -ltnp"}`, 1},
		{"correct fenced", "```json\n{\"cause\":\"port-in-use\",\"command\":\"ss -ltnp\"}\n```", 1},
		{"wrong cause", `{"cause":"config-error","command":"ss -ltnp"}`, 0.5},
		{"wrong command", `{"cause":"port-in-use","command":"lsblk"}`, 0.5},
		{"both wrong", `{"cause":"missing-binary","command":"netstat -r"}`, 0},
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

func TestScenLvmResizeOrderTest_Eval(t *testing.T) {
	tc := scenLvmResizeOrderTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct order", `["B","A","C"]`, 1},
		{"correct order fenced", "```json\n[\"B\",\"A\",\"C\"]\n```", 1},
		{"wrong: grows fs before extending LV", `["A","C","B"]`, 0},
		{"wrong: includes umount", `["B","A","C","D"]`, 0},
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

package tests

import (
	"context"
	"testing"
)

func TestLinuxPctExecTest_Eval(t *testing.T) {
	tc := linuxPctExecTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct with user prefix and double dash",
			response: `ssh root@10.0.0.100 "pct exec 251 -- systemctl status myservice"`,
			want:     1,
		},
		{
			name:     "correct without user prefix",
			response: `ssh 10.0.0.100 pct exec 251 -- systemctl status myservice`,
			want:     1,
		},
		{
			name:     "wrong ip",
			response: `ssh root@10.0.0.200 "pct exec 251 -- systemctl status myservice"`,
			want:     0,
		},
		{
			name:     "wrong container id",
			response: `ssh root@10.0.0.100 "pct exec 999 -- systemctl status myservice"`,
			want:     0,
		},
		{
			name:     "missing ssh hop, direct pct exec",
			response: `pct exec 251 -- systemctl status myservice`,
			want:     0,
		},
		{
			name:     "ssh with -t flag before user@host",
			response: `ssh -t root@10.0.0.100 "pct exec 251 -- systemctl status myservice"`,
			want:     1,
		},
		{
			name:     "ssh with multiple flags before user@host",
			response: `ssh -t -o StrictHostKeyChecking=no root@10.0.0.100 "pct exec 251 -- systemctl status myservice"`,
			want:     1,
		},
		{
			name: "scattered mention across paragraphs is not a real one-line command",
			response: `First, ssh to the Proxmox host at 10.0.0.100.

Then, separately, run pct exec 251 to enter the container.

Finally, check systemctl status myservice inside it.`,
			want: 0,
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

func TestLinuxSystemdOneshotTest_Eval(t *testing.T) {
	tc := linuxSystemdOneshotTest()

	fullUnit := `[Unit]
Description=Bootstrap script
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/bootstrap.sh
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target`

	missingRemainAfterExit := `[Unit]
Description=Bootstrap script
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/bootstrap.sh

[Install]
WantedBy=multi-user.target`

	wrongType := `[Unit]
Description=Bootstrap script
After=network.target

[Service]
Type=simple
Restart=always
ExecStart=/usr/local/bin/bootstrap.sh

[Install]
WantedBy=graphical.target`

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"full unit with RemainAfterExit scores 1", fullUnit, 1},
		{"missing RemainAfterExit loses optional weight", missingRemainAfterExit, 0.75},
		{"wrong type/target/restart-loop scores 0", wrongType, 0},
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

func TestLinuxJournalctlFilterTest_Eval(t *testing.T) {
	tc := linuxJournalctlFilterTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"quoted negative window", `journalctl -u nginx.service --since "-2 hours"`, 1},
		{"english phrasing window", `journalctl -u nginx.service --since "2 hours ago"`, 1},
		{"compact -2h form, no quotes", `Run: journalctl -u nginx.service --since -2h`, 1},
		// The unit and since checks are independent (A3): a response that
		// nails one but not the other now earns that check's half-credit
		// rather than scoring 0 outright, the same partial-credit
		// convention eval.All already uses elsewhere in this file.
		{"wrong unit still credits the correct since-window", `journalctl -u apache.service --since "-2 hours"`, 0.5},
		{"missing unit filter still credits the correct since-window", `journalctl --since "-2 hours"`, 0.5},
		{"missing both filters entirely scores 0", `journalctl -f`, 0},
		{"long-form --unit= (A3 bug probe)", `journalctl --unit=nginx.service --since=-2h`, 1},
		{"short-form -S (A3 bug probe)", `journalctl -u nginx.service -S -2h`, 1},
		{"since option before unit option, order-free (A3 bug probe)", `journalctl --since "-2 hours" -u nginx.service`, 1},
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

func TestLinuxCgroupOOMDiagnosisTest_Eval(t *testing.T) {
	tc := linuxCgroupOOMDiagnosisTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct object", `{"oom_killed": true, "oom_kill_count": 3}`, 1},
		{"prose then JSON", "Yes, it was OOM-killed.\n" + `{"oom_killed": true, "oom_kill_count": 3}`, 1},
		{"fenced JSON", "```json\n" + `{"oom_killed": true, "oom_kill_count": 3}` + "\n```", 1},
		{"confuses oom counter with oom_kill counter", `{"oom_killed": true, "oom_kill_count": 47}`, 0.5},
		{"wrongly claims no kill happened", `{"oom_killed": false, "oom_kill_count": 0}`, 0},
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

func TestLinuxNftablesDNATTest_Eval(t *testing.T) {
	tc := linuxNftablesDNATTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "iptables form",
			response: `iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 8080 -j DNAT --to-destination 10.0.5.20:80`,
			want:     1,
		},
		{
			name:     "iptables form with flags reordered around the match",
			response: `sudo iptables -t nat -A PREROUTING -p tcp -i eth0 --dport 8080 -j DNAT --to-destination 10.0.5.20:80`,
			want:     1,
		},
		{
			name:     "nft form",
			response: `nft add rule ip nat prerouting tcp dport 8080 dnat to 10.0.5.20:80`,
			want:     1,
		},
		{
			name:     "wrong destination IP",
			response: `iptables -t nat -A PREROUTING -p tcp --dport 8080 -j DNAT --to-destination 10.0.5.21:80`,
			want:     0,
		},
		{
			name:     "wrong chain (OUTPUT instead of PREROUTING)",
			response: `iptables -t nat -A OUTPUT -p tcp --dport 8080 -j DNAT --to-destination 10.0.5.20:80`,
			want:     0,
		},
		{
			name:     "-A PREROUTING given before -t nat is equally valid iptables syntax (A12 bug probe)",
			response: `iptables -A PREROUTING -t nat -p tcp --dport 8080 -j DNAT --to-destination 10.0.5.20:80`,
			want:     1,
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

func TestLinuxLVMExtendOrderTest_Eval(t *testing.T) {
	tc := linuxLVMExtendOrderTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct order", `["pvcreate","vgextend","lvextend","resize2fs"]`, 1},
		{"correct order, prose wrapped", "Steps:\n" + `["pvcreate","vgextend","lvextend","resize2fs"]`, 1},
		{"correct order, fenced JSON", "```json\n" + `["pvcreate","vgextend","lvextend","resize2fs"]` + "\n```", 1},
		{"wrong order (lvextend before vgextend)", `["pvcreate","lvextend","vgextend","resize2fs"]`, 0},
		{"includes destructive mkfs instead of resize2fs", `["pvcreate","vgextend","lvextend","mkfs"]`, 0},
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

func TestLinuxSSHPortForwardTest_Eval(t *testing.T) {
	tc := linuxSSHPortForwardTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"loopback IP form", `ssh -L 15432:127.0.0.1:5432 ops@db1.internal`, 1},
		{"localhost form", `ssh -L 15432:localhost:5432 ops@db1.internal`, 1},
		{"extra flags before -L", `ssh -N -L 15432:127.0.0.1:5432 ops@db1.internal`, 1},
		// The forward-spec and destination checks are independent (A6):
		// getting one right and the other wrong now earns that check's
		// half-credit instead of scoring 0 outright.
		{"wrong local port still credits the correct destination", `ssh -L 5432:127.0.0.1:5432 ops@db1.internal`, 0.5},
		{"wrong host still credits the correct forward spec", `ssh -L 15432:127.0.0.1:5432 ops@db2.internal`, 0.5},
		{"destination given before -L, options-after-destination (A6 bug probe)", `ssh ops@db1.internal -L 15432:127.0.0.1:5432`, 1},
		{"-l user form for the destination (A6 bug probe)", `ssh -L 15432:127.0.0.1:5432 -l ops db1.internal`, 1},
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

func TestLinuxCronExpressionTest_Eval(t *testing.T) {
	tc := linuxCronExpressionTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"canonical single spacing", "30 3 * * 1", 1},
		{"extra spaces between fields", "30  3  *  *  1", 1},
		{"tab-separated fields", "30\t3\t*\t*\t1", 1},
		{"wrong day of week", "30 3 * * 2", 0},
		{"wrong time", "45 3 * * 1", 0},
		{"fenced code block, no language tag (A5 bug probe)", "```\n30 3 * * 1\n```", 1},
		{"fenced code block with a language tag (A5 bug probe)", "```cron\n30 3 * * 1\n```", 1},
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

func TestLinuxSystemdTimerTest_Eval(t *testing.T) {
	tc := linuxSystemdTimerTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name: "full correct answer",
			response: `Create backup.service and backup.timer. In backup.timer's
[Timer] section: OnCalendar=*-*-* 02:00:00. In [Install]:
WantedBy=timers.target.`,
			want: 1,
		},
		{
			name:     "reordered phrasing, service named first then timer",
			response: `You need a .service unit for the work and a .timer unit for the schedule. Set OnCalendar=02:00 in the timer, and WantedBy=timers.target under [Install].`,
			want:     1,
		},
		{
			name:     "minimal but complete",
			response: `backup.service + backup.timer, OnCalendar=*-*-* 02:00:00, WantedBy=timers.target`,
			want:     1,
		},
		{
			name:     "missing OnCalendar key entirely",
			response: `Use backup.service and backup.timer, with WantedBy=timers.target in [Install].`,
			want:     2.0 / 3.0,
		},
		{
			name:     "wrong install target and no OnCalendar (cron-style thinking)",
			response: `Just add a backup.service and backup.timer, and enable it with WantedBy=multi-user.target.`,
			want:     1.0 / 3.0,
		},
		{
			name:     "WantedBy with a colon separator (A15 bug probe)",
			response: `backup.service + backup.timer, OnCalendar=*-*-* 02:00:00, WantedBy: timers.target`,
			want:     1,
		},
		{
			name:     "WantedBy phrased as 'set to' in prose (A15 bug probe)",
			response: `Use backup.service and backup.timer. OnCalendar=02:00 daily. WantedBy is set to timers.target under [Install].`,
			want:     1,
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

func TestLinuxProcMeminfoAvailableTest_Eval(t *testing.T) {
	tc := linuxProcMeminfoAvailableTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"exact number", "9644", 1},
		{"prose wrapped", "The available memory is 9644 MB.", 1},
		{"with unit suffix", "9644 MB", 1},
		{"divided by 1000 instead of 1024", "9876", 0},
		{"used MemFree instead of MemAvailable", "1000", 0},
		{"off-by-one must not pass now that tolerance is 0, not 1 (A17 bug probe)", "9645", 0},
		{"off-by-one the other direction must not pass (A17 bug probe)", "9643", 0},
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

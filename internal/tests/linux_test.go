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

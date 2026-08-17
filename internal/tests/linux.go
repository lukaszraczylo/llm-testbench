package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerLinuxTests(r *testkit.Registry) {
	r.Register(linuxPctExecTest())
	r.Register(linuxSystemdOneshotTest())
}

// linuxPctExecTest: one-line command to check a service inside an LXC
// container through its Proxmox host.
func linuxPctExecTest() testkit.Test {
	prompt := `Container CT 251 is an LXC container running on Proxmox host
10.0.0.100. Your laptop cannot reach CT 251 directly - only the Proxmox
host is reachable over the network. Give a single, one-line shell command,
run from your laptop, that checks the output of "systemctl status
myservice" inside CT 251 by going through the Proxmox host.`

	// Requires ssh to the host (10.0.0.100), pct exec into container 251,
	// and the actual systemctl status command, in that logical order, on
	// one line. No (?s): DOTALL would let ssh/host/pct/systemctl match
	// across unrelated paragraphs of a longer response instead of one
	// actual command. [^\n]*? between ssh and the host (rather than
	// requiring ssh directly followed by an optional user@) tolerates any
	// flags (e.g. "ssh -t root@10.0.0.100").
	pattern := `(?i)ssh\b[^\n]*?10\.0\.0\.100\b[^\n]*pct\s+exec\s+251\b[^\n]*systemctl\s+status\s+myservice`

	return testkit.Test{
		ID:          "linux-pct-exec",
		Category:    "operations",
		Subcategory: "linux",
		Description: "One-line ssh + pct exec command to check a service status inside a Proxmox LXC container.",
		Prompt:      prompt,
		MaxTokens:   200,
		Eval:        eval.Regex(pattern),
	}
}

// linuxSystemdOneshotTest: write a systemd unit that runs a script exactly
// once at boot after networking is up, with no restart loop.
func linuxSystemdOneshotTest() testkit.Test {
	prompt := `Write a systemd unit file that runs the script
/usr/local/bin/bootstrap.sh exactly once, at boot, after network
connectivity is actually available (not merely after the network
interfaces are configured). The unit must not restart the script if it
exits - one run per boot only, no restart loop. Respond with the unit file
contents only.`

	evaluator := eval.All(
		eval.W(eval.ContainsAll("Type=oneshot", "network-online.target", "WantedBy=multi-user.target"), 3),
		eval.W(eval.ContainsAny("RemainAfterExit"), 1),
	)

	return testkit.Test{
		ID:          "linux-systemd-oneshot",
		Category:    "operations",
		Subcategory: "linux",
		Description: "Write a run-once-at-boot systemd oneshot unit gated on network-online.target.",
		System:      terseCodeOnly,
		Prompt:      prompt,
		MaxTokens:   400,
		Eval:        evaluator,
	}
}

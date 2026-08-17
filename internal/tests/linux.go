package tests

import (
	"context"
	"regexp"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerLinuxTests(r *testkit.Registry) {
	r.Register(linuxPctExecTest())
	r.Register(linuxSystemdOneshotTest())
	r.Register(linuxJournalctlFilterTest())
	r.Register(linuxCgroupOOMDiagnosisTest())
	r.Register(linuxNftablesDNATTest())
	r.Register(linuxLVMExtendOrderTest())
	r.Register(linuxSSHPortForwardTest())
	r.Register(linuxCronExpressionTest())
	r.Register(linuxSystemdTimerTest())
	r.Register(linuxProcMeminfoAvailableTest())
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

// linuxJournalctlUnitPattern matches the unit filter in either short (-u)
// or long (--unit) form, tolerant of "=" or whitespace between the option
// and its value (journalctl, via getopt_long, accepts both). It is checked
// independently of linuxJournalctlSincePattern below (A3), so either
// option can appear on the line before the other - the original single
// regex hardcoded "-u ... --since ..." order and rejected the equally
// valid "--unit"/"-S" long/short forms and "--since=" spelling.
const linuxJournalctlUnitPattern = `(?i)\bjournalctl\b[^\n]*(?:-u|--unit)[=\s]+nginx\.service\b`

// linuxJournalctlSincePattern matches the since-window filter in either
// short (-S) or long (--since) form, independent of
// linuxJournalctlUnitPattern (A3).
const linuxJournalctlSincePattern = `(?i)\bjournalctl\b[^\n]*(?:-S|--since)[=\s]+["']?-?2\s*h(?:ours?)?(?:\s+ago)?["']?`

// linuxJournalctlFilterTest: one-line journalctl command filtered to one
// unit over a relative time window.
func linuxJournalctlFilterTest() testkit.Test {
	prompt := `Give a single, one-line journalctl command that shows only
log entries for the systemd unit "nginx.service" from the last 2 hours.`

	evaluator := eval.All(
		eval.W(eval.Regex(linuxJournalctlUnitPattern), 1),
		eval.W(eval.Regex(linuxJournalctlSincePattern), 1),
	)

	return testkit.Test{
		ID:          "linux-journalctl-filter",
		Category:    "operations",
		Subcategory: "linux",
		Description: "One-line journalctl command filtered by unit and a relative 2-hour time window.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// linuxCgroupMemoryEvents is the inline cgroup v2 memory.events fixture for
// linuxCgroupOOMDiagnosisTest.
const linuxCgroupMemoryEvents = `low 0
high 0
max 47
oom 3
oom_kill 3`

// linuxCgroupOOMDiagnosisTest: read a cgroup v2 memory.events file and
// report whether the kernel actually OOM-killed the cgroup's processes.
//
// ground truth: cgroup v2's memory.events "oom" counts memory allocation
// failures that triggered the OOM killer to run, while "oom_kill" counts
// processes the kernel actually killed as a result. Here oom_kill is 3
// (not 0), so the cgroup's processes were in fact OOM-killed, 3 times.
func linuxCgroupOOMDiagnosisTest() testkit.Test {
	prompt := `Here is the contents of a container's cgroup v2
memory.events file:

` + linuxCgroupMemoryEvents + `

Based only on this file, was any process in this cgroup actually killed by
the kernel's OOM killer, and if so, how many times? Respond with only a
JSON object: {"oom_killed": true or false, "oom_kill_count": <number>}.`

	evaluator := eval.Mean(
		eval.JSONField("oom_killed", true),
		eval.JSONField("oom_kill_count", 3),
	)

	return testkit.Test{
		ID:          "linux-cgroup-oom-diagnosis",
		Category:    "operations",
		Subcategory: "linux",
		Description: "Diagnose an actual OOM-kill count from cgroup v2 memory.events (oom_kill, not the broader oom counter).",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// linuxNftablesDNATPattern requires a DNAT rule (iptables or nft form) on
// the nat table's prerouting chain, matching TCP port 8080 and forwarding
// to 10.0.5.20:80, on a single line. The "nat" table and "prerouting"
// chain tokens are matched order-free relative to each other (A12): a
// syntactically valid iptables invocation can give -A PREROUTING before
// -t nat (e.g. "iptables -A PREROUTING -t nat -p tcp --dport 8080 -j DNAT
// --to-destination 10.0.5.20:80"), and the original pattern's fixed
// "nat...prerouting" order rejected that ordering. The rest of the match
// (dport, dnat, destination) keeps its original same-line, in-order
// anchoring, since iptables and nft do put the match/target keywords after
// the table/chain in both valid orderings.
const linuxNftablesDNATPattern = `(?i)(?:iptables|nft)\b[^\n]*?(?:nat[^\n]*?prerouting|prerouting[^\n]*?nat)[^\n]*?(?:--dport|dport)\s*8080[^\n]*?dnat[^\n]*?10\.0\.5\.20:80`

// linuxNftablesDNATTest: one-line DNAT rule forwarding an external port to
// an internal host:port.
func linuxNftablesDNATTest() testkit.Test {
	prompt := `Give a single, one-line firewall command (iptables or nft,
your choice) that DNATs incoming TCP traffic on this host's eth0 interface,
port 8080, to internal address 10.0.5.20, port 80.`

	return testkit.Test{
		ID:          "linux-nftables-dnat",
		Category:    "operations",
		Subcategory: "linux",
		Description: "One-line DNAT rule (iptables or nft) forwarding TCP 8080 on eth0 to 10.0.5.20:80.",
		Prompt:      prompt,
		Eval:        eval.Regex(linuxNftablesDNATPattern),
	}
}

// linuxLVMExtendWant is the unique correct 4-step LVM growth sequence for
// linuxLVMExtendOrderTest.
var linuxLVMExtendWant = []string{"pvcreate", "vgextend", "lvextend", "resize2fs"}

// linuxLVMExtendOrderTest: order the LVM commands that grow a logical
// volume and its filesystem onto a newly added disk.
//
// ground truth: a new disk must first become an LVM physical volume
// (pvcreate), then be added to the volume group (vgextend), before the
// logical volume can grow into that new space (lvextend), after which the
// filesystem itself must be grown to use it (resize2fs, for ext4). mkfs is
// never used here - it would format over, and destroy, the existing data.
func linuxLVMExtendOrderTest() testkit.Test {
	prompt := `The logical volume data-lv on volume group data-vg (ext4
filesystem) is full. You have already partitioned a newly added disk as
/dev/sdb1, but done nothing else with it.

From this list of steps - pvcreate, vgextend, lvextend, resize2fs, mkfs -
choose exactly the 4 steps needed, in the correct order, to grow data-lv
and its filesystem onto the new space without destroying the existing
data. Respond with only a JSON array of the step ids, e.g.
["step1","step2",...].`

	return testkit.Test{
		ID:          "linux-lvm-extend-order",
		Category:    "operations",
		Subcategory: "linux",
		Description: "Order the 4 LVM/filesystem commands (of 5 candidates) that grow a logical volume onto a newly added disk.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals(linuxLVMExtendWant),
	}
}

// linuxSSHPortForwardSpecPattern matches the -L local-forward flag and its
// argument, independent of where the SSH destination appears on the line -
// -L can come before or after the destination in a valid ssh invocation
// (A6).
const linuxSSHPortForwardSpecPattern = `(?i)\bssh\b[^\n]*-L\s*15432:(?:127\.0\.0\.1|localhost):5432\b`

// linuxSSHPortForwardDestPattern matches the SSH destination in either its
// "user@host" form or the equivalent "-l user ... host" form (A6): both
// name the same destination, and the original pattern only accepted the
// former.
const linuxSSHPortForwardDestPattern = `(?i)\bssh\b[^\n]*(?:ops@db1\.internal|-l\s+ops\b[^\n]*\bdb1\.internal\b)`

// linuxSSHPortForwardTest: ssh -L local port-forward command derived from a
// scenario.
func linuxSSHPortForwardTest() testkit.Test {
	prompt := `A Postgres database only listens on 127.0.0.1:5432 on remote
host db1.internal, which you can reach over SSH as user "ops". Give a
single, one-line ssh command, run from your laptop, that forwards your
laptop's local port 15432 to that remote Postgres instance, so a local
client can connect to localhost:15432.`

	evaluator := eval.Mean(
		eval.Regex(linuxSSHPortForwardSpecPattern),
		eval.Regex(linuxSSHPortForwardDestPattern),
	)

	return testkit.Test{
		ID:          "linux-ssh-port-forward",
		Category:    "operations",
		Subcategory: "linux",
		Description: "One-line ssh -L local port-forward command reaching a Postgres instance that only listens on remote loopback.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// linuxCronExpressionPattern requires the exact 5-field cron schedule for
// "03:30 every Monday", tolerant only of whitespace width between fields -
// unlike eval.Equals, this stays correct even if the model adds a leading
// or trailing space, since the fields themselves have no valid rephrasing.
// (?m) makes ^/$ match per line rather than only at the very start/end of
// the whole string, so the expression still matches as one line within a
// longer, fenced, or multi-line response (A5).
const linuxCronExpressionPattern = `(?m)^\s*30\s+3\s+\*\s+\*\s+1\s*$`

// linuxCronExpressionEval strips a markdown code fence (if any) before
// applying linuxCronExpressionPattern. Without this, a response that
// wraps the correct expression in a ` + "```" + ` fence scored 0, because the fence
// markers and surrounding whitespace kept the line-anchored regex from
// finding a clean match (A5).
func linuxCronExpressionEval() eval.Evaluator {
	re := regexp.MustCompile(linuxCronExpressionPattern)
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		stripped := eval.ExtractCodeBlock(response, "")
		if re.MatchString(stripped) {
			return eval.Score{Value: 1, Detail: "matches the 5-field cron expression"}
		}
		return eval.Score{Value: 0, Detail: "does not match the 5-field cron expression"}
	})
}

// linuxCronExpressionTest: produce the 5-field cron expression matching a
// stated schedule.
//
// ground truth: cron fields are minute hour day-of-month month
// day-of-week. 03:30 is minute=30, hour=3; day-of-month and month are
// unrestricted (*); Monday is day-of-week 1 in standard cron numbering
// (0 and 7 both mean Sunday).
func linuxCronExpressionTest() testkit.Test {
	prompt := `Give the single 5-field cron expression that runs a job at
03:30 every Monday only. Use numeric day-of-week (Monday = 1). Respond
with only the expression, nothing else.`

	return testkit.Test{
		ID:          "linux-cron-expression",
		Category:    "operations",
		Subcategory: "linux",
		Description: "Produce the 5-field cron expression for a 03:30-every-Monday schedule.",
		Prompt:      prompt,
		Eval:        linuxCronExpressionEval(),
	}
}

// linuxSystemdTimerTest: name the systemd timer+service unit pair (not
// cron) for a fixed daily schedule, with the required timer keys.
func linuxSystemdTimerTest() testkit.Test {
	prompt := `A backup script must run every day at 02:00, managed by
systemd (not cron), using a timer unit plus a service unit. Name the two
unit file types this requires, give the key line in the .timer file that
sets the daily 02:00 schedule, and give the [Install] target the .timer
unit's WantedBy needs.`

	// ground truth: a systemd timer job is a .timer unit (the schedule)
	// paired with a same-named .service unit (the work). The .timer's
	// OnCalendar= key sets the schedule (e.g. "*-*-* 02:00:00" for daily at
	// 02:00), and its [Install] section needs WantedBy=timers.target for
	// systemctl enable to activate it. The WantedBy check accepts "=",
	// ":", or "(is) set to" between the key and its value (A15), not just a
	// bare "WantedBy=timers.target" substring - a response that phrases
	// this in prose ("WantedBy is set to timers.target") states exactly the
	// same fact.
	evaluator := eval.Mean(
		eval.ContainsAll(".service", ".timer"),
		eval.ContainsAny("OnCalendar"),
		eval.Regex(`(?i)WantedBy\s*(?:=|:|\b(?:is\s+)?set to\b)\s*timers\.target`),
	)

	return testkit.Test{
		ID:          "linux-systemd-timer",
		Category:    "operations",
		Subcategory: "linux",
		Description: "Name the systemd .timer/.service unit pair, OnCalendar schedule key, and timers.target for a daily job.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// linuxProcMeminfo is the inline /proc/meminfo fixture for
// linuxProcMeminfoAvailableTest.
const linuxProcMeminfo = `MemTotal:       16384000 kB
MemFree:         1024000 kB
MemAvailable:    9876000 kB
Buffers:          512000 kB
Cached:          3200000 kB`

// linuxProcMeminfoAvailableWant is MemAvailable (9876000 kB) converted to
// whole megabytes, floored: 9876000 / 1024 = 9644.53125 -> 9644. The
// prompt asks for the value rounded down, so 9644 is the single exact
// correct answer - tolerance 0 below, not 1 (A17): a tolerance of 1 was
// letting off-by-one wrong answers (9643 or 9645) score full credit for a
// deterministic arithmetic conversion with no room for a legitimate
// rephrasing.
var linuxProcMeminfoAvailableWant = 9876000 / 1024

// linuxProcMeminfoAvailableTest: interpret /proc/meminfo's MemAvailable
// field (not MemFree) as the memory actually available for new processes.
//
// ground truth: MemFree undercounts usable memory because it excludes
// reclaimable page cache and buffers; MemAvailable is the kernel's own
// estimate of memory available for new allocations without swapping,
// already accounting for reclaimable cache. /proc/meminfo's "kB" is always
// 1024-byte kibibytes, so converting to MB divides by 1024, not 1000.
func linuxProcMeminfoAvailableTest() testkit.Test {
	prompt := `Here is a container host's /proc/meminfo:

` + linuxProcMeminfo + `

How much memory, in whole megabytes (rounded down), is actually available
for new processes to allocate without triggering swap? Use the correct
field for this - not MemFree, which excludes reclaimable cache and
buffers. Note that /proc/meminfo's "kB" unit is always 1024-byte kibibytes,
so convert by dividing by 1024, not 1000. Respond with only the number.`

	return testkit.Test{
		ID:          "linux-proc-meminfo-available",
		Category:    "operations",
		Subcategory: "linux",
		Description: "Convert /proc/meminfo's MemAvailable field (not MemFree) to whole megabytes.",
		Prompt:      prompt,
		Eval:        eval.Numeric(eval.ExtractLastNumber[int], linuxProcMeminfoAvailableWant, 0),
	}
}

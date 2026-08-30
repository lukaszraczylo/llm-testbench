package tests

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerScenarioTests(r *testkit.Registry) {
	r.Register(scenDiskpressureFirstcmdTest())
	r.Register(scenInodeExhaustionTest())
	r.Register(scenCrashloopOrderTest())
	r.Register(scenNetLayerIsolateTest())
	r.Register(scenServiceDegradeTest())
	r.Register(scenLogRootcauseMCQTest())
	r.Register(scenBackupRestoreOrderTest())
	r.Register(scenCertexpiryTriageTest())
	r.Register(scenSystemdFailreasonTest())
	r.Register(scenPortbindFailTest())
	r.Register(scenLvmResizeOrderTest())
}

// scenDiskpressureDf is the inline df output for scenDiskpressureFirstcmdTest.
const scenDiskpressureDf = `Filesystem      Size  Used Avail Use% Mounted on
/dev/sda1       100G  100G    0G 100% /
tmpfs           16G   1.1G   15G   8% /run
/dev/sdb1       500G   30G  470G   6% /data`

// scenFirstDiskCommand scores a response whose {"first_command"} field is a
// safe, size-sorted du scan of the root filesystem. It demands the two
// essential verb flags (du to measure recursive usage, sort to order the
// result) and rejects a destructive first step (rm). A naive model that
// jumps straight to deleting files, or that lists with ls (which does not
// measure recursive usage) or find -size (which only matches known sizes),
// scores zero. This is deliberately not a bare ContainsAny on the whole
// response: the prompt forces a JSON object, and matching inside the
// command field keeps the check grounded in an actual command rather than
// prose that merely names du.
func scenFirstDiskCommand() eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		raw, err := eval.ExtractJSON(response)
		if err != nil {
			return eval.Score{Value: 0, Detail: "no JSON object found"}
		}
		var obj map[string]string
		if err := json.Unmarshal([]byte(raw), &obj); err != nil {
			return eval.Score{Value: 0, Detail: "JSON not usable as a string map"}
		}
		cmd := strings.ToLower(strings.TrimSpace(obj["first_command"]))
		if cmd == "" {
			return eval.Score{Value: 0, Detail: "first_command empty"}
		}
		for _, req := range []string{"du", "sort"} {
			if !strings.Contains(cmd, req) {
				return eval.Score{Value: 0, Detail: "first_command missing " + req}
			}
		}
		if strings.HasPrefix(cmd, "rm") || strings.Contains(cmd, " rm ") || strings.Contains(cmd, "rm -") {
			return eval.Score{Value: 0, Detail: "first_command starts by deleting files, not measuring"}
		}
		return eval.Score{Value: 1, Detail: "first_command is a safe du|sort scan of the filesystem"}
	})
}

// scenDiskpressureFirstcmdTest: give the exact one-line first command to
// find what is filling a 100%-full root filesystem.
//
// ground truth: / is 100% full on /dev/sda1 while /data (/dev/sdb1) has
// 470G free, so the scan must stay within the / filesystem and rank top-
// level directories by size. The canonical, standard-OSS one-liner is
// `du -xh --max-depth=1 / 2>/dev/null | sort -rh | head -20` - du with
// -x to not descend into the nearly-empty /data mount, a depth of 1 to get
// per-top-level-directory usage, sort -rh descending by human size, and
// head to the top consumers. ls does not measure recursive usage and find
// -size only matches known file sizes, so neither finds the fillers.
func scenDiskpressureFirstcmdTest() testkit.Test {
	prompt := `This Linux host's root filesystem /dev/sda1 is 100%
full; /data on /dev/sdb1 has plenty of free space. You must find which
paths under / are consuming the space, so you can free it. Here is the
relevant df output:

` + scenDiskpressureDf + `

Give the exact one-line shell command you would run FIRST to identify the
largest consumers under /, staying on the / filesystem and producing a
size-sorted list. Respond with only a JSON object:
{"first_command":"<the exact one-line command>"}`

	return testkit.Test{
		ID:          "scen-diskpressure-firstcmd",
		Category:    "operations",
		Subcategory: "scenario",
		Description: "Give the exact first shell command (du|sort) to find what filled a 100%-full root filesystem.",
		Prompt:      prompt,
		Eval:        scenFirstDiskCommand(),
	}
}

// scenInodeExhaustionDfHi is the inline df -hi output for
// scenInodeExhaustionTest.
const scenInodeExhaustionDfHi = `$ df -h /
Filesystem      Size  Used Avail Use% Mounted on
/dev/sda1       100G   35G   65G  35% /

$ df -hi /
Filesystem     Inodes IUsed IFree IUse% Mounted on
/dev/sda1       1.0M  1.0M    0  100% /`

// scenInodeExhaustionTest: diagnose "No space left on device" that is
// actually inode exhaustion (blocks free, inodes exhausted).
//
// ground truth: writes fail with "No space left on device" even though
// df -h shows 65G free, because df -hi shows the filesystem's inode
// table is 100% used (1.0M of 1.0M). Inode exhaustion, not block
// exhaustion, is the cause; the command that reveals it is `df -hi`
// (df with the inode flag). The fix is deleting cheap small files
// (often an unbounded cache dir filling the inode table with tiny files),
// not growing the disk (which adds blocks, not inodes) or swapping mounts.
func scenInodeExhaustionTest() testkit.Test {
	prompt := `Deleting nothing and with the disk apparently not full,
this host now fails writes with "No space left on device". Here are two
diagnostic outputs:

` + scenInodeExhaustionDfHi + `

What is the cause, and which exact command produced the revealing
output? Respond with only a JSON object:
{"cause":"<one of: inodes-exhausted, disk-full, quota-exceeded>",
"command":"<one of: df -hi, df -h, stat -f> "}`

	evaluator := eval.Mean(
		eval.JSONField("cause", "inodes-exhausted"),
		eval.JSONField("command", "df -hi"),
	)

	return testkit.Test{
		ID:          "scen-inode-exhaustion",
		Category:    "operations",
		Subcategory: "scenario",
		Description: "Recognize ENOSPC from inode exhaustion (not disk space) and the df -hi command that reveals it.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// scenCrashloopOrderTest: an incident where the operator must pick the
// diagnostic steps to find why a CrashLoopBackOff container crashes,
// before taking any action.
//
// ground truth: the only observation is the CrashLoopBackOff status and
// the restart count - nothing about *why* the container dies. Both
// read-only diagnostics are needed: B (`kubectl describe pod`, recent
// events and the last exit reason - rules out OOM-kill, image problems,
// probe failures) and A (`kubectl logs <pod> --previous`, the actual
// error output of the crashed run - names the failing line). Neither
// alone roots an app crash: describe shows the exit code but not the
// error message; logs --previous show the error but not the node-level
// events. Whether describe or logs comes first is a matter of taste, so
// the evaluator scores the SET {A, B}. What the test discriminates is
// diagnose-before-acting: C (`rollout restart`) and D (`scale
// --replicas=0`) act without diagnosing, and E (ssh to a node for
// journalctl) reaches for node-level tooling prematurely when the
// container's own state and logs are one kubectl call away.
func scenCrashloopOrderTest() testkit.Test {
	prompt := `A pod named ingestion-5d7c9b6f8b-p2k1q in namespace data
keeps crashing and restarting: kubectl get pods shows STATUS
CrashLoopBackOff, RESTARTS 14. You know nothing else about why it dies,
and you must find the root cause before changing anything. These are the
available steps:

A) kubectl logs <pod> --previous   (read the last crashed run's output)
B) kubectl describe pod <pod>      (inspect events and last exit reason)
C) kubectl rollout restart deployment ingestion
D) kubectl scale deployment ingestion --replicas=0
E) ssh to a node and run journalctl -u kubelet

Respond with only a JSON array of the step letters that make up your
diagnosis, containing exactly the SMALLEST set of steps sufficient to find
the root cause (any order). Do not include steps that only act without
diagnosing, and do not include diagnostics that are redundant when a
simpler kubectl call already surfaces the same information.`

	return testkit.Test{
		ID:          "scen-crashloop-order",
		Category:    "operations",
		Subcategory: "scenario",
		Description: "Pick the diagnostic steps (describe + logs --previous) that root-cause a CrashLoopBackOff before acting.",
		Prompt:      prompt,
		Eval:        eval.JSONStringSet([]string{"A", "B"}),
	}
}

// scenNetLayerSymptom is the inline diagnostic session for
// scenNetLayerIsolateTest.
const scenNetLayerSymptom = `$ curl -sS https://api.example.org/data
curl: (6) Could not resolve host: api.example.org

$ ping -c1 10.0.0.7
64 bytes from 10.0.0.7: icmp_seq=1 ttl=64 time=0.8 ms
(10.0.0.7 is a working internal host on the same subnet)

$ cat /etc/resolv.conf
nameserver 10.0.0.53`

// scenNetLayerIsolateTest: isolate which network layer is broken from
// inline symptoms and pick the strict layer among an enumerated set.
//
// ground truth: curl fails with "Could not resolve host" - DNS
// resolution, not a routing or TCP problem (an ICMP packet to an internal
// host on the same subnet succeeds, so IP reachability on the local
// segment is fine, and curl never even reached TCP/TLS because it could
// not translate the name). The nameserver 10.0.0.53 is configured but
// the name does not resolve, so the failing layer is strictly dns; it is
// not ip-routing, tcp-connect, tls, or http, none of which are reached
// when resolution itself fails.
func scenNetLayerIsolateTest() testkit.Test {
	prompt := `A service on this host cannot reach https://api.example.org.
Here is the diagnostic session:

` + scenNetLayerSymptom + `

At which network layer is the failure occurring? Respond with only a
JSON object:
{"layer":"<one of: dns, ip-routing, tcp-connect, tls, http>"}

Choose exactly one layer from the enumerated list.`

	return testkit.Test{
		ID:          "scen-net-layer-isolate",
		Category:    "operations",
		Subcategory: "scenario",
		Description: "Isolate a network failure to the DNS layer from inline curl/ping/resolv.conf symptoms.",
		Prompt:      prompt,
		Eval:        eval.JSONField("layer", "dns"),
	}
}

// scenServiceDegradeContext is the inline telemetry + deploy timeline for
// scenServiceDegradeTest.
const scenServiceDegradeContext = `Canary deployed to 100% at 14:00: config file
change lowering the app's connection-pool size (pool: 200 -> 50) and
raising the worker count.

Latency (p99) of the checkout service API:
  13:30  210ms
  14:05  1400ms
  14:30  1650ms
  15:00  1700ms

Resource metrics for the checkout pods: CPU at 12%, memory at 40% -
both low. No crash, no 5xx errors, no data loss.`

// scenServiceDegradeTest: a service-degradation decision tree where the
// regression is tracked to a config change with healthy resources.
//
// ground truth: the p99 latency jumped from ~210ms to ~1400ms exactly at
// the 14:00 canary rollout, whose only meaningful change is the
// connection-pool size drop (200 -> 50) - the obvious pattern match is the
// config change, and CPU/memory are low so the pool is not saturated by
// resource pressure. The correct action is to roll back / revert that
// config change (rollback-config). scale-out is wrong (no CPU/memory
// pressure to scale for), restart alone is wrong (it does not revert the
// config), and restore-backup is wrong (no data loss to recover from).
func scenServiceDegradeTest() testkit.Test {
	prompt := `The checkout service API latency spiked and stayed high.
Here is the context:

` + scenServiceDegradeContext + `

Which single action best restores service? Respond with only a JSON
object:
{"action":"<one of: rollback-config, scale-out, restore-backup, restart>"}`

	return testkit.Test{
		ID:          "scen-service-degrade",
		Category:    "operations",
		Subcategory: "scenario",
		Description: "Pick the correct remediation (config rollback) for a canary-introduced service degradation with healthy resources.",
		Prompt:      prompt,
		Eval:        eval.JSONField("action", "rollback-config"),
	}
}

// scenPgConnectionLog is the inline log excerpt for
// scenLogRootcauseMCQTest.
const scenPgConnectionLog = `2026-08-29 14:22:11 UTC FATAL:  remaining connection slots are reserved for non-replication superuser connections
2026-08-29 14:22:11 UTC DETAIL:  1000 concurrent connections, 100 remaining connection slots available
2026-08-29 14:22:12 UTC ERROR:  connection to server at "db.internal" (10.0.1.25), port 5432 failed: FATAL:  remaining connection slots are reserved for non-replication superuser connections`

// scenLogRootcauseMCQTest: map a log pattern to its root cause from
// lettered options.
//
// ground truth: "remaining connection slots are reserved" with a detail
// line counting 1000 concurrent connections is the textbook PostgreSQL
// signal that the connection slot limit (max_connections) has been
// reached - applications are holding more connections than the server is
// configured to allow. It is not a disk-full condition (no out-of-space
// message), not a slow-query problem (no long query shown), and not an
// authentication failure (no `password authentication failed` line).
func scenLogRootcauseMCQTest() testkit.Test {
	prompt := `Applications calling the Postgres database below now fail to
connect. Here is a log excerpt:

` + scenPgConnectionLog + `

What is the root cause of these connection failures?

A) connection pool exhausted (max_connections reached)
B) disk full
C) a single slow query blocking others
D) authentication failure

Respond with only a JSON object: {"answer":"<the single letter A-D>"}`

	return testkit.Test{
		ID:          "scen-log-rootcause-mcq",
		Category:    "operations",
		Subcategory: "scenario",
		Description: "Map a PostgreSQL max_connections log pattern to its root cause via a lettered MCQ.",
		Prompt:      prompt,
		Eval:        eval.JSONField("answer", "A"),
	}
}

// scenBackupRestoreOptions lists the available steps for
// scenBackupRestoreOrderTest.
const scenBackupRestoreOptions = `A) take a fresh logical backup (pg_dump) of the current data
B) apply the destructive migration
C) validate row counts and data after the migration
D) restore last night's backup before doing anything else`

// scenBackupRestoreOrderTest: order the steps for a safe data-changing
// migration, guaranteeing recoverability.
//
// ground truth: to make a destructive migration recoverable you must
// snapshot the CURRENT state first (A - a fresh pg_dump), then apply the
// migration (B), then validate the result (C). Restoring last night's
// backup (D) first is wrong: it is a stale snapshot that would throw
// away today's writes before any change has even been made, and it cannot
// be the safety net for the migration.
func scenBackupRestoreOrderTest() testkit.Test {
	prompt := `You are about to apply a destructive migration to a
production Postgres database and must guarantee you can get back to the
current state. The available steps are:

` + scenBackupRestoreOptions + `

Respond with only a JSON array of the step letters, in the order from
first to last, that makes this migration recoverable - include exactly
the steps you would run, in order.`

	return testkit.Test{
		ID:          "scen-backup-restore-order",
		Category:    "operations",
		Subcategory: "scenario",
		Description: "Order the steps (fresh backup, then migrate, then validate) that make a destructive migration recoverable.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals([]string{"A", "B", "C"}),
	}
}

// scenCertExpirySymptom is the inline symptom for
// scenCertexpiryTriageTest.
const scenCertExpirySymptom = `$ date
Sat Aug 29 09:14:02 UTC 2026

$ curl -v https://gateway.internal/
...
* SSL connection using TLSv1.3
* SSL certificate verify result: certificate has expired

$ openssl x509 -in /etc/ssl/svc.crt -noout -dates
notBefore=Aug  1 12:00:00 2025 GMT
notAfter=Aug  1 12:00:00 2026 GMT`

// scenCertexpiryTriageTest: triage a TLS failure whose cause is an
// expired certificate.
//
// ground truth: `date` anchors wall time at Aug 29 2026; the openssl
// -dates readout shows the served cert valid until Aug 1 2026 - the
// current date is past notAfter, so the certificate expired and the
// cause category is cert-expired. Clock-skew would need the host clock
// to disagree with reality, but the cert's own window explains the
// failure exactly, and cipher-mismatch would show a handshake/algo error,
// not a verify result. The standard command to inspect a certificate's
// validity end date directly is openssl x509 -noout -enddate (the
// readout above came from the -dates variant of the same inspection).
func scenCertexpiryTriageTest() testkit.Test {
	prompt := `Clients now fail to connect to gateway.internal with a TLS
verification error. Here is the diagnostic session:

` + scenCertExpirySymptom + `

What is the cause, and which standard command inspects a certificate's
validity directly? Respond with only a JSON object:
{"cause":"<one of: cert-expired, clock-skew, cipher-mismatch>",
"check_command":"<one of: openssl-x509-enddate, date, openssl-cipher>"}`

	evaluator := eval.Mean(
		eval.JSONField("cause", "cert-expired"),
		eval.JSONField("check_command", "openssl-x509-enddate"),
	)

	return testkit.Test{
		ID:          "scen-certexpiry-triage",
		Category:    "operations",
		Subcategory: "scenario",
		Description: "Identify an expired certificate as the TLS failure cause and the openssl command that inspects validity.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// scenSystemdStatus is the inline systemctl status excerpt for
// scenSystemdFailreasonTest.
const scenSystemdStatus = `$ systemctl status worker-queue
● worker-queue.service - Queue worker
     Loaded: loaded (/etc/systemd/system/worker-queue.service; enabled)
     Active: failed (Result: exit-code) since Tue 2026-08-25 09:12:03 UTC; 12s ago
   Main PID: 4821 (code=exited, status=1/FAILURE)
        CPU: 120ms

$ journalctl -u worker-queue -n 6
Aug 25 09:12:03 host worker-queue[4821]: config/openapi.yaml: open: no such file or directory
Aug 25 09:12:03 host worker-queue[4821]: fatal: failed to load configuration
Aug 25 09:12:03 host worker-queue[4821]: exiting after startup failure`

// scenSystemdFailreasonTest: reason about why a systemd unit failed from
// an inline status + journal excerpt.
//
// ground truth: the journal line "config/openapi.yaml: open: no such
// file or directory" - the binary exits because its configuration file
// cannot be found at the expected path. The category is missing-config,
// not permission (no "Permission denied"), not missing-binary (the
// binary ran and printed a config-diagnostic trace), and not port-in-use
// (no bind error). The fix is to provide or point the unit at the
// correct config path, not to reinstall the binary or chase a port.
func scenSystemdFailreasonTest() testkit.Test {
	prompt := `A systemd unit is failing. Here is the status and journal:

` + scenSystemdStatus + `

What category of failure is this? Respond with only a JSON object:
{"category":"<one of: missing-config, permission, missing-binary, port-in-use>"}

Choose exactly one category from the enumerated list.`

	return testkit.Test{
		ID:          "scen-systemd-failreason",
		Category:    "operations",
		Subcategory: "scenario",
		Description: "Classify a systemd unit failure (missing config file) from inline status and journal output.",
		Prompt:      prompt,
		Eval:        eval.JSONField("category", "missing-config"),
	}
}

// scenPortbindError is the inline startup error for scenPortbindFailTest.
const scenPortbindError = `$ service listener start
(1) listen tcp :8080: bind: address already in use
listener: could not bind to 0.0.0.0:8080, exiting

$ ss -ltnp
State    Recv-Q   Send-Q   Local Address:Port   Peer Address:Port  Process
LISTEN   0      128    0.0.0.0:8080          0.0.0.0:*          users:(("listener",pid=1044,fd=3))`

// scenPortbindFailTest: diagnose an "address already in use" bind
// failure and the command that identifies the occupying process.
//
// ground truth: the startup error "bind: address already in use" for
// port 8080, and the `ss -ltnp` output shows an existing listener (pid
// 1044) already bound to 0.0.0.0:8080 - a duplicate/lingering instance
// still holds the port. The cause category is port-in-use, and the
// standard command that lists listening sockets with the owning process
// is `ss -ltnp` (ss with listener, numerical, port and process flags);
// netstat is the older equivalent but ss is the modern standard on this
// home-cluster stack.
func scenPortbindFailTest() testkit.Test {
	prompt := `A service fails to start. Here is the output:

` + scenPortbindError + `

What is the cause, and which command identified the process holding the
port? Respond with only a JSON object:
{"cause":"<one of: port-in-use, missing-binary, config-error>",
"command":"<one of: ss -ltnp, netstat -r, lsblk>"}`

	evaluator := eval.Mean(
		eval.JSONField("cause", "port-in-use"),
		eval.JSONField("command", "ss -ltnp"),
	)

	return testkit.Test{
		ID:          "scen-portbind-fail",
		Category:    "operations",
		Subcategory: "scenario",
		Description: "Diagnose an address-in-use bind failure and the ss -ltnp command that reveals the holding process.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// scenLvmSteps lists the LVM extend steps for scenLvmResizeOrderTest.
const scenLvmSteps = `A) lvextend -L +20G /dev/vg0/data        (grow the logical volume)
B) pvs                                     (check free space in the volume group)
C) resize2fs /dev/vg0/data                 (grow the filesystem to fill the LV)
D) umount /dev/vg0/data                    (unmount the volume)`

// scenLvmResizeOrderTest: order the steps of a safe LVM logical-volume
// resize.
//
// ground truth: to extend a filesystem on LVM you first confirm there is
// free space in the volume group (B - `pvs`), then extend the logical
// volume (A - `lvextend`), then grow the filesystem to fill the newly
// added space (C - `resize2fs`). Doing C before A, or A before
// confirming free space in B, fails (nothing to grow into); D (umount)
// is not required for an online extend of a mounted ext4/xfs volume.
func scenLvmResizeOrderTest() testkit.Test {
	prompt := `You must safely grow the filesystem on LVM logical volume
/dev/vg0/data while it stays online. The available steps are:

` + scenLvmSteps + `

Respond with only a JSON array of the step letters, in the order from
first to last, that correctly extends this volume - include exactly the
steps you would run, in order.`

	return testkit.Test{
		ID:          "scen-lvm-resize-order",
		Category:    "operations",
		Subcategory: "scenario",
		Description: "Order the LVM resize steps (check VG space, extend LV, then grow the filesystem) for an online extend.",
		Prompt:      prompt,
		Eval:        eval.JSONStringArrayEquals([]string{"B", "A", "C"}),
	}
}

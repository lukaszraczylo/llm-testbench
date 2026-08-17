package tests

import (
	"context"
	"regexp"
	"strings"

	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerMacOSTests(r *testkit.Registry) {
	r.Register(macosTimeoutPortabilityTest())
	r.Register(macosLaunchdCronTest())
	r.Register(macosSedInplaceTest())
	r.Register(macosLaunchctlBootstrapTest())
	r.Register(macosPlutilPlistReadTest())
	r.Register(macosMdfindSpotlightTest())
	r.Register(macosAPFSSnapshotTest())
	r.Register(macosSSHIdentityAgentTest())
	r.Register(macosCaffeinateTest())
	r.Register(macosStatBSDGNUTest())
}

// macosTimeoutPortabilityTest: explain why `timeout` is missing on stock
// macOS and give two portable fixes.
func macosTimeoutPortabilityTest() testkit.Test {
	prompt := `A deployment script runs this on a fresh, stock macOS machine (no
Homebrew packages installed yet):

` + "```sh" + `
timeout 30 ./run-migration.sh
` + "```" + `

It fails immediately with:

` + "```" + `
timeout: command not found
` + "```" + `

Explain why this happens on stock macOS, and give two different portable
fixes: one that installs a replacement, and one that works with zero extra
packages installed.`

	evaluator := eval.Mean(
		eval.ContainsAny("coreutils", "gtimeout"),
		eval.ContainsAny("perl -e 'alarm'", "perl -e \"alarm\"", "alarm(", "background & kill", "& kill", "builtin"),
		eval.ContainsAll("GNU"),
	)

	return testkit.Test{
		ID:          "macos-timeout-portability",
		Category:    "operations",
		Subcategory: "macos",
		Description: "Explain why `timeout` is missing on stock macOS and give two portable fixes.",
		Prompt:      prompt,
		MaxTokens:   500,
		Eval:        evaluator,
	}
}

// macosLaunchdCronTest: identify launchd as the persistence mechanism for a
// recurring macOS job and the minimal plist keys it needs.
func macosLaunchdCronTest() testkit.Test {
	prompt := `On macOS, you need a background job that:

1. Survives a reboot (it must still be active after the machine restarts).
2. Runs automatically every 5 minutes, with no user needing to be logged in
   to trigger it manually.

Which macOS mechanism should you use for this (not cron), and what are the
minimal keys a property list for it needs to include to run every 5
minutes? Name the mechanism, the plist keys, and which directory the plist
file belongs in for a per-machine (not per-user) job.`

	// The prompt asks specifically for a per-machine (not per-user) job, so
	// LaunchDaemons is the only correct plist directory; LaunchAgents is
	// per-user and would be wrong here (a contrastive mention of
	// LaunchAgents, e.g. explaining why it's not the right choice, is fine
	// and is not penalized - see B2 for the same principle).
	evaluator := eval.Mean(
		eval.ContainsAny("launchd", "launchctl"),
		eval.ContainsAny("StartInterval", "StartCalendarInterval"),
		eval.ContainsAll("LaunchDaemons"),
	)

	return testkit.Test{
		ID:          "macos-launchd-cron",
		Category:    "operations",
		Subcategory: "macos",
		Description: "Identify launchd as the mechanism for a persistent, reboot-surviving, 5-minute recurring macOS job.",
		Prompt:      prompt,
		MaxTokens:   500,
		Eval:        evaluator,
	}
}

// macosSedPattern requires the BSD-form "sed -i" with an empty
// single-quoted or double-quoted extension argument, invoking a foo->bar
// substitution on file.txt, on a single line (no DOTALL) so a scattered,
// multi-paragraph mention of the same tokens does not count as a real
// one-line command. The substitution's delimiter is accepted as any of the
// common sed delimiter characters (/, |, comma, #, colon, semicolon, @),
// not just "/" - "s/foo/bar/" is the conventional example, but any
// consistently-used delimiter is equally correct BSD sed syntax (AN3). Go's
// RE2 engine has no backreferences, so each delimiter choice is spelled out
// as its own alternative rather than captured-and-reused.
//
// ground truth: BSD sed's -i flag takes a mandatory in-place-suffix
// argument (used for backup files); passing an empty single-quoted or
// double-quoted string means "no backup file". GNU sed's -i takes an
// OPTIONAL argument, so
// "sed -i 's/foo/bar/' file.txt" (no argument to -i) works on Linux but
// errors on macOS, where it is parsed as "-i 's/foo/bar/'" (using the sed
// script itself as the backup suffix) followed by a missing script operand.
const macosSedPattern = `(?i)\bsed\b[^\n]*-i\s*(''|"")[^\n]*(?:s/foo/bar/|s\|foo\|bar\||s,foo,bar,|s#foo#bar#|s:foo:bar:|s;foo;bar;|s@foo@bar@)g?[^\n]*file\.txt`

// macosSedInplaceTest: give the portable BSD sed one-liner for in-place
// substitution with no backup file, contrasted with the GNU-only form that
// fails on stock macOS.
func macosSedInplaceTest() testkit.Test {
	prompt := `On a stock macOS machine (BSD sed, not GNU sed), this command:

` + "```sh" + `
sed -i 's/foo/bar/' file.txt
` + "```" + `

fails with an error about a missing extension or a "no such file"
complaint. Give the corrected one-line BSD sed command that replaces "foo"
with "bar" in file.txt, edits the file in place, and creates NO backup
file.`

	return testkit.Test{
		ID:          "macos-sed-inplace",
		Category:    "operations",
		Subcategory: "macos",
		Description: "Give the BSD sed -i '' in-place substitution syntax that works on stock macOS with no backup file.",
		Prompt:      prompt,
		Eval:        eval.Regex(macosSedPattern),
	}
}

// macosLaunchctlBootstrapTest: identify launchctl bootstrap with an
// explicit system domain target as the modern replacement for the
// deprecated launchctl load, for loading a LaunchDaemon.
func macosLaunchctlBootstrapTest() testkit.Test {
	prompt := `A new LaunchDaemon plist has just been written to
/Library/LaunchDaemons/com.example.myjob.plist on a current macOS release.

On modern macOS (10.11 El Capitan and later), which launchctl subcommand
should you use to load and start this LaunchDaemon, and which domain
target must you pass it (this is a system-wide daemon, not a per-user
agent)? Also name the older launchctl subcommand for loading jobs that
Apple deprecated in favor of this one, and briefly say why (it lacked
explicit domain targeting).`

	// ground truth: Apple deprecated the session-based "launchctl load" /
	// "launchctl unload" pair starting in OS X 10.11, replacing them with
	// "launchctl bootstrap" / "launchctl bootout", which require an
	// explicit domain target (system/, gui/<uid>, user/<uid>). A
	// system-wide LaunchDaemon uses the "system" domain, giving
	// "sudo launchctl bootstrap system /Library/LaunchDaemons/....plist".
	// Component 3 deliberately requires an explicit deprecation cue rather
	// than a bare "launchctl load" mention: a response that just uses
	// launchctl load (as its own recommended command, not something to
	// avoid) must not get credit for correctly flagging it as deprecated.
	evaluator := eval.Mean(
		eval.ContainsAny("bootstrap"),
		eval.ContainsAny("bootstrap system", "system domain", "domain system", "target system", "system target"),
		eval.ContainsAny("deprecated", "avoid launchctl load", "no longer recommended", "outdated", "legacy", "superseded", "obsolete", "replaced"),
	)

	return testkit.Test{
		ID:          "macos-launchctl-bootstrap",
		Category:    "operations",
		Subcategory: "macos",
		Description: "Identify launchctl bootstrap+system domain as the modern replacement for the deprecated launchctl load.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// macosPlutilFlagPattern requires a properly bounded "-p" flag token
// (immediately preceded by whitespace or the start of the response, and
// immediately followed by whitespace or the end of the response). An
// unbounded substring search for "-p" also matches inside the word
// "pretty-print" (its internal hyphen followed by "p"), which would let a
// response that only talks *about* pretty-printing - without actually
// invoking "plutil -p" - score a false positive (A2).
var macosPlutilFlagPattern = regexp.MustCompile(`(?i)(?:^|\s)-p(?:\s|$)`)

// macosPlutilPlistReadEval accepts three materially correct ways to read a
// plist file's contents in human-readable form using only macOS-bundled
// tools: "plutil -p <path>" (with -p bounded as an actual flag token, not
// matched as a substring of "pretty-print"), "plutil -convert xml1 -o -
// <path>" (convert to XML and write to stdout instead of editing in
// place), and "defaults read <domain-or-path>" (A2).
func macosPlutilPlistReadEval() eval.Evaluator {
	return eval.EvaluatorFunc(func(_ context.Context, response string) eval.Score {
		if !strings.Contains(response, "com.example.app") {
			return eval.Score{Value: 0, Detail: "does not name the target plist/domain"}
		}
		lower := strings.ToLower(response)
		switch {
		case strings.Contains(lower, "plutil") && macosPlutilFlagPattern.MatchString(response):
			return eval.Score{Value: 1, Detail: "plutil -p (a properly bounded flag token)"}
		case strings.Contains(lower, "plutil") && strings.Contains(lower, "-convert") && strings.Contains(lower, "xml1"):
			return eval.Score{Value: 1, Detail: "plutil -convert xml1"}
		case strings.Contains(lower, "defaults read"):
			return eval.Score{Value: 1, Detail: "defaults read"}
		default:
			return eval.Score{Value: 0, Detail: "no recognized plist-reading command"}
		}
	})
}

// macosPlutilPlistReadTest: pretty-print a plist file's contents using a
// tool that ships with macOS (no Homebrew installs).
func macosPlutilPlistReadTest() testkit.Test {
	prompt := `You need to inspect the raw contents of this plist file on a
stock macOS machine, with no Homebrew packages installed:

/Library/Preferences/com.example.app.plist

Give a single command, using only a tool that ships with macOS by default,
that prints the plist's contents to your terminal in a readable
(non-binary) form.`

	// ground truth: plutil ships with macOS and its -p flag pretty-prints a
	// plist (binary or XML) to stdout in a human-readable form, taking the
	// file path directly - no domain-name lookup or Homebrew install
	// needed, unlike "defaults read <domain>". "plutil -convert xml1 -o -"
	// (convert to XML, write to stdout) and "defaults read" are also
	// accepted as materially correct alternatives (A2).
	return testkit.Test{
		ID:          "macos-plutil-plist-read",
		Category:    "operations",
		Subcategory: "macos",
		Description: "Pretty-print a plist file's contents using the built-in plutil -p command.",
		Prompt:      prompt,
		Eval:        macosPlutilPlistReadEval(),
	}
}

// macosMdfindSpotlightEval gates the whole score on mdfind actually being
// used: the prompt itself supplies both a plausible flag ("-name" is a real
// "find" flag too) and the search term ("invoice"), so a bare filesystem
// walk with "find" that happens to echo those prompt-supplied tokens must
// not be able to reach a passing score. Only once mdfind is confirmed
// present does the response get scored on the remaining, more permissive
// criteria (A13).
func macosMdfindSpotlightEval() eval.Evaluator {
	sub := eval.Mean(
		eval.ContainsAny("-onlyin", "-name"),
		eval.ContainsAny("invoice"),
	)
	return eval.EvaluatorFunc(func(ctx context.Context, response string) eval.Score {
		if !strings.Contains(strings.ToLower(response), "mdfind") {
			return eval.Score{Value: 0, Detail: "mdfind not used; a filesystem walk is not what was asked for"}
		}
		return sub.Evaluate(ctx, response)
	})
}

// macosMdfindSpotlightTest: use mdfind (Spotlight's metadata index) instead
// of a full filesystem walk to find files by name.
func macosMdfindSpotlightTest() testkit.Test {
	prompt := `On macOS, give a single command that searches Spotlight's
existing metadata index (not a full filesystem walk with "find") for files
whose name contains "invoice", scoped only to your home directory (~).`

	// ground truth: mdfind queries the already-built Spotlight index rather
	// than walking the filesystem; -onlyin scopes the search to a
	// directory, and either -name or a plain query string matches on file
	// name. mdfind's presence is a hard gate (A13): "-name" is also a real
	// "find" flag, and "invoice" is echoed straight from the prompt, so
	// neither can carry the score for a wrong, find-based answer.
	return testkit.Test{
		ID:          "macos-mdfind-spotlight",
		Category:    "operations",
		Subcategory: "macos",
		Description: "Use mdfind against Spotlight's index, scoped to a directory, instead of a full filesystem walk.",
		Prompt:      prompt,
		Eval:        macosMdfindSpotlightEval(),
	}
}

// macosAPFSSnapshotTest: create and list local APFS snapshots via tmutil,
// which works even with no external Time Machine disk attached.
func macosAPFSSnapshotTest() testkit.Test {
	prompt := `Before a risky system change, you want to create a local APFS
snapshot of the root volume as a quick rollback point - with no external
Time Machine disk attached (APFS local snapshots do not need one). Give
the command to create a local snapshot now, and the separate command to
list the local snapshots that currently exist.`

	// ground truth: tmutil manages local APFS snapshots independently of
	// having a Time Machine backup destination attached. "tmutil
	// localsnapshot" is the current, documented verb that creates one
	// immediately ("tmutil snapshot" does not appear in tmutil's own usage
	// listing on current macOS and is accepted only as a legacy/undocumented
	// alias - verified on this machine, A1); "tmutil listlocalsnapshots /"
	// lists the local snapshots that exist for the root volume.
	evaluator := eval.Mean(
		eval.ContainsAny("tmutil localsnapshot", "tmutil snapshot"),
		eval.ContainsAny("tmutil listlocalsnapshots", "listlocalsnapshots"),
	)

	return testkit.Test{
		ID:          "macos-apfs-snapshot",
		Category:    "operations",
		Subcategory: "macos",
		Description: "Create and list local APFS snapshots via tmutil with no external Time Machine disk attached.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// macosSSHIdentityAgentTest: point ssh at 1Password's SSH agent socket via
// the IdentityAgent ssh_config directive.
func macosSSHIdentityAgentTest() testkit.Test {
	prompt := `You use the 1Password app as your SSH agent instead of the
default ssh-agent, and your shell environment does NOT set SSH_AUTH_SOCK.
1Password's agent socket lives at:

~/Library/Group Containers/2BUA8C4S2C.com.1password/t/agent.sock

Give the single ssh_config line to add (to ~/.ssh/config, either globally
or under a Host block) that points ssh at this socket, so ssh and git-over-
ssh commands can find your keys through 1Password.`

	// ground truth: ssh_config's IdentityAgent directive overrides which
	// agent socket ssh (and anything using libssh's config, like git)
	// connects to; 1Password's own setup docs specify this exact directive
	// and socket path for its SSH agent integration.
	evaluator := eval.Mean(
		eval.ContainsAny("IdentityAgent"),
		eval.ContainsAny("1password/t/agent.sock", "agent.sock"),
	)

	return testkit.Test{
		ID:          "macos-ssh-identity-agent",
		Category:    "operations",
		Subcategory: "macos",
		Description: "Point ssh at 1Password's SSH agent socket via the IdentityAgent ssh_config directive.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// macosCaffeinateTest: use caffeinate, built into macOS, to keep the
// machine awake only for the duration of a given command.
func macosCaffeinateTest() testkit.Test {
	prompt := `A long-running data migration script, ./migrate.sh, needs to
run on a MacBook without the machine going to sleep partway through - and
you want sleep prevention to end automatically the moment the script
finishes, with no separate cleanup step. Give a single command, using a
tool built into macOS, that does this.`

	// ground truth: caffeinate, bundled with macOS, takes a power-management
	// assertion for as long as a given utility runs when you pass that
	// utility as its argument, and releases the assertion automatically
	// when the utility exits - no manual cleanup needed. migrate.sh must be
	// caffeinate's own argument on the same command line (e.g.
	// "caffeinate ./migrate.sh"), not merely mentioned somewhere in the
	// response: a backgrounded "caffeinate & ./migrate.sh" variant runs the
	// two independently, so caffeinate's assertion does not track the
	// script's lifetime and would not auto-release when it exits (A11).
	evaluator := eval.Regex(`(?i)\bcaffeinate\b[^\n&;|]*\.?/?migrate\.sh`)

	return testkit.Test{
		ID:          "macos-caffeinate",
		Category:    "operations",
		Subcategory: "macos",
		Description: "Use caffeinate to prevent sleep only for the duration of a given script.",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

// macosStatPattern requires the BSD stat -f format-string form printing
// only the byte size (%z) of file.txt. The %z format specifier is accepted
// bare or wrapped in either single or double quotes (both are valid shell
// quoting for a format string with no special characters in it) - not just
// double quotes (A4).
const macosStatPattern = `(?i)\bstat\b[^\n]*-f\s*['"]?%z['"]?[^\n]*file\.txt`

// macosStatBSDGNUTest: give the BSD stat command (macOS's built-in stat)
// for a file's size in bytes, and name the differing GNU stat flag.
func macosStatBSDGNUTest() testkit.Test {
	prompt := `On macOS (BSD stat, not GNU stat), give a single command that
prints ONLY the size of file.txt in bytes, nothing else. Then name the
single-letter flag GNU stat (as found on Linux) uses in place of macOS's
-f for its own format-string argument.`

	// ground truth: BSD stat takes its format string via -f, and %z is the
	// byte-size format specifier, giving "stat -f%z file.txt". GNU stat
	// instead uses -c for its format string (with %s as the byte-size
	// specifier) - the flag letter itself, not just the format string
	// syntax, differs between the two implementations.
	evaluator := eval.Mean(
		eval.Regex(macosStatPattern),
		eval.ContainsAny("-c"),
	)

	return testkit.Test{
		ID:          "macos-stat-bsd-gnu",
		Category:    "operations",
		Subcategory: "macos",
		Description: "Give the BSD stat -f%z byte-size command and name the differing GNU stat flag (-c).",
		Prompt:      prompt,
		Eval:        evaluator,
	}
}

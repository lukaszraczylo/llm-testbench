package tests

import (
	"github.com/lukaszraczylo/llm-testbench/internal/eval"
	"github.com/lukaszraczylo/llm-testbench/internal/testkit"
)

func registerMacOSTests(r *testkit.Registry) {
	r.Register(macosTimeoutPortabilityTest())
	r.Register(macosLaunchdCronTest())
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

package tests

import (
	"context"
	"testing"
)

func TestMacosTimeoutPortabilityTest_Eval(t *testing.T) {
	tc := macosTimeoutPortabilityTest()

	good := `macOS ships BSD userland, not GNU coreutils, so there is no
"timeout" binary out of the box - it is a GNU coreutils tool. Two portable
fixes: (1) brew install coreutils, then use "gtimeout 30 ./script.sh"
(GNU coreutils installs its tools with a "g" prefix to avoid clobbering the
BSD originals); or (2) with zero extra packages, background the command
and kill it after a sleep, e.g. run it, sleep 30 & kill it if still
running, or use "perl -e 'alarm shift; exec @ARGV' 30 ./script.sh" which
relies only on the perl builtin that ships with macOS.`

	badMissingAll := "Just run the script differently, I'm not sure why it fails."
	badMissingGNU := "Install coreutils via brew and use gtimeout, or use perl -e 'alarm'."

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"good answer full credit", good, 1},
		{"missing everything scores 0", badMissingAll, 0},
		{"missing GNU explanation loses one third", badMissingGNU, 2.0 / 3.0},
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

func TestMacosLaunchdCronTest_Eval(t *testing.T) {
	tc := macosLaunchdCronTest()

	good := `Use launchd, managed via launchctl, not cron. Put a plist in
/Library/LaunchDaemons for a system-wide job. Minimal keys: Label,
ProgramArguments, and StartInterval set to 300 (seconds) to run every 5
minutes; RunAtLoad is optional.`

	badWrongMechanism := "Just add a crontab entry with */5 * * * *."
	badMissingKeys := "Use launchd and put the plist in LaunchDaemons."
	badWrongDirectory := `Use launchd, managed via launchctl. Put a plist in
~/Library/LaunchAgents. Minimal keys: Label, ProgramArguments, and
StartInterval set to 300.`
	goodContrastiveMention := `Use launchd via launchctl, not cron. This must be a
per-machine job, so the plist goes in /Library/LaunchDaemons, not
~/Library/LaunchAgents (LaunchAgents is per-user and would not run without
a logged-in session). Minimal keys: Label, ProgramArguments, and
StartInterval set to 300.`

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"good answer full credit", good, 1},
		{"wrong mechanism (cron) scores 0", badWrongMechanism, 0},
		{"missing plist keys loses one third", badMissingKeys, 2.0 / 3.0},
		{"LaunchAgents (per-user) is the wrong directory for this per-machine job", badWrongDirectory, 2.0 / 3.0},
		{"contrastive LaunchAgents mention alongside correct LaunchDaemons still scores full credit", goodContrastiveMention, 1},
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

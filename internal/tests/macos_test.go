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

func TestMacosSedInplaceTest_Eval(t *testing.T) {
	tc := macosSedInplaceTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"single-quote empty extension", `sed -i '' 's/foo/bar/' file.txt`, 1},
		{"double-quote empty extension with -e flag", `On macOS: sed -i "" -e 's/foo/bar/' file.txt`, 1},
		{"global replace flag still matches", `sed -i '' 's/foo/bar/g' file.txt`, 1},
		{"GNU-style with no extension argument is wrong on macOS", `sed -i 's/foo/bar/' file.txt`, 0},
		{"backup extension creates a file, contradicting the requirement", `sed -i.bak 's/foo/bar/' file.txt`, 0},
		{"pipe delimiter is equally valid sed syntax (AN3)", `sed -i '' 's|foo|bar|' file.txt`, 1},
		{"comma delimiter (AN3)", `sed -i '' 's,foo,bar,g' file.txt`, 1},
		{"hash delimiter (AN3)", `sed -i "" 's#foo#bar#' file.txt`, 1},
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

func TestMacosLaunchctlBootstrapTest_Eval(t *testing.T) {
	tc := macosLaunchctlBootstrapTest()

	good := `Use "sudo launchctl bootstrap system /Library/LaunchDaemons/com.example.myjob.plist".
The older "launchctl load" command is deprecated because it has no explicit
domain target.`
	goodReordered := `The modern subcommand is bootstrap, targeting the system
domain: sudo launchctl bootstrap system /Library/LaunchDaemons/com.example.myjob.plist.
launchctl load is deprecated.`
	goodPhrasing := `Load it with launchctl bootstrap, using the system domain
target since this is a system-wide daemon. Avoid launchctl load, which
Apple deprecated for lacking an explicit domain target.`
	badWrongSubcommand := `Just run "sudo launchctl load /Library/LaunchDaemons/com.example.myjob.plist".`
	badMissingDomain := `Use launchctl bootstrap to load the daemon.`

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"good answer full credit", good, 1},
		{"good answer, reordered phrasing", goodReordered, 1},
		{"good answer, alternate phrasing", goodPhrasing, 1},
		{"wrong subcommand (deprecated load only) scores 0", badWrongSubcommand, 0},
		{"missing domain target and deprecation note loses two thirds", badMissingDomain, 1.0 / 3.0},
		{"'legacy' deprecation cue (A14)", `Use launchctl bootstrap system /Library/LaunchDaemons/com.example.myjob.plist; launchctl load is the legacy subcommand.`, 1},
		{"'superseded' deprecation cue (A14)", `launchctl bootstrap, system domain target, superseded launchctl load.`, 1},
		{"'obsolete' deprecation cue (A14)", `bootstrap system domain target; launchctl load is obsolete.`, 1},
		{"'replaced' deprecation cue (A14)", `launchctl bootstrap system domain target replaced launchctl load.`, 1},
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

func TestMacosPlutilPlistReadTest_Eval(t *testing.T) {
	tc := macosPlutilPlistReadTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct plutil -p", `plutil -p /Library/Preferences/com.example.app.plist`, 1},
		{"prose wrapped", `Run: plutil -p /Library/Preferences/com.example.app.plist to pretty-print it.`, 1},
		{"quoted path", `plutil -p "/Library/Preferences/com.example.app.plist"`, 1},
		{"wrong tool entirely (cat prints binary garbage)", `cat /Library/Preferences/com.example.app.plist`, 0},
		{"plutil without -p just validates, does not print", `plutil /Library/Preferences/com.example.app.plist`, 0},
		{"plutil -convert xml1 to stdout is a materially correct alternative (A2)", `plutil -convert xml1 -o - /Library/Preferences/com.example.app.plist`, 1},
		{"defaults read is a materially correct alternative (A2)", `defaults read com.example.app`, 1},
		{"mentions pretty-print in prose without invoking -p as a bounded flag (A2 bug probe)", `plutil can pretty-print the plist at /Library/Preferences/com.example.app.plist, though I don't recall the exact flag.`, 0},
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

func TestMacosMdfindSpotlightTest_Eval(t *testing.T) {
	tc := macosMdfindSpotlightTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"onlyin with query", `mdfind -onlyin ~ "invoice"`, 1},
		{"name flag", `mdfind -onlyin ~ -name invoice`, 1},
		{"prose wrapped", `Run mdfind -onlyin ~ invoice to search the Spotlight index.`, 1},
		{"uses find instead of mdfind, walks the filesystem, gated to 0 (A13)", `find ~ -iname "*invoice*"`, 0},
		{"mdfind used but searching for the wrong term entirely", `mdfind -onlyin ~ "receipt"`, 0.5},
		{"find command echoing both prompt-supplied tokens must not pass the gate (A13 bug probe)", `find ~ -name "*invoice*"`, 0},
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

func TestMacosAPFSSnapshotTest_Eval(t *testing.T) {
	tc := macosAPFSSnapshotTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"both commands", "Create: tmutil snapshot\nList: tmutil listlocalsnapshots /", 1},
		{"prose wrapped", "Run `tmutil snapshot` to create one, then `tmutil listlocalsnapshots /` to list them.", 1},
		{"listlocalsnapshots without explicit tmutil prefix restated", "First tmutil snapshot, then check with listlocalsnapshots /.", 1},
		{"only creates, never lists", "Just run tmutil snapshot.", 0.5},
		{"wrong tool (diskutil) mentioned instead of tmutil", "Use diskutil apfs snapshot / to create one.", 0},
		{"current documented localsnapshot verb (A1)", "Create: tmutil localsnapshot\nList: tmutil listlocalsnapshots /", 1},
		{"localsnapshot only, never lists (A1)", "Just run tmutil localsnapshot.", 0.5},
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

func TestMacosSSHIdentityAgentTest_Eval(t *testing.T) {
	tc := macosSSHIdentityAgentTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{
			name:     "correct directive and socket",
			response: `IdentityAgent "~/Library/Group Containers/2BUA8C4S2C.com.1password/t/agent.sock"`,
			want:     1,
		},
		{
			name:     "inside a Host block, prose wrapped",
			response: "Add this under your Host block:\nHost *\n  IdentityAgent ~/Library/Group Containers/2BUA8C4S2C.com.1password/t/agent.sock",
			want:     1,
		},
		{
			name:     "shortened socket mention still credited",
			response: "Set IdentityAgent to point at 1Password's agent.sock in its Group Containers path.",
			want:     1,
		},
		{
			name:     "sets SSH_AUTH_SOCK env var instead of ssh_config directive",
			response: `export SSH_AUTH_SOCK=~/Library/Group Containers/2BUA8C4S2C.com.1password/t/agent.sock`,
			want:     0.5,
		},
		{
			name:     "wrong directive entirely",
			response: `Add IdentityFile ~/.ssh/id_ed25519 to your config.`,
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

func TestMacosCaffeinateTest_Eval(t *testing.T) {
	tc := macosCaffeinateTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"bare invocation", `caffeinate ./migrate.sh`, 1},
		{"with system-sleep flag", `caffeinate -s ./migrate.sh`, 1},
		{"prose wrapped", "Run `caffeinate -i ./migrate.sh` and it stops holding the assertion once migrate.sh exits.", 1},
		{"wrong script named, not the target, scores 0 now that migrate.sh must be on the same line (A11)", `caffeinate ./other-script.sh`, 0},
		{"unrelated tool, does not auto-release on exit", `Just run pmset noidle before starting your script.`, 0},
		{"backgrounded caffeinate is a broken answer and must not pass (A11 bug probe)", `caffeinate & ./migrate.sh`, 0},
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

func TestMacosStatBSDGNUTest_Eval(t *testing.T) {
	tc := macosStatBSDGNUTest()

	tests := []struct {
		name     string
		response string
		want     float64
	}{
		{"correct BSD form and GNU flag", `stat -f%z file.txt` + "\nGNU stat uses -c instead.", 1},
		{"quoted format string", `stat -f "%z" file.txt, and on Linux GNU stat uses -c.`, 1},
		{"prose wrapped", "Run stat -f%z file.txt on macOS; the GNU equivalent flag is -c.", 1},
		{"correct BSD command but never names the GNU flag", `stat -f%z file.txt`, 0.5},
		{"GNU -c%s syntax used on macOS scores half: -c token matches but -f BSD form is missing", `stat -c%s file.txt`, 0.5},
		{"single-quoted format string (A4)", `stat -f '%z' file.txt, and on Linux GNU stat uses -c.`, 1},
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

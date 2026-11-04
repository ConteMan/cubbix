package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type lidTestExit int

func (e lidTestExit) Error() string { return "private-process-error" }
func (e lidTestExit) ExitCode() int { return int(e) }

func TestLidInstallDiagnosticsAreBoundedAndPrivate(t *testing.T) {
	for _, tc := range []struct {
		output, step, failure string
		script, command       int
	}{
		{"0:1: execution error: cubbix-step=bootstrap\rcubbix-command-exit=37\rcubbix-error=bootstrap-failed\rcubbix-step=bootstrap\rcubbix-exit=1 (1)\rprivate-account-and-path", "bootstrap", "bootstrap-failed", 1, 37},
		{"cubbix-step=copy\r\ncubbix-step=verify\r\ncubbix-exit=9", "verify", "script-failed", 9, -1},
		{"cubbix-step=private-account\ncubbix-error=private-path\ncubbix-exit=999999999999999999999", "authorize", "script-failed", -1, -1},
		{"User canceled. (-128)", "authorize", "authorization-cancelled", -1, -1},
	} {
		err := lidAdminResult([]byte(tc.output), lidTestExit(1))
		var failure *lidInstallFailure
		if !errors.As(err, &failure) {
			t.Fatal(err)
		}
		d := failure.diagnostic
		if d.step != tc.step || d.failure != tc.failure || d.exit == nil || *d.exit != 1 {
			t.Fatal(d)
		}
		for _, pair := range []struct {
			got  *int
			want int
		}{{d.scriptExit, tc.script}, {d.commandExit, tc.command}} {
			if pair.want < 0 && pair.got != nil || pair.want >= 0 && (pair.got == nil || *pair.got != pair.want) {
				t.Fatal(d)
			}
		}
		if (tc.failure == "authorization-cancelled") != errors.Is(err, errLidDenied) {
			t.Fatal(err)
		}
	}
	if lidAdminResult([]byte("private-output"), nil) != nil {
		t.Fatal("successful script failed")
	}
}

func TestLidInstallDiagnosticsSurvivePollingAndClearOnRetry(t *testing.T) {
	a, sleep, _, svc, _ := lidTestApp(t)
	svc.status, svc.after = svcNotRunning, svcNotRunning
	svc.registerError = lidAdminResult([]byte("cubbix-error=unload-timeout\rcubbix-step=unload-wait\rcubbix-exit=1\rprivate-output"), lidTestExit(1))
	if err := a.lid.enable(time.Time{}, 20, true); err == nil {
		t.Fatal("install succeeded")
	}
	before := a.cliState().Awake.Lid
	if before.InstallStep != "unload-wait" || before.InstallError != "unload-timeout" || before.InstallExitCode == nil || *before.InstallExitCode != 1 || len(sleep.writes) != 0 {
		t.Fatal(before)
	}
	a.lid.Inspect()
	after := a.cliState().Awake.Lid
	if after.InstallStep != before.InstallStep || after.InstallError != before.InstallError {
		t.Fatal(after)
	}
	data, _ := json.Marshal(after)
	if strings.Contains(string(data), "private-") || !strings.Contains(string(data), `"installScriptExitCode":1`) {
		t.Fatal(string(data))
	}
	svc.registerError = nil
	svc.status = svcNotRunning
	if err := a.lid.enable(time.Time{}, 20, true); err != nil {
		t.Fatal(err)
	}
	after = a.cliState().Awake.Lid
	if after.InstallError != "" || after.InstallStep != "confirm" || after.InstallExitCode == nil || *after.InstallExitCode != 0 || after.InstallScriptExitCode != nil {
		t.Fatal(after)
	}
}

// Every external executable is replaced before execution. All file redirects
// land in a temporary directory; the fixtures never install or signal anything.
func TestLidInstallerPhaseFailureMarkers(t *testing.T) {
	for _, phase := range []string{"copy", "verify", "replace", "plist", "success"} {
		t.Run(phase, func(t *testing.T) {
			dir := t.TempDir()
			fixture := filepath.Join(dir, "command-fixture")
			body := `#!/bin/sh
set -eu
case "$1" in
 install) [ "$LID_TEST_PHASE" != copy ] || exit 9 ;;
 codesign) [ "$LID_TEST_PHASE" != verify ] || exit 9 ;;
 mv) [ "$LID_TEST_PHASE" != replace ] || exit 9 ;;
 chown) [ "$LID_TEST_PHASE" != plist ] || exit 9 ;;
 cat) while IFS= read -r line; do :; done ;;
 launchctl)
  case "$2" in
   print) [ -f "$LID_TEST_LOADED" ] ;;
   bootstrap) : > "$LID_TEST_LOADED" ;;
  esac ;;
esac
`
			if err := os.WriteFile(fixture, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			script := lidInstallScript
			for _, name := range []string{"/usr/bin/install", "/usr/bin/codesign", "/bin/rm", "/bin/mv", "/bin/cat", "/usr/sbin/chown", "/bin/chmod", "/bin/launch" + "ctl", "/bin/sleep"} {
				script = strings.ReplaceAll(script, name, "'"+fixture+"' "+filepath.Base(name))
			}
			script = strings.ReplaceAll(script, lidPlistPath, filepath.Join(dir, "helper.plist"))
			cmd := exec.Command("/bin/sh", "-c", script)
			cmd.Env = append(os.Environ(), "LID_TEST_PHASE="+phase, "LID_TEST_LOADED="+filepath.Join(dir, "loaded"))
			out, err := cmd.CombinedOutput()
			if phase == "success" {
				if err != nil || !strings.Contains(string(out), "cubbix-step=confirm\ncubbix-exit=0") {
					t.Fatal(err, string(out))
				}
				return
			}
			var failure *lidInstallFailure
			if !errors.As(lidAdminResult(out, err), &failure) || failure.diagnostic.step != phase || failure.diagnostic.scriptExit == nil {
				t.Fatal(err, string(out))
			}
			if strings.Contains(string(out), "cubbix-step=bootstrap") {
				t.Fatal("failed trust stage reached bootstrap")
			}
		})
	}
}

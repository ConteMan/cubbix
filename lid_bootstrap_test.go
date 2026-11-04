package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Execute only the launchd tail, with EVERY external executable replaced by a
// fixture. Never execute the installer, a system service command, or a real wait.
func TestLidBootstrapWithSimulatedLaunchd(t *testing.T) {
	launchctl := "/bin/launch" + "ctl"
	start := strings.Index(lidInstallScript, launchctl+" bootout")
	if start < 0 {
		t.Fatal("missing bootout")
	}
	tail := lidInstallScript[start:]
	for _, line := range strings.Split(tail, "\n") {
		for _, word := range strings.Fields(line) {
			word = strings.TrimSuffix(word, ";")
			if strings.HasPrefix(word, "/") && word != launchctl && word != "/bin/sleep" && word != lidPlistPath {
				t.Fatal("unexpected executable/path in simulated tail", word)
			}
		}
	}
	for _, tc := range []struct {
		name                    string
		pending, failures       string
		missing                 bool
		wantError               bool
		bootstraps, unloadWaits int
	}{
		{"first-install", "0", "0", false, false, 1, 0},
		{"async-unload-and-retry", "2", "2", false, false, 3, 2},
		{"second-click-residue", "1", "1", false, false, 2, 1},
		{"unload-timeout", "99", "0", false, true, 0, 50},
		{"bootstrap-exhausted", "0", "99", false, true, 5, 0},
		{"success-without-loaded-service", "0", "0", true, true, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			mock := filepath.Join(dir, "launchd-fixture")
			body := `#!/bin/sh
set -eu
printf '%s\n' "$1" >> "$LID_TEST_DIR/events"
case "$1" in
  bootout)
    echo "$LID_TEST_PENDING" > "$LID_TEST_DIR/pending"
    echo 0 > "$LID_TEST_DIR/attempts"
    exit 3 ;;
  print)
    IFS= read -r pending < "$LID_TEST_DIR/pending"
    if [ "$pending" -gt 0 ]; then
      echo "$((pending - 1))" > "$LID_TEST_DIR/pending"
      exit 0
    fi
    [ -f "$LID_TEST_DIR/loaded" ] ;;
  bootstrap)
    IFS= read -r attempts < "$LID_TEST_DIR/attempts"
    attempts=$((attempts + 1))
    echo "$attempts" > "$LID_TEST_DIR/attempts"
    if [ "$attempts" -le "$LID_TEST_FAILURES" ]; then
      if [ "$attempts" -eq 1 ]; then exit 5; else exit 37; fi
    fi
    if [ "$LID_TEST_MISSING" = false ]; then echo loaded > "$LID_TEST_DIR/loaded"; fi ;;
  *) exit 99 ;;
esac
`
			if err := os.WriteFile(mock, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			pause := filepath.Join(dir, "wait-fixture")
			if err := os.WriteFile(pause, []byte("#!/bin/sh\nprintf 'sleep %s\\n' \"$1\" >> \"$LID_TEST_DIR/events\"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
			program := "set -eu\n" + strings.ReplaceAll(strings.ReplaceAll(tail, launchctl, quote(mock)), "/bin/sleep", quote(pause))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "/bin/sh", "-c", program)
			missing := "false"
			if tc.missing {
				missing = "true"
			}
			cmd.Env = append(os.Environ(), "LID_TEST_DIR="+dir, "LID_TEST_PENDING="+tc.pending, "LID_TEST_FAILURES="+tc.failures, "LID_TEST_MISSING="+missing)
			out, err := cmd.CombinedOutput()
			if ctx.Err() != nil || (err != nil) != tc.wantError {
				t.Fatalf("unexpected completion: %v %s", err, out)
			}
			data, err := os.ReadFile(filepath.Join(dir, "events"))
			if err != nil {
				t.Fatal(err)
			}
			events := string(data)
			if !strings.HasPrefix(events, "bootout\nprint\n") || strings.Count(events, "bootstrap\n") != tc.bootstraps || strings.Count(events, "sleep 0.1\n") != tc.unloadWaits {
				t.Fatal("unexpected unload/retry sequence", events)
			}
			if tc.bootstraps > 0 {
				before, _, _ := strings.Cut(events, "bootstrap\n")
				if strings.Count(before, "sleep 0.1\n") != tc.unloadWaits || strings.Count(before, "print\n") != tc.unloadWaits+1 {
					t.Fatal("bootstrap ran before unload confirmation", events)
				}
			}
			if tc.bootstraps > 0 && strings.Count(events, "sleep 1\n") != tc.bootstraps-1 {
				t.Fatal("missing bootstrap backoff", events)
			}
			if !tc.wantError && !strings.HasSuffix(events, "bootstrap\nprint\n") {
				t.Fatal("missing final load confirmation", events)
			}
		})
	}
}

func lidTestReadyClock(c *lidController) *int {
	now := time.Now()
	waits := 0
	c.now = func() time.Time { return now }
	c.wait = func(d time.Duration) { waits++; now = now.Add(d) }
	return &waits
}

func TestLidInstallSuccessRequiresReadyHelper(t *testing.T) {
	for _, status := range []lidServiceStatus{svcNotRegistered, svcNotRunning, svcEnabled, svcBroken} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			a, sleep, _, svc, conn := lidTestApp(t)
			svc.status, svc.after = svcNotRunning, status
			conn.gone = true
			waits := lidTestReadyClock(a.lid)
			var notices []string
			a.notifyBattery = func(_, text string) { notices = append(notices, text) }
			session := a.selectedSession()
			session.Lid = true
			if err := a.startAwake(session); err == nil || err.Error() != "LidHelperNotRunning" {
				t.Fatal("installer exit was mistaken for readiness", err)
			}
			if *waits != 100 || svc.reads < 100 || svc.registers != 1 || a.lid.snapshot().state != lidBlocked || a.lid.snapshot().online || sleep.value != 0 || len(sleep.writes) != 0 || a.lid.markerExists() || a.config.AwakeLid || a.awake.snapshot().Session.Lid || len(notices) == 0 {
				t.Fatal("startup failure was hidden or persisted", *waits, a.lid.snapshot(), notices)
			}
			// Polling must remain read-only; the next manual click authorizes repair.
			a.lid.Inspect()
			if svc.registers != 1 {
				t.Fatal("polling installed")
			}
			conn.gone = false
			svc.status = svcNotRunning
			a.toggleLid()
			if svc.registers != 2 || a.lid.snapshot().state != lidOn || !a.config.AwakeLid {
				t.Fatal("manual retry did not authorize and enable", a.lid.snapshot())
			}
		})
	}
}

func TestLidDelayedReadinessAndApproval(t *testing.T) {
	for _, approval := range []bool{false, true} {
		a, sleep, _, svc, conn := lidTestApp(t)
		svc.status, svc.after = svcNotRunning, svcNotRunning
		waits := lidTestReadyClock(a.lid)
		a.lid.dial = func() (lidConnection, error) {
			if *waits < 2 {
				return nil, errors.New("not ready")
			}
			return conn, nil
		}
		wait := a.lid.wait
		a.lid.wait = func(d time.Duration) {
			wait(d)
			if approval {
				svc.status = svcRequiresApproval
			}
		}
		err := a.lid.enable(time.Time{}, 20, true)
		if approval {
			if err == nil || err.Error() != "LidAuthNeeded" || *waits != 1 || len(sleep.writes) != 0 {
				t.Fatal(err, *waits)
			}
		} else if err != nil || *waits != 2 || sleep.value != 1 || a.lid.snapshot().service != svcEnabled {
			t.Fatal("delayed authenticated helper not accepted", err, *waits)
		}
	}
}

func TestLidInspectNotRunningExplainsAndNeverInstalls(t *testing.T) {
	for _, status := range []lidServiceStatus{svcNotRunning, svcBroken, svcRequiresApproval} {
		a, sleep, _, svc, conn := lidTestApp(t)
		svc.status = status
		conn.gone = true
		a.lid.Inspect()
		want := map[lidServiceStatus]string{svcNotRunning: "LidHelperNotRunning", svcBroken: "LidServiceBroken", svcRequiresApproval: "LidAuthNeeded"}[status]
		if a.lid.snapshot().state != lidBlocked || a.lidReason() != want || svc.registers != 0 || len(sleep.writes) != 0 {
			t.Fatal(a.lid.snapshot())
		}
		for _, locale := range []string{"zh-CN", "en"} {
			if !panelView(a, locale).HasText(a.text(locale, want)) || !strings.Contains(a.awakeStatus(locale, time.Now()), a.text(locale, want)) {
				t.Fatal("missing localized feedback", locale)
			}
		}
	}
}

func TestLidBrokenInstallNotifiesAndCancelCanRetry(t *testing.T) {
	for _, denied := range []bool{false, true} {
		a, sleep, _, svc, _ := lidTestApp(t)
		svc.status, svc.after = svcNotRunning, svcNotRunning
		svc.registerError = errors.New("installer failed")
		want := "LidServiceBroken"
		if denied {
			svc.registerError = errLidDenied
			want = "LidDenied"
		}
		var notices []string
		a.notifyBattery = func(_, text string) { notices = append(notices, text) }
		if err := a.awake.start(a.selectedSession()); err != nil {
			t.Fatal(err)
		}
		err := a.enableLid(time.Time{}, true)
		if err == nil || err.Error() != want || len(notices) != 1 || len(sleep.writes) != 0 {
			t.Fatal("missing installation failure feedback", err, notices)
		}
		a.notifyLid(want)
		if len(notices) != 1 {
			t.Fatal("duplicate failure notification")
		}
		svc.registerError = nil
		err = a.enableLid(time.Time{}, true)
		if err != nil || svc.registers != 2 || a.lid.snapshot().state != lidOn {
			t.Fatal("retry failed", err)
		}
	}
}

func TestLidFreshInstallIsNotReportedAsInstalled(t *testing.T) {
	a, _, _, svc, conn := lidTestApp(t)
	svc.status = svcNotRegistered
	conn.gone = true
	a.lid.Inspect()
	if a.lidReason() != "" || a.lid.snapshot().online || svc.registers != 0 {
		t.Fatal(a.lid.snapshot())
	}
}

func TestLidNotRunningNeverAuthorizesQuietly(t *testing.T) {
	a, sleep, _, svc, _ := lidTestApp(t)
	svc.status = svcNotRunning
	if err := a.lid.enable(time.Time{}, 20, false); err == nil || err.Error() != "LidAuthNeeded" {
		t.Fatal(err)
	}
	req, ok := parsePaletteURL("cubbix://awake?minutes=120&lid=1", true, true, jsonFileLimit)
	if !ok {
		t.Fatal("invalid test URL")
	}
	if err := a.runURL(req); err == nil {
		t.Fatal("URL accepted a stopped helper")
	}
	if svc.registers != 0 || len(sleep.writes) != 0 {
		t.Fatal("unprompted authorization")
	}
}

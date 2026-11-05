package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

type retryStopChild struct {
	done chan error
	kill func() error
}

func (c *retryStopChild) Wait() error { return <-c.done }
func (c *retryStopChild) Kill() error { return c.kill() }

func TestModuleCloseRetriesFailedKill(t *testing.T) {
	a, sleep, _, svc, _ := lidTestApp(t)
	child := &retryStopChild{done: make(chan error, 1)}
	t.Cleanup(func() { close(child.done) })
	child.kill = func() error { return errors.New("kill failed") }
	a.awake.launch = func([]string) (awakeChild, error) { return child, nil }
	session := awakeSession{Mode: "screen", End: time.Now().Add(time.Hour), Lid: true}
	if err := a.startAwake(session); err != nil {
		t.Fatal(err)
	}
	a.changeAwakeEnabled(false)
	if !a.config.AwakeEnabled || !a.awake.snapshot().Active || a.moduleError != "AwakeStopError" {
		t.Fatalf("failed kill discarded the session: %+v, %q", a.awake.snapshot(), a.moduleError)
	}
	if sleep.value != 0 || a.lid.markerExists() || svc.unregisters != 0 {
		t.Fatal("failed kill did not preserve the safe intermediate state")
	}
	child.kill = func() error { child.done <- errors.New("killed"); return nil }
	a.changeAwakeEnabled(false)
	if a.config.AwakeEnabled || a.awake.snapshot().Active || svc.unregisters != 1 {
		t.Fatal("the second close failed to retry and confirm the child exit")
	}
}

func TestConfirmedStopDoesNotWaitForMainThreadNotification(t *testing.T) {
	k, child := fakeAwake(time.Now())
	if err := k.start(awakeSession{Mode: "screen"}); err != nil {
		t.Fatal(err)
	}
	// Simulate a child-exit notification waiting for the main thread. The
	// main thread must be able to finish its own stop before consuming it.
	blocked := make(chan struct{})
	defer func() {
		select {
		case <-blocked:
		default:
			close(blocked)
		}
	}()
	notified := make(chan struct{}, 2)
	k.changed = func() { notified <- struct{}{}; <-blocked }
	// First put the existing exit watcher inside its UI notification. This
	// makes the scheduling deterministic: confirmation must observe the exit
	// even though that notification has not yet returned.
	child.done <- errors.New("exited")
	select {
	case <-notified:
	case <-time.After(time.Second):
		t.Fatal("the child watcher did not reach its notification")
	}
	result := make(chan error, 1)
	go func() { result <- k.stopConfirmed(50 * time.Millisecond) }()
	select {
	case <-notified:
	case <-time.After(time.Second):
		t.Fatal("confirmation could not finish before UI notifications")
	}
	// stopConfirmed's own notify also waits. Release notifications now; a
	// timeout means the watcher incorrectly kept the exit signal behind one.
	close(blocked)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("confirmation was held behind a UI notification")
	}
}

func TestOrdinaryModuleCloseRunsOffMainThread(t *testing.T) {
	a := awakeTestApp(t)
	a.moduleAsync = true
	a.awake.changed = nil
	child := &retryStopChild{done: make(chan error)}
	child.kill = func() error { return nil }
	defer close(child.done)
	a.awake.launch = func([]string) (awakeChild, error) { return child, nil }
	if err := a.startAwake(awakeSession{Mode: "screen"}); err != nil {
		t.Fatal(err)
	}
	completed := make(chan func(), 1)
	a.urlDispatch = func(fn func()) { completed <- fn }
	previous := moduleStopTimeout
	moduleStopTimeout = 50 * time.Millisecond
	defer func() { moduleStopTimeout = previous }()
	a.changeAwakeEnabled(false)
	if !a.moduleClosing || !a.config.AwakeEnabled {
		t.Fatal("ordinary close blocked or committed before confirmation")
	}
	for _, target := range []string{"awake", "json", "palette", "panel"} {
		a.beginTileShortcutRecording(target)
		if a.shortcutRecording || a.shortcutSuspended {
			t.Fatalf("%s recorder interrupted the close", target)
		}
	}
	select {
	case finish := <-completed:
		finish()
	case <-time.After(time.Second):
		t.Fatal("ordinary close did not finish within its budget")
	}
	if a.moduleClosing || !a.config.AwakeEnabled || a.moduleError != "AwakeStopError" {
		t.Fatal("unconfirmed async close did not preserve the module")
	}
}

func TestModuleCloseGatesLidEntryPoints(t *testing.T) {
	a, sleep, _, svc, _ := lidTestApp(t)
	a.moduleClosing = true
	// Taking the controller lock makes accidental main-thread IPC visible
	// immediately as a stuck test, without any real helper or pmset call.
	a.lid.mu.Lock()
	done := make(chan struct{})
	go func() {
		a.pollLid()
		a.syncLid()
		a.toggleLid()
		a.forceLid()
		if err := a.stopAwake(); err == nil || err.Error() != "ModuleBusy" {
			t.Error("stop bypassed the close gate")
		}
		close(done)
	}()
	select {
	case <-done:
		a.lid.mu.Unlock()
	case <-time.After(time.Second):
		a.lid.mu.Unlock()
		<-done
		t.Fatal("a close entry point blocked on the controller lock")
	}
	if sleep.writes != nil || svc.unregisters != 0 || a.lidPolling {
		t.Fatal("a gated operation touched the fake system")
	}
}

func TestModuleCloseRefusesCachedPaletteActions(t *testing.T) {
	for _, closing := range []bool{false, true} {
		a := awakeTestApp(t)
		cmd := findCommand(a.commands(), "awake.start")
		ran := false
		action := &paletteAction{ID: "awake.start.lid", Run: func(*application) error { ran = true; return nil }}
		a.palette = &paletteState{mode: paletteActionMode, actionsCommand: cmd, selected: 0,
			rows: []paletteRow{{Kind: paletteRowAction, Action: action}}}
		a.moduleClosing = closing
		a.config.AwakeEnabled = closing
		a.runPalette()
		if ran {
			t.Fatal("a cached action bypassed the module state")
		}
		if closing && a.palette.runError != "ModuleBusy" {
			t.Fatal("a busy action lost its error")
		}
	}
}

func TestCheckedAtRestoredUTCUsesLocalCalendar(t *testing.T) {
	a := awakeTestApp(t)
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// DST starts on March 8: local 00:30 the following morning is less than
	// 24 hours after local 00:15 on the previous calendar day.
	now := time.Date(2026, 3, 9, 0, 30, 0, 0, zone)
	for _, test := range []struct {
		when time.Time
		want string
	}{
		{now.Add(-15 * time.Minute), "00:15"},
		{time.Date(2026, 3, 8, 0, 15, 0, 0, zone), "Yesterday 00:15"},
		{time.Date(2026, 2, 28, 23, 15, 0, 0, zone), "Feb 28, 23:15"},
		{time.Date(2025, 12, 31, 23, 15, 0, 0, zone), "Dec 31, 2025, 23:15"},
	} {
		if got := a.checkedAtText("en", test.when.UTC(), now); got != test.want {
			t.Fatalf("restored UTC date = %q, want %q", got, test.want)
		}
	}
}

func TestCanceledCheckCannotStampLateResult(t *testing.T) {
	for _, result := range []error{nil, errors.New("transport closed")} {
		returned := make(chan struct{})
		u := newUpdateManager(func(ctx context.Context) (*updateCandidate, error) {
			<-ctx.Done()
			close(returned)
			return nil, result
		}, nil, nil)
		u.automatic = true
		u.request(false)
		u.setAutomatic(false)
		<-returned
		eventually(t, time.Second, func() bool { return !u.isBusy() })
		if !u.lastCheck().IsZero() || u.snapshot().ErrorKey != "" {
			t.Fatal("a canceled request stamped its late check result")
		}
		u.close()
	}
}

type closeReviewService struct {
	lidService
	unregister func() error
}

func (s closeReviewService) Unregister() error { return s.unregister() }

func TestModuleCloseUnregisterFailureKeepsEntryPoints(t *testing.T) {
	a, sleep, _, svc, _ := lidTestApp(t)
	a.registerAwakeShortcut()
	child := &retryStopChild{done: make(chan error, 1)}
	t.Cleanup(func() { close(child.done) })
	child.kill = func() error {
		if sleep.value != 0 || a.lid.markerExists() || !a.config.AwakeEnabled || a.ownedShortcut("awake") == "" {
			t.Error("child kill ran before confirmed lid recovery or after premature commit")
		}
		child.done <- errors.New("killed")
		return nil
	}
	a.awake.launch = func([]string) (awakeChild, error) { return child, nil }
	if err := a.startAwake(awakeSession{Mode: "screen", End: time.Now().Add(time.Hour), Lid: true}); err != nil {
		t.Fatal(err)
	}
	a.lid.service = closeReviewService{lidService: svc, unregister: func() error {
		if a.awake.snapshot().Active || sleep.value != 0 || a.lid.markerExists() || !a.config.AwakeEnabled || a.ownedShortcut("awake") == "" {
			t.Error("unregister ran before confirmed stop or after premature commit")
		}
		return errors.New("authorization canceled")
	}}
	a.changeAwakeEnabled(false)
	if !a.config.AwakeEnabled || a.moduleClosing || a.moduleError != "LidRestoreFailed" || a.ownedShortcut("awake") == "" {
		t.Fatal("unregister failure lost the module's entry points")
	}
	if a.awake.snapshot().Active || a.config.AwakeSession != nil || sleep.value != 0 {
		t.Fatal("unregister failure resurrected the stopped session")
	}
	saved, err := loadConfig(a.path)
	if err != nil || !saved.AwakeEnabled || saved.AwakeSession != nil {
		t.Fatal("unregister failure persisted an unsafe module/session state")
	}
}

func TestCheckTimePersistenceRetriesAfterSaveFailure(t *testing.T) {
	a := awakeTestApp(t)
	parent := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(parent, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.path = filepath.Join(parent, "config.toml")
	checked := time.Now().Add(-time.Minute).UTC()
	a.updates.state.CheckedAt = checked
	a.moduleError = "LidRestoreFailed"
	a.syncUpdateCheck()
	if a.settingsError != "ConfigError" || !a.updates.lastCheck().Equal(checked) {
		t.Fatal("failed save lost its check time or error")
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	a.syncUpdateCheck()
	saved, err := loadConfig(a.path)
	if err != nil || saved.UpdateLastCheck != checked.Format(time.RFC3339Nano) || a.settingsError != "" || a.moduleError != "LidRestoreFailed" {
		t.Fatalf("save retry did not persist the time independently of recovery: %+v, %v", saved, err)
	}
	// A progress/notes notification with the same completed check never
	// creates another file; the committed completion is already persisted.
	if err := os.Remove(a.path); err != nil {
		t.Fatal(err)
	}
	a.syncUpdateCheck()
	if _, err := os.Stat(a.path); !os.IsNotExist(err) {
		t.Fatal("an unchanged check completion was saved twice")
	}
}

func TestClosingRecorderShowsBusyReason(t *testing.T) {
	a := awakeTestApp(t)
	a.page, a.moduleClosing = settingsAwake, true
	tt := ui.NewTester(a.view, 720, 388)
	tt.SetLocale("en")
	tt.Frame()
	if !hasAccessibleText(tt, a.text("en", "ModuleBusy")) {
		t.Fatal("the disabled recorder lost its busy explanation")
	}
	if _, err := os.Stat(a.path); err == nil {
		t.Fatal("rendering the closing state wrote configuration")
	}
}

func TestModuleClosePreservesUserFocus(t *testing.T) {
	a := awakeTestApp(t)
	a.page = settingsAwake
	initial, kept := true, false
	tt := ui.NewTester(func(c *ui.Context) {
		_, tk := applyTheme(c)
		other := ui.ButtonBase(c.Key("other-control")).Label("another control")
		if initial {
			other.Focus()
		}
		a.awakeModulePage(c, tk, "en", newSettingsLayout(a, settingsAwake), "")
		kept = other.FocusWithin()
	}, 720, 388)
	tt.Frame()
	initial = false
	a.config.AwakeEnabled = false
	a.requestModuleFocus()
	tt.Frame()
	if !kept || a.focusModuleSwitch {
		t.Fatal("the completed close stole an existing focus or kept a pending request")
	}
	a.page = settingsGeneral
	a.requestModuleFocus()
	if a.focusModuleSwitch {
		t.Fatal("a close scheduled focus on a page the user had left")
	}
}

func TestLateRecorderInputCannotRegisterAShortcut(t *testing.T) {
	a := awakeTestApp(t)
	registered := 0
	a.registerShortcut = func(string, func()) error { registered++; return nil }
	previous := a.config.AwakeShortcut
	a.recordShortcut(ui.Ctrl|ui.Alt|ui.Cmd, ui.KeyL)
	if registered != 0 || a.config.AwakeShortcut != previous {
		t.Fatal("late input committed a shortcut without an active recorder")
	}
}

func TestRecordingCancelPreservesConflictingIncumbent(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(fmt.Sprint(commit), func(t *testing.T) {
			a := awakeTestApp(t)
			registered := map[string]bool{}
			calls := map[string]int{}
			a.registerShortcut = func(acc string, _ func()) error {
				calls[acc]++
				if registered[acc] {
					return errors.New("already registered")
				}
				registered[acc] = true
				return nil
			}
			a.unregisterShortcut = func(acc string) { delete(registered, acc) }
			a.registerAwakeShortcut()
			acc := a.config.AwakeShortcut
			a.changeAwakeEnabled(false)
			a.config.JSONShortcut = acc
			a.registerJSONShortcut()
			a.changeAwakeEnabled(true)
			if a.ownedShortcut("json") != acc || a.ownedShortcut("awake") != "" {
				t.Fatal("setup did not preserve the incumbent")
			}
			before := calls[acc]
			a.beginTileShortcutRecording("palette")
			if commit {
				a.recordShortcut(ui.Ctrl|ui.Alt|ui.Cmd, ui.KeyL)
			} else {
				a.cancelShortcutRecording()
			}
			if !registered[acc] || a.ownedShortcut("json") != acc || a.ownedShortcut("awake") != "" {
				t.Fatal("ending an unrelated recording lost the incumbent's shortcut")
			}
			if calls[acc] != before+1 || a.suspendedShortcut != nil || a.shortcutSuspended || a.shortcutRecording {
				t.Fatal("resume registered twice or kept suspended state")
			}
			a.changeAwakeEnabled(false)
			if !registered[acc] {
				t.Fatal("closing the failed slot released the resumed incumbent")
			}
		})
	}
}

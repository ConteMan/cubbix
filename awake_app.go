package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/ConteMan/cubbix/internal/awake"
	"math"
	"os/exec"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// moduleStopTimeout bounds how long a close waits for the keep-awake child to
// exit. A stop that cannot be confirmed keeps the module open instead of
// pretending the session ended.
var moduleStopTimeout = 3 * time.Second

func (a *application) awakeChanged() {
	a.syncLid()
	s := a.awake.snapshot()
	if a.panel != nil {
		a.panel.errorKey = ""
	}
	a.config.AwakeSession = nil
	if s.Active {
		copy := s.Session
		a.config.AwakeSession = &copy
	}
	a.save()
	a.refreshAwake()
}
func (a *application) refreshAwake() {
	if a.tray != nil {
		name := "assets/menubar-template.tiff"
		if a.awake.snapshot().Active {
			name = "assets/menubar-template-active.tiff"
		}
		data, _ := assets.ReadFile(name)
		_ = a.tray.SetIcon(data, true)
		a.tray.SetToolTip(a.awakeStatus(mygo.App.Locale(), time.Now()))
	}
	a.invalidateWindow()
	a.refreshMenubarPanel()
}
func (a *application) startAwake(s awakeSession) error {
	if a.moduleClosing {
		return errors.New("ModuleBusy")
	}
	if a.lidBusy {
		a.lidLastCode = "lid-busy"
		a.lidMessage = "LidEnableFailed"
		return fmt.Errorf("LidEnableFailed")
	}
	if !a.config.AwakeEnabled {
		return fmt.Errorf("keep awake disabled")
	}
	wanted := s.Lid
	current := a.awake.snapshot()
	if wanted && (a.lid == nil || !a.lid.Available()) {
		return fmt.Errorf("LidUnsupported")
	}
	if !wanted && a.lid != nil {
		state := a.lid.snapshot()
		if current.Session.Lid || state.state == lidOn || state.state == lidRestoreFailed || !state.online && state.ours {
			if err := a.lid.stop(); err != nil {
				a.notifyLid("LidRestoreFailed")
				a.refreshAwake()
				return err
			}
			a.setSessionLid(false)
		}
	}
	// Lid belongs to this selection, not to the previous assertion's state.
	// Keep it through asynchronous enable and the existing-lease fast path.
	if err := a.awake.start(s); err != nil {
		return err
	}
	if wanted {
		if a.lid.snapshot().state == lidOn && (a.lid.deadline.IsZero() || !s.End.IsZero() && !s.End.After(a.lid.deadline)) {
			return nil
		}
		return a.enableLid(s.End, a.lidAuthorize && !a.lidURL)
	}
	return nil
}

// reportAwakeError serves launch restoration, which still reports in
// Settings. Shortcuts, panels and URL actions consume their returned errors.
func (a *application) reportAwakeError(err error) {
	if err != nil {
		a.moduleError = "AwakeStartError"
		a.invalidateWindow()
	}
}
func (a *application) selectedSession() awakeSession {
	s := awakeSession{Lid: a.savedLidPreference(), Mode: a.config.AwakeMode, Minutes: a.config.AwakeMinutes, Until: a.config.AwakeUntil}
	if s.Mode == "" {
		s.Mode = "screen"
	}
	if a.config.AwakeUntil != "" {
		var err error
		s.End, err = awake.UntilTime(a.config.AwakeUntil, time.Now())
		if err != nil {
			s.End = time.Now().Add(-time.Second)
		}
	} else if a.config.AwakeMinutes < 0 {
		s.End = time.Now().Add(-time.Second)
	} else if a.config.AwakeMinutes > 0 {
		s.End = time.Now().Add(time.Duration(a.config.AwakeMinutes) * time.Minute)
	}
	return s
}
func (a *application) toggleAwake() { _ = a.toggleAwakeResult() }

// shortcutToggleAwake is the global shortcut path: it reports through the
// feedback presenter and still returns the result for the caller.
func (a *application) shortcutToggleAwake() {
	a.announceAwake(a.toggleAwakeResult())
}

// toggleAwakeResult is the shared toggle. toggleAwake keeps the old signature
// for silent callers (menu-style actions), which show the new state themselves.
func (a *application) toggleAwakeResult() error {
	// Nothing new may start while a close is still confirming the system state.
	if a.moduleClosing {
		return errors.New("ModuleBusy")
	}
	if !a.config.AwakeEnabled {
		return nil
	}
	if a.awake.snapshot().Active {
		return a.stopAwake()
	}
	return a.startAwake(a.selectedSession())
}
func (a *application) setMode(mode string) error {
	if a.moduleClosing {
		return errors.New("ModuleBusy")
	}
	if mode != "screen" && mode != "tasks" {
		return fmt.Errorf("invalid mode: %q", mode)
	}
	s := a.awake.snapshot()
	if s.Active {
		s.Session.Mode = mode
		if err := a.startAwake(s.Session); err != nil {
			return err
		}
	}
	a.config.AwakeMode = mode
	a.save()
	a.refreshAwake()
	return nil
}
func (a *application) setDuration(minutes int) error {
	if minutes < 0 || minutes > awake.MinutesMaximum {
		return fmt.Errorf("DurationError")
	}
	s := awakeSession{Lid: a.savedLidPreference(), Mode: a.config.AwakeMode, Minutes: minutes}
	if minutes > 0 {
		s.End = time.Now().Add(time.Duration(minutes) * time.Minute)
	}
	if err := a.startAwake(s); err != nil {
		return err
	}
	a.config.AwakeMinutes = minutes
	a.config.AwakeUntil = ""
	a.save()
	a.refreshAwake()
	return nil
}

// ownShortcut records the combination a slot really registered. A failed
// attempt owns nothing, so a later close cannot release somebody else's keys.
func (a *application) ownShortcut(target, acc string) {
	if a.registeredShortcut == nil {
		a.registeredShortcut = map[string]string{}
	}
	a.registeredShortcut[target] = acc
}

func (a *application) ownedShortcut(target string) string {
	if a.registeredShortcut == nil {
		return ""
	}
	return a.registeredShortcut[target]
}

// releaseOwnedShortcut unregisters exactly what this slot registered.
func (a *application) releaseOwnedShortcut(target string) {
	owned := a.ownedShortcut(target)
	if owned == "" {
		return
	}
	delete(a.registeredShortcut, target)
	a.unregisterShortcut(owned)
}

func (a *application) setSlotError(target, key string) {
	switch target {
	case "awake":
		a.shortcutError = key
	case "json":
		a.jsonShortcutError = key
	case "palette":
		a.paletteShortcutError = key
	case "panel":
		a.panelShortcutError = key
	}
}

// registerSlot is the one registration path. It owns what it registers and
// never touches a combination another slot registered.
func (a *application) registerSlot(target, acc string, enabled bool, callback func()) {
	if !enabled {
		a.releaseOwnedShortcut(target)
		a.clearSlotError(target)
		return
	}
	if owned := a.ownedShortcut(target); owned != "" && sameShortcut(owned, acc) {
		a.clearSlotError(target)
		return
	}
	a.releaseOwnedShortcut(target)
	a.clearSlotError(target)
	if reason := a.shortcutConflict(acc, target); reason != "" {
		a.setSlotError(target, reason)
		return
	}
	if err := a.registerShortcut(acc, callback); err != nil {
		a.setSlotError(target, "ShortcutUnavailable")
		return
	}
	a.ownShortcut(target, acc)
}

func (a *application) registerAwakeShortcut() {
	a.registerSlot("awake", a.config.AwakeShortcut, a.config.AwakeEnabled, a.shortcutToggleAwake)
}

// moduleBusy reports a keep-awake operation that must settle first.
func (a *application) moduleBusy() bool {
	return a.moduleClosing || a.lidBusy || a.lidPolling
}

// changeAwakeEnabled applies a module switch target. Turning the module on only
// restores its entry points; it never resumes the session the module had before.
func (a *application) changeAwakeEnabled(target bool) {
	// A recording would otherwise re-register the slot the change is about.
	a.cancelShortcutRecording()
	if a.moduleClosing {
		a.moduleError = "ModuleBusy"
		a.invalidateWindow()
		return
	}
	if target == a.config.AwakeEnabled {
		a.invalidateWindow()
		return
	}
	a.moduleError = ""
	if target {
		a.config.AwakeEnabled = true
		a.registerAwakeShortcut()
		a.save()
		a.refreshModuleSurfaces()
		return
	}
	a.closeAwakeModule()
}

// closeAwakeModule restores the lid, confirms the plain assertion really
// stopped, and only then uninstalls the helper and commits the closed state.
// The order is 004's invariant: never stop keep-awake before the system sleep
// setting is ours again.
func (a *application) closeAwakeModule() {
	if a.lidBusy || a.lidPolling {
		// A running authorization or inspection owns the controller. Refusing
		// here keeps the main thread free instead of waiting on its lock.
		a.moduleError = "ModuleBusy"
		a.invalidateWindow()
		return
	}
	a.moduleClosing = true
	a.moduleGeneration++
	generation := a.moduleGeneration
	a.moduleError = ""
	a.refreshModuleSurfaces()

	lid := a.lid
	needService := false
	if lid != nil && lid.Available() && lid.service != nil {
		needService = lid.service.Status() != svcNotRegistered
	}
	run := func() error {
		var failure error
		if lid != nil {
			failure = lid.stop()
		}
		if failure == nil {
			// The lid is already ours again, so the plain assertion stops
			// without asking the controller to restore it a second time.
			failure = a.awake.stopConfirmed(moduleStopTimeout)
		}
		if failure == nil && needService {
			if err := lid.service.Unregister(); err != nil {
				// The session is already safe; the helper is what is left.
				failure = errors.New("LidRestoreFailed")
			}
		}
		return failure
	}
	if !a.moduleAsync && !a.lidAsync {
		// Tests can run synchronously with injected controllers. Production
		// always uses a worker, including ordinary caffeinate-only sessions.
		a.finishAwakeClose(generation, run())
		return
	}
	go func() {
		failure := run()
		a.urlDispatch(func() { a.finishAwakeClose(generation, failure) })
	}()
}

// finishAwakeClose commits the close on the main thread. A failure never writes
// the closed state: the switch goes back, the recovery entry stays, and no
// already stopped session comes back to life.
func (a *application) finishAwakeClose(generation uint64, failure error) {
	if generation != a.moduleGeneration || a.quitting {
		return
	}
	a.moduleClosing = false
	stopped := !a.awake.snapshot().Active
	if stopped {
		// A session that really ended is never remembered, whatever the rest
		// of the transaction did: reopening the module must not resurrect it.
		a.setSessionLid(false)
		a.config.AwakeSession = nil
	}
	if failure != nil {
		key := "LidRestoreFailed"
		if failure.Error() == "AwakeStopError" {
			key = "AwakeStopError"
		} else if failure.Error() == "ModuleBusy" {
			key = "ModuleBusy"
		}
		a.moduleError = key
		if stopped {
			// The file keeps the confirmed state, never the closed one.
			a.save()
		}
		a.invalidateWindow()
		a.refreshModuleSurfaces()
		return
	}
	a.setSessionLid(false)
	a.config.AwakeEnabled = false
	a.releaseOwnedShortcut("awake")
	a.clearSlotError("awake")
	a.moduleError = ""
	a.requestModuleFocus()
	a.save()
	a.refreshModuleSurfaces()
}

// refreshModuleSurfaces refreshes every surface a module change reaches: the
// settings window, the menu bar icon and tooltip, the card panel and the
// command palette.
func (a *application) refreshModuleSurfaces() {
	a.refreshAwake()
	a.invalidatePaletteAfterModuleChange()
}

// awakeMode names the mode a session runs in.
func (a *application) awakeMode(locale string, session awakeSession) string {
	if session.Mode == "tasks" {
		return a.text(locale, "R05Tasks")
	}
	return a.text(locale, "R05Screen")
}

// awakeSuffix is the tail of a status line: the followed process, a deadline,
// no deadline, or the time left. A followed process and a deadline are read
// before the countdown so neither is hidden by it.
func (a *application) awakeSuffix(locale string, session awakeSession, now time.Time) string {
	text := func(key string) string { return a.text(locale, key) }
	if session.PID > 0 {
		return fmt.Sprintf(text("AwakeFollowing"), session.Name)
	}
	if session.Until != "" {
		return fmt.Sprintf(text("AwakeUntil"), session.Until)
	}
	if session.End.IsZero() {
		return text("R05Unlimited")
	}
	minutes := max(0, int(math.Ceil(session.End.Sub(now).Minutes())))
	return fmt.Sprintf(text("AwakeRemaining"), minutes/60, minutes%60)
}

func (a *application) awakeStatus(locale string, now time.Time) string {
	text := func(key string) string { return a.text(locale, key) }
	s := a.awake.snapshot()
	if a.lid != nil {
		lid := a.lid.snapshot()
		if lid.reason != "" {
			prefix := text("R05Off")
			if s.Active {
				prefix = a.awakeMode(locale, s.Session) + " · " + a.awakeSuffix(locale, s.Session, now)
			}
			return prefix + " · " + text(lid.reason)
		}
	}
	if !s.Active {
		if s.Error != "" {
			return text("R05Off") + " · ⚠ " + text(s.Error)
		}
		return text("R05Off")
	}
	mode := a.awakeMode(locale, s.Session)
	if s.Session.Lid {
		mode += " · " + text("R05Lid")
		// While watching, the 1 belongs to another tool. Keep the status line
		// honest without adding a third tray icon.
		if a.lid != nil && a.lid.snapshot().watching {
			mode += text("R05LidHeld")
		}
	}
	return mode + " · " + a.awakeSuffix(locale, s.Session, now)
}

type powerMonitor struct {
	known, battery bool
	notify         func()
}

func (p *powerMonitor) sample(battery bool, active bool) {
	if p.known && !p.battery && battery && active && p.notify != nil {
		p.notify()
	}
	p.known, p.battery = true, battery
}
func batteryPower(ctx context.Context) (bool, error) {
	data, err := exec.CommandContext(ctx, "/usr/bin/pmset", "-g", "batt").Output()
	if err != nil {
		return false, err
	}
	if strings.Contains(string(data), "'Battery Power'") {
		return true, nil
	}
	if strings.Contains(string(data), "'AC Power'") {
		return false, nil
	}
	return false, fmt.Errorf("unknown power source")
}
func (a *application) startAwakeServices() {
	a.awake.setBeforeStop(func() {
		if a.lid != nil {
			_ = a.lid.stop()
		}
	})
	a.pollLid()
	a.awake.changed = func() {
		mygo.RunOnMain(func() {
			if !a.quitting {
				a.awakeChanged()
			}
		})
	}
	a.restoreAwakeAfterLaunch()
	a.registerAwakeShortcut()
	ctx, cancel := context.WithCancel(context.Background())
	a.stopAwakeServices = cancel
	monitor := powerMonitor{notify: func() {
		mygo.RunOnMain(func() {
			if !a.quitting && a.awake.snapshot().Active && a.notifyBattery != nil {
				a.notifyBattery(a.text(mygo.App.Locale(), "R05BatteryTitle"), a.text(mygo.App.Locale(), "R05BatteryBody"))
			}
		})
	}}
	go func() {
		poll := time.NewTicker(time.Second)
		defer poll.Stop()
		minute := 0
		for {
			if minute%5 == 0 {
				mygo.RunOnMain(func() {
					if !a.quitting {
						a.pollLid()
					}
				})
				sampleCtx, stop := context.WithTimeout(ctx, 2*time.Second)
				battery, err := batteryPower(sampleCtx)
				stop()
				if err == nil {
					monitor.sample(battery, a.awake.snapshot().Active)
				}
			}
			if minute%60 == 0 {
				mygo.RunOnMain(func() {
					if !a.quitting {
						a.refreshAwake()
					}
				})
			}
			select {
			case <-ctx.Done():
				return
			case <-poll.C:
				a.awake.poll()
				minute++
			}
		}
	}()
}

func (a *application) restoreAwakeAfterLaunch() {
	restore := a.config.RestoreAwake && a.config.AwakeEnabled
	session := restoreSession(a.config.AwakeSession, time.Now(), a.awake.alive)
	a.config.RestoreAwake = false
	a.config.AwakeSession = nil
	a.save()
	if restore && session != nil {
		if session.Lid {
			session.Lid = false
			if a.lid != nil {
				a.lidMessage = "R05LidRestored"
			}
		}
		a.reportAwakeError(a.startAwake(*session))
	}
}

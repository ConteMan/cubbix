package main

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

func (a *application) awakeChanged() {
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
	if !a.config.AwakeEnabled {
		return fmt.Errorf("keep awake disabled")
	}
	return a.awake.start(s)
}

// reportAwakeError serves the global shortcut and launch restoration. Panels
// and URL actions consume their returned errors without writing Settings.
func (a *application) reportAwakeError(err error) {
	if err != nil {
		a.settingsError = "AwakeStartError"
		a.invalidateWindow()
	}
}
func (a *application) selectedSession() awakeSession {
	s := awakeSession{Mode: a.config.AwakeMode, Minutes: a.config.AwakeMinutes, Until: a.config.AwakeUntil}
	if s.Mode == "" {
		s.Mode = "screen"
	}
	if a.config.AwakeUntil != "" {
		var err error
		s.End, err = untilTime(a.config.AwakeUntil, time.Now())
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
func (a *application) toggleAwake() {
	if !a.config.AwakeEnabled {
		return
	}
	if a.awake.snapshot().Active {
		a.awake.stop()
	} else {
		a.reportAwakeError(a.startAwake(a.selectedSession()))
	}
}
func (a *application) setMode(mode string) error {
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
	if minutes < 0 || minutes > awakeMinutesMaximum {
		return fmt.Errorf("DurationError")
	}
	s := awakeSession{Mode: a.config.AwakeMode, Minutes: minutes}
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
func (a *application) changeAwakeEnabled() {
	if !a.config.AwakeEnabled {
		a.awake.stop()
		a.unregisterShortcut(a.config.AwakeShortcut)
	} else {
		a.registerAwakeShortcut()
	}
	a.save()
	a.refreshAwake()
}
func (a *application) registerAwakeShortcut() {
	if !a.config.AwakeEnabled {
		return
	}
	a.shortcutError = ""
	if reason := a.shortcutConflict(a.config.AwakeShortcut, "awake"); reason != "" {
		a.shortcutError = reason
		return
	}
	if err := a.registerShortcut(a.config.AwakeShortcut, a.toggleAwake); err != nil {
		a.shortcutError = "ShortcutUnavailable"
	}
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
	if !s.Active {
		if s.Error != "" {
			return text("R05Off") + " · ⚠ " + text(s.Error)
		}
		return text("R05Off")
	}
	return a.awakeMode(locale, s.Session) + " · " + a.awakeSuffix(locale, s.Session, now)
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
		a.reportAwakeError(a.startAwake(*session))
	}
}

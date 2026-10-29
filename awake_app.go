package main

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

func (a *application) awakeChanged() {
	s := a.awake.snapshot()
	a.config.AwakeSession = nil
	if s.Active {
		copy := s.Session
		a.config.AwakeSession = &copy
	}
	a.save()
	a.refreshAwake()
}
func (a *application) refreshAwake() {
	if a.tray == nil {
		return
	}
	name := "assets/menubar-template.tiff"
	if a.awake.snapshot().Active {
		name = "assets/menubar-template-active.tiff"
	}
	data, _ := assets.ReadFile(name)
	_ = a.tray.SetIcon(data, true)
	a.tray.SetMenu(a.menu())
	a.tray.SetToolTip(a.awakeStatus(mygo.App.Locale(), time.Now()))
	a.invalidateWindow()
}
func (a *application) startAwake(s awakeSession) {
	if !a.config.AwakeEnabled {
		return
	}
	if err := a.awake.start(s); err != nil {
		a.settingsError = "AwakeStartError"
		a.invalidateWindow()
	}
}
func (a *application) selectedSession() awakeSession {
	s := awakeSession{Mode: a.config.AwakeMode}
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
		a.startAwake(a.selectedSession())
	}
}
func (a *application) setMode(mode string) {
	a.config.AwakeMode = mode
	s := a.awake.snapshot()
	if s.Active {
		s.Session.Mode = mode
		a.startAwake(s.Session)
	}
	a.save()
	a.refreshAwake()
}
func (a *application) setDuration(minutes int) {
	a.config.AwakeMinutes = minutes
	a.config.AwakeUntil = ""
	a.save()
	a.startAwake(a.selectedSession())
	a.refreshAwake()
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
	if err := a.registerShortcut(a.config.AwakeShortcut, a.toggleAwake); err != nil {
		a.shortcutError = "ShortcutUnavailable"
	}
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
	mode := "R05Screen"
	if s.Session.Mode == "tasks" {
		mode = "R05Tasks"
	}
	suffix := text("R05Unlimited")
	if !s.Session.End.IsZero() {
		minutes := max(0, int(math.Ceil(s.Session.End.Sub(now).Minutes())))
		suffix = fmt.Sprintf(text("AwakeRemaining"), minutes/60, minutes%60)
	}
	if s.Session.PID > 0 {
		suffix = fmt.Sprintf(text("AwakeFollowing"), s.Session.Name)
	}
	return text(mode) + " · " + suffix
}
func (a *application) awakeMenu(locale string) []*mygo.MenuItem {
	text := func(key string) string { return a.text(locale, key) }
	active := a.awake.snapshot().Active
	items := []*mygo.MenuItem{{ID: "awake-status", Label: a.awakeStatus(locale, time.Now()), Disabled: true}}
	if !active {
		items = append(items, &mygo.MenuItem{ID: "awake-start", Label: text("R05Start"), Disabled: !a.config.AwakeEnabled, Accelerator: a.config.AwakeShortcut, Click: func(*mygo.MenuItem, *mygo.Window) { a.toggleAwake() }})
	}
	modes := []*mygo.MenuItem{}
	for _, m := range []struct{ value, key string }{{"screen", "R05Screen"}, {"tasks", "R05Tasks"}} {
		modes = append(modes, &mygo.MenuItem{Label: text(m.key), Type: mygo.MenuItemRadio, Checked: a.config.AwakeMode == m.value, Click: func(*mygo.MenuItem, *mygo.Window) { a.setMode(m.value) }})
	}
	durations := []*mygo.MenuItem{}
	for _, d := range []struct {
		minutes int
		key     string
	}{{30, "R05Half"}, {60, "R05Hour"}, {120, "R05Two"}, {240, "R05Four"}} {
		durations = append(durations, &mygo.MenuItem{Label: text(d.key), Type: mygo.MenuItemRadio, Checked: a.config.AwakeMinutes == d.minutes && a.config.AwakeUntil == "", Click: func(*mygo.MenuItem, *mygo.Window) { a.setDuration(d.minutes) }})
	}
	durations = append(durations, mygo.Separator(), &mygo.MenuItem{Label: text("R05Custom"), Click: func(*mygo.MenuItem, *mygo.Window) { a.openAwakeDialog("custom") }}, &mygo.MenuItem{Label: text("R05Until"), Click: func(*mygo.MenuItem, *mygo.Window) { a.openAwakeDialog("until") }}, &mygo.MenuItem{Label: text("R05Unlimited"), Type: mygo.MenuItemRadio, Checked: a.config.AwakeMinutes == 0 && a.config.AwakeUntil == "", Click: func(*mygo.MenuItem, *mygo.Window) { a.setDuration(0) }})
	items = append(items, mygo.Separator(), &mygo.MenuItem{Label: text("R05Mode"), Disabled: !a.config.AwakeEnabled, Submenu: modes}, &mygo.MenuItem{Label: text("R05Duration"), Disabled: !a.config.AwakeEnabled, Submenu: durations}, &mygo.MenuItem{Label: text("R05Follow"), Disabled: !a.config.AwakeEnabled, Click: func(*mygo.MenuItem, *mygo.Window) { a.openAwakeDialog("follow") }})
	if active {
		items = append(items, &mygo.MenuItem{ID: "awake-stop", Label: text("R05Stop"), Accelerator: a.config.AwakeShortcut, Click: func(*mygo.MenuItem, *mygo.Window) { a.awake.stop() }})
	}
	return append(items, mygo.Separator())
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

type runningProcess struct {
	PID            int
	Name, Identity string
}

func listProcesses(ctx context.Context) ([]runningProcess, error) {
	data, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,lstart=,comm=").Output()
	if err != nil {
		return nil, err
	}
	var out []runningProcess
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		name := strings.Join(fields[6:], " ")
		parts := strings.Split(name, "/")
		name = parts[len(parts)-1]
		out = append(out, runningProcess{pid, name, strings.Join(fields[1:6], " ")})
	}
	return out, nil
}

func (a *application) restoreAwakeAfterLaunch() {
	restore := a.config.RestoreAwake && a.config.AwakeEnabled
	session := restoreSession(a.config.AwakeSession, time.Now(), a.awake.alive)
	a.config.RestoreAwake = false
	a.config.AwakeSession = nil
	a.save()
	if restore && session != nil {
		a.startAwake(*session)
	}
}

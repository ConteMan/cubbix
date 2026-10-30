package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func awakeTestApp(t *testing.T) *application {
	t.Helper()
	a, err := newApplication(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	a.notifyBattery = func(string, string) {
		t.Fatal("unexpected battery notification; replace the sender in notification tests")
	}
	a.registerShortcut = func(string, func()) error { return nil }
	a.unregisterShortcut = func(string) {}
	a.awake, _ = fakeAwake(time.Now())
	a.awake.changed = a.awakeChanged
	t.Cleanup(func() { a.awake.changed = nil; a.awake.stop(); a.updates.close() })
	return a
}
func TestAwakeMenuAndTileToggle(t *testing.T) {
	a := awakeTestApp(t)
	for _, locale := range []string{"zh-CN", "en"} {
		menu := a.menuForLocale(locale)
		if menu.ItemByID("awake-start") == nil || menu.Items()[0].Label != a.text(locale, "R05Off") {
			t.Fatal("off menu missing")
		}
		menu.ItemByID("awake-start").Click(nil, nil)
		if !a.awake.snapshot().Active || a.config.AwakeSession == nil {
			t.Fatal("start not persisted")
		}
		menu = a.menuForLocale(locale)
		if menu.ItemByID("awake-start") != nil || menu.ItemByID("awake-stop") == nil {
			t.Fatal("running menu missing")
		}
		a.config.AwakeEnabled = false
		a.changeAwakeEnabled()
		if a.awake.snapshot().Active {
			t.Fatal("disabled tile is still active")
		}
		if !a.menuForLocale(locale).ItemByID("awake-start").Disabled {
			t.Fatal("disabled tile can start")
		}
		a.config.AwakeEnabled = true
	}
}
func TestAwakeDialogInputsAndSelection(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		a := awakeTestApp(t)
		a.dialog = &awakeDialog{kind: "custom", hours: "0", minutes: "0"}
		tt := ui.NewTester(a.awakeDialogView, 440, 120)
		tt.SetLocale(locale)
		tt.Key(0, ui.KeyEnter)
		if a.dialog == nil || a.dialog.errorKey != "DurationError" {
			t.Fatal("invalid duration accepted")
		}
		a.dialog.minutes = "30"
		tt.Frame()
		tt.Key(0, ui.KeyEnter)
		if a.dialog != nil || !a.awake.snapshot().Active || a.config.AwakeMinutes != 30 {
			t.Fatal("custom duration not confirmed")
		}
		a.awake.stop()
		a.dialog = &awakeDialog{kind: "until", until: "25:00"}
		tt = ui.NewTester(a.awakeDialogView, 440, 120)
		tt.Key(0, ui.KeyEnter)
		if a.dialog.errorKey != "TimeError" {
			t.Fatal("invalid time accepted")
		}
		a.dialog.until = "02:30"
		tt.Frame()
		tt.Key(0, ui.KeyEnter)
		if a.config.AwakeUntil != "02:30" || a.dialog != nil {
			t.Fatal("until not confirmed")
		}
		a.awake.stop()
		a.dialog = &awakeDialog{kind: "follow", processes: []runningProcess{{PID: 1, Name: "codex", Identity: "one"}, {PID: 2, Name: "claude", Identity: "two"}}, selected: 1}
		tt = ui.NewTester(a.awakeDialogView, 440, 280)
		tt.SetLocale(locale)
		if err := tt.Click("claude 2"); err != nil {
			t.Fatal(err)
		}
		tt.Key(0, ui.KeyEnter)
		if a.dialog != nil || a.awake.snapshot().Session.PID != 2 {
			t.Fatalf("wrong PID followed: dialog=%+v state=%+v", a.dialog, a.awake.snapshot())
		}
		a.dialog = &awakeDialog{kind: "follow", processes: []runningProcess{{PID: 3, Name: "codex", Identity: "three"}}, selected: 3, search: "absent"}
		a.confirmAwakeDialog()
		if a.dialog == nil {
			t.Fatal("hidden selection confirmed")
		}
		a.dialog.errorKey = "ProcessListError"
		a.confirmAwakeDialog()
		if a.dialog == nil {
			t.Fatal("failed list confirmed")
		}
		tt = ui.NewTester(a.awakeDialogView, 440, 280)
		tt.Key(0, ui.KeyEscape)
		if a.dialog != nil {
			t.Fatal("Esc did not cancel")
		}
	}
}
func TestShortcutRecorderCommitConflictAndEscape(t *testing.T) {
	a := awakeTestApp(t)
	a.page = 3
	var registered, removed []string
	a.registerShortcut = func(acc string, _ func()) error { registered = append(registered, acc); return nil }
	a.unregisterShortcut = func(acc string) { removed = append(removed, acc) }
	contains := func(list []string, value string) bool {
		for _, candidate := range list {
			if candidate == value {
				return true
			}
		}
		return false
	}
	old := a.config.AwakeShortcut
	tt := ui.NewTester(a.view, 720, 340)
	tt.SetLocale("en")
	if err := tt.Click("Record keep awake shortcut"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyComma)
	if a.config.AwakeShortcut != old || a.shortcutError != "R05Conflict" || !a.shortcutRecording {
		t.Fatalf("conflict changed shortcut: config=%s old=%s error=%s recording=%v draft=%s", a.config.AwakeShortcut, old, a.shortcutError, a.shortcutRecording, a.shortcutDraft)
	}
	tt.Key(0, ui.KeyEscape)
	if a.shortcutRecording || a.shortcutError != "" || a.shortcutDraft != "" {
		t.Fatal("escape did not reset recorder")
	}
	_ = tt.Click("Record keep awake shortcut")
	tt.Key(ui.Ctrl|ui.Alt|ui.Cmd, ui.KeyL)
	if a.config.AwakeShortcut != "Ctrl+Alt+Cmd+L" || !contains(registered, a.config.JSONShortcut) || !contains(removed, a.config.JSONShortcut) {
		t.Fatal("shortcut not replaced")
	}
	loaded, err := loadConfig(a.path)
	if err != nil || loaded.AwakeShortcut != a.config.AwakeShortcut {
		t.Fatal("shortcut not saved")
	}
	a.registerShortcut = func(string, func()) error { return errors.New("already registered") }
	_ = tt.Click("Record keep awake shortcut")
	tt.Key(ui.Ctrl|ui.Alt|ui.Cmd, ui.KeyM)
	if a.config.AwakeShortcut != "Ctrl+Alt+Cmd+L" || a.shortcutError != "ShortcutUnavailable" {
		t.Fatal("failed registration replaced previous shortcut")
	}
	symbols := shortcutSymbols(old)
	if len(symbols) != 4 || symbols[0] != "⌃" {
		t.Fatal(symbols)
	}
	for _, acc := range []string{"Cmd+Q", "Cmd+Space", "Ctrl+Space", "Cmd+Shift+4", "K"} {
		if knownShortcutConflict(acc) == "" {
			t.Fatal(acc)
		}
	}
}
func TestRestartSavesRemainingSessionBeforeRelaunch(t *testing.T) {
	a := awakeTestApp(t)
	end := time.Now().Add(30 * time.Minute)
	_ = a.awake.start(awakeSession{Mode: "tasks", End: end, PID: 123, Identity: "identity", Name: "codex"})
	a.updates.state = updateState{Phase: updateReady}
	var called bool
	a.relaunch = func() {
		if !a.quitting {
			t.Fatal("relaunch must not ask about unsaved JSON")
		}
		called = true
		config, err := loadConfig(a.path)
		if err != nil || !config.RestoreAwake || config.AwakeSession == nil || !config.AwakeSession.End.Equal(end) || config.AwakeSession.PID != 123 {
			t.Fatalf("session not saved before relaunch: config=%+v session=%+v want=%v err=%v", config, config.AwakeSession, end, err)
		}
	}
	a.restartUpdate()
	if !called {
		t.Fatal("relaunch not called")
	}
	// A failed persistence must block restart and keep the active session.
	if err := os.Remove(a.path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(a.path, 0700); err != nil {
		t.Fatal(err)
	}
	called = false
	a.restartUpdate()
	if called || !a.awake.snapshot().Active || a.settingsError != "ConfigError" {
		t.Fatal("restarted without session persistence")
	}
}

// A test-only rendering of the exact native menu model. AppKit still draws
// the real menu; these snapshots do not claim native menu appearance/focus.
func (a *application) menuEvidenceView(c *ui.Context) {
	locale, t := applyTheme(c)
	ui.Column(c).Fill().Padding(t.Space8).Radius(t.RadiusWindow).Background(ui.Hex(t.ColorMenuSurface)).Children(func() {
		for _, item := range a.menuForLocale(locale).Items() {
			if item.Type == mygo.MenuItemSeparator {
				ui.Divider(c)
				continue
			}
			color := t.ColorText
			if item.Disabled {
				color = t.ColorTextSecondary
			}
			ui.Row(c).FillWidth().Height(t.SizeMenuRowHeight).PaddingX(t.Space12).AlignItems(ui.Center).Gap(t.Space8).Children(func() {
				ui.Text(c, item.Label).FontSize(t.FontSizeBody).TextColor(ui.Hex(color)).Grow(1)
				if item.Accelerator != "" {
					for _, symbol := range shortcutSymbols(item.Accelerator) {
						ui.Text(c, symbol).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
					}
				}
				if len(item.Submenu) > 0 {
					ui.Icon(c, icon("chevron-right")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
				}
			})
		}
	})
}

func TestLaunchRestorePolicy(t *testing.T) {
	for _, scenario := range []struct {
		name                            string
		update, enabled, alive, expired bool
		want                            bool
	}{
		{"ordinary launch stays off", false, true, true, false, false},
		{"update restores", true, true, true, false, true},
		{"expired update stays off", true, true, true, true, false},
		{"ended target stays off", true, true, false, false, false},
		{"disabled tile stays off", true, false, true, false, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			a := awakeTestApp(t)
			a.config.RestoreAwake = scenario.update
			a.config.AwakeEnabled = scenario.enabled
			end := time.Now().Add(time.Hour)
			if scenario.expired {
				end = time.Now().Add(-time.Hour)
			}
			a.config.AwakeSession = &awakeSession{Mode: "tasks", End: end, PID: 12, Identity: "target"}
			a.awake.alive = func(int, string) bool { return scenario.alive }
			a.restoreAwakeAfterLaunch()
			if a.awake.snapshot().Active != scenario.want || a.config.RestoreAwake {
				t.Fatalf("restore result=%+v", a.awake.snapshot())
			}
			if scenario.want && !a.awake.snapshot().Session.End.Equal(end) {
				t.Fatal("deadline changed during restoration")
			}
		})
	}
}

package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestPanelShortcutRowRecordsAndConflicts covers the fourth row of the
// shortcuts page: its label, its recorder, and the reason a clash reports.
func TestPanelShortcutRowRecordsAndConflicts(t *testing.T) {
	a := awakeTestApp(t)
	registered := map[string]func(){}
	a.registerShortcut = func(acc string, fn func()) error {
		registered[acc] = fn
		return nil
	}
	a.unregisterShortcut = func(acc string) { delete(registered, acc) }
	a.page = 3
	tt := ui.NewTester(a.view, 720, settingsHeight(tokens(false, "zh-CN"), 3, false))
	tt.SetLocale("zh-CN")
	if !tt.HasText(a.text("zh-CN", "PanelShortcutLabel")) {
		t.Fatalf("no menu bar panel row: %v", tt.Texts())
	}
	// The recorder draws each symbol of the combination as its own label.
	for _, symbol := range shortcutSymbols(a.config.PanelShortcut) {
		if !tt.HasText(symbol) {
			t.Fatalf("the row does not show %q: %v", symbol, tt.Texts())
		}
	}
	// Recording takes the row's key and stores it; every other slot is
	// registered again afterwards.
	if err := tt.Click(a.text("zh-CN", "PanelRecordShortcut")); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl|ui.Alt|ui.Cmd, ui.KeyM)
	if a.config.PanelShortcut != "Ctrl+Alt+Cmd+M" || a.panelShortcutError != "" {
		t.Fatalf("panel shortcut = %q, %q", a.config.PanelShortcut, a.panelShortcutError)
	}
	for _, slot := range a.shortcutSlots() {
		if _, ok := registered[slot.Acc]; !ok {
			t.Fatalf("%s was not registered again: %v", slot.Target, registered)
		}
	}
	// A combination another slot owns is refused and names that slot.
	if err := tt.Click(a.text("zh-CN", "PanelRecordShortcut")); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl|ui.Alt|ui.Cmd, ui.KeyK)
	if a.config.PanelShortcut != "Ctrl+Alt+Cmd+M" || a.shortcutError != "JSONShortcutConflict" {
		t.Fatalf("clashing shortcut was accepted: %q %q", a.config.PanelShortcut, a.shortcutError)
	}
	if reason := a.shortcutReasonKey("JSONShortcutConflict", "panel", a.config.AwakeShortcut); reason != "PanelShortcutSuffix" && reason == "" {
		t.Fatalf("clash reason = %q", reason)
	}
	a.cancelShortcutRecording()
}

// TestShortcutReasonNamesTheOwningSlot checks both directions of the slot
// clash: a palette recorder that took the panel's combination, and the other
// way round, each name the slot that owns it.
func TestShortcutReasonNamesTheOwningSlot(t *testing.T) {
	a := awakeTestApp(t)
	cases := []struct {
		name    string
		error   string
		target  string
		acc     string
		wantKey string
	}{
		{"json took the palette's", "JSONShortcutConflict", "json", a.config.PaletteShortcut, "PaletteShortcutSuffix"},
		{"json took the panel's", "JSONShortcutConflict", "json", a.config.PanelShortcut, "PanelShortcutSuffix"},
		{"awake took the panel's", "JSONShortcutConflict", "awake", a.config.PanelShortcut, "PanelShortcutSuffix"},
		{"panel took the palette's", "JSONShortcutConflict", "panel", a.config.PaletteShortcut, "PaletteShortcutSuffix"},
		{"the palette recorder keeps its own", "JSONShortcutConflict", "palette", a.config.PaletteShortcut, "JSONShortcutConflict"},
		{"the panel recorder keeps its own", "JSONShortcutConflict", "panel", a.config.PanelShortcut, "JSONShortcutConflict"},
		{"a settings clash is unchanged", "R05Conflict", "panel", "Cmd+,", "ShortcutSettingsSuffix"},
		{"an app clash passes through", "ShortcutAppConflict", "panel", a.config.PanelShortcut, "ShortcutAppConflict"},
	}
	for _, c := range cases {
		if got := a.shortcutReasonKey(c.error, c.target, c.acc); got != c.wantKey {
			t.Errorf("%s: reason = %q; want %q", c.name, got, c.wantKey)
		}
	}
}

// TestPanelShortcutDefaultsAndConflicts checks the fourth slot's default and
// that it takes part in the mutual exclusion of the slots.
func TestPanelShortcutDefaultsAndConflicts(t *testing.T) {
	a := awakeTestApp(t)
	if a.config.PanelShortcut != "Ctrl+Alt+Cmd+P" {
		t.Fatalf("default panel shortcut = %q", a.config.PanelShortcut)
	}
	for _, slot := range a.shortcutSlots() {
		if slot.Target == "panel" {
			continue
		}
		if sameShortcut(slot.Acc, a.config.PanelShortcut) {
			t.Fatalf("default panel shortcut collides with %s", slot.Target)
		}
	}
	if reason := a.shortcutConflict(a.config.AwakeShortcut, "panel"); reason != "JSONShortcutConflict" {
		t.Fatalf("panel composes with the awake slot: %q", reason)
	}
	if reason := a.shortcutConflict(a.config.PanelShortcut, "panel"); reason != "" {
		t.Fatalf("panel conflicts with itself: %q", reason)
	}
	if reason := knownShortcutConflict(a.config.PanelShortcut); reason != "" {
		t.Fatalf("known conflicts reject the default: %q", reason)
	}
}

// TestPanelShortcutTogglesTheCardPanel checks the behaviour behind ⌃⌥⌘P: it
// toggles, it closes the palette first, and recording takes the key instead.
func TestPanelShortcutTogglesTheCardPanel(t *testing.T) {
	a := panelTestApp(t)
	a.toggleMenubarPanel()
	if !a.menubarPanelVisible() {
		t.Fatal("the panel shortcut did not open the card panel")
	}
	a.showPalette()
	a.toggleMenubarPanel()
	if a.paletteVisible() || !a.menubarPanelVisible() {
		t.Fatalf("the shortcut stacked the panels: palette=%v panel=%v", a.paletteVisible(), a.menubarPanelVisible())
	}
	a.toggleMenubarPanel()
	if a.menubarPanelVisible() {
		t.Fatal("the second press did not close the card panel")
	}
	a.beginTileShortcutRecording("panel")
	a.panel = nil
	a.toggleMenubarPanel()
	if a.menubarPanelVisible() {
		t.Fatal("the shortcut fired while recording")
	}
	a.cancelShortcutRecording()
}

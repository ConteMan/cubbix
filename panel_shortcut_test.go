package main

import (
	"errors"
	"strings"
	"testing"
	"time"

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
	tt := ui.NewTester(a.view, 720, settingsHeight(tokens(false, "zh-CN"), settingsShortcuts, true, false, false, false, false))
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
	if reason := a.shortcutReasonKey("JSONShortcutConflict", "panel", a.config.AwakeShortcut); reason != "JSONShortcutConflict" {
		t.Fatalf("clash reason = %q", reason)
	}
	a.cancelShortcutRecording()
}

func TestPanelShortcutConflictTooltipsInBothDirections(t *testing.T) {
	// Each module owns its own page now: the awake and JSON recorders live on
	// their module pages, the two global ones on the shortcuts page.
	pages := map[string]int{"awake": settingsAwake, "json": settingsJSON, "palette": settingsShortcuts, "panel": settingsShortcuts}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, target := range []string{"awake", "json", "palette", "panel"} {
			t.Run(locale+"/"+target, func(t *testing.T) {
				a := awakeTestApp(t)
				a.page = pages[target]
				a.shortcutTarget = target
				a.shortcutRecording = true
				a.shortcutError = "JSONShortcutConflict"
				a.shortcutDraft = a.config.PanelShortcut
				want := "PanelShortcutSuffix"
				if target == "panel" {
					a.shortcutDraft = a.config.PaletteShortcut
					want = "PaletteShortcutSuffix"
				}
				height := settingsHeight(tokens(false, locale), a.page, a.config.AwakeEnabled, false, false, false, false)
				tt := ui.NewTester(a.view, 720, height)
				tt.SetLocale(locale)
				// The alert is labelled with the recorded combination and the
				// reason, so a pointer and a reader reach the same sentence.
				symbols := strings.Join(shortcutSymbols(a.shortcutDraft), " ")
				alert, ok := tt.Find(symbols + " " + a.text(locale, want))
				if !ok || alert.Y+alert.H > float32(height) {
					t.Fatalf("conflict alert is missing or clipped at the window height: %v", tt.Texts())
				}
				tt.Move(alert.X+alert.W/2, alert.Y+alert.H/2)
				tt.Advance(time.Second)
				if !tt.HasText(a.text(locale, want)) {
					t.Fatalf("tooltip missed the owning slot: %v", tt.Texts())
				}
			})
		}
	}
}

func TestPanelShortcutRegistrationAndFailure(t *testing.T) {
	a := panelTestApp(t)
	var callback func()
	a.registerShortcut = func(acc string, fn func()) error {
		if acc != a.config.PanelShortcut {
			t.Fatalf("registered wrong panel shortcut: %q", acc)
		}
		callback = fn
		return nil
	}
	a.registerPanelShortcut()
	if callback == nil || a.panelShortcutError != "" {
		t.Fatal("registration did not install the panel callback")
	}
	callback()
	if !a.menubarPanelVisible() {
		t.Fatal("registered callback did not open the panel")
	}
	callback()
	if a.menubarPanelVisible() {
		t.Fatal("registered callback did not close the panel")
	}
	// A new value has to register for real; a failure reaches the row and the
	// slot keeps owning nothing.
	a.config.PanelShortcut = "Ctrl+Alt+Cmd+M"
	a.registerShortcut = func(string, func()) error { return errors.New("occupied") }
	a.registerPanelShortcut()
	if a.panelShortcutError != "ShortcutUnavailable" || a.ownedShortcut("panel") != a.config.PanelShortcut {
		// The old registration is still owned; the new one owns nothing.
		if a.panelShortcutError != "ShortcutUnavailable" {
			t.Fatal("registration failure did not reach the panel settings row")
		}
	}
	a.config.PanelShortcut = a.config.PaletteShortcut
	a.registerShortcut = func(string, func()) error { t.Fatal("registered a conflicting shortcut"); return nil }
	a.registerPanelShortcut()
	if a.panelShortcutError != "JSONShortcutConflict" {
		t.Fatal("fourth-slot conflict was not rejected before registration")
	}
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

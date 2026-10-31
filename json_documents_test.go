package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestJSONDocumentNamesListsAndIsolation(t *testing.T) {
	a := awakeTestApp(t)
	one := a.addJSONDocument(`{"x":1}`, "", false)
	two := a.addJSONDocument(`[]`, "/selected/items.json", true)
	if !one.dirty || two.dirty || one.number == two.number {
		t.Fatal("documents share initial state")
	}
	for _, locale := range []string{"zh-CN", "en"} {
		if one.name(a, locale) != "JSON" || two.name(a, locale) != "items.json" {
			t.Fatal("document names")
		}
		rows := a.menubarPanelRows()
		if len(rows.documents) != 2 || rows.documents[1].name(a, locale) != two.name(a, locale) {
			t.Fatal("panel window entries missing")
		}
	}
	// The second and later unopened windows show the fixed name; no numbering.
	third := a.addJSONDocument("", "", true)
	fourth := a.addJSONDocument("", "", true)
	if third.name(a, "en") != "JSON" || fourth.name(a, "en") != "JSON" {
		t.Fatal("unopened windows must keep the fixed name")
	}
	// A rename is the name in the window, the menu and the card list; an empty
	// name or the default name goes back to the default.
	one.rename(a, "en", "  orders  ")
	if one.name(a, "en") != "orders" || one.name(a, "zh-CN") != "orders" {
		t.Fatal("rename did not apply to every locale")
	}
	if rows := a.menubarPanelRows(); rows.documents[0].name(a, "en") != "orders" {
		t.Fatal("card list kept the old name")
	}
	one.rename(a, "en", "")
	if one.name(a, "en") != "JSON" {
		t.Fatal("empty rename did not restore the default")
	}
	one.rename(a, "en", "JSON")
	if one.custom != "" {
		t.Fatal("the default name was stored as a rename")
	}
	one.rename(a, "en", strings.Repeat("x", jsonWindowNameLimit+10))
	if len([]rune(one.name(a, "en"))) != jsonWindowNameLimit {
		t.Fatal("rename is not bounded")
	}
	one.rename(a, "en", "")
	a.removeJSONDocument(third)
	a.removeJSONDocument(fourth)
	one.pinned = true
	one.changed(3, true)
	one.changed(2, false)
	if !one.dirty || two.pinned || one.revision != 3 {
		t.Fatal("stale update or shared state")
	}
	for range 4 {
		a.addJSONDocument("", "", true)
	}
	// Six open windows fold into one count row until the list is expanded.
	a.panel = &menubarPanelState{}
	if rows := a.menubarPanelRows(); !rows.folded || rows.listRows != 1 {
		t.Fatalf("large list must fold: %+v", rows)
	}
	a.panel.expanded = true
	if rows := a.menubarPanelRows(); rows.folded || rows.listRows != 7 {
		t.Fatalf("expanded list rows = %+v", rows)
	}
	a.removeJSONDocument(one)
	three := a.addJSONDocument("", "", true)
	if three.number != 9 || !one.closed {
		t.Fatal("closed document reused")
	}
	for _, d := range a.menubarPanelRows().documents {
		if d == one {
			t.Fatal("closed document listed in the panel")
		}
	}
}
func TestJSONFilePreservesRawInvalidInputAndAtomicSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selected.json")
	raw := "{invalid: untouched}\n"
	if err := os.WriteFile(path, []byte(raw), 0640); err != nil {
		t.Fatal(err)
	}
	got, err := readJSONFile(path)
	if err != nil || got != raw {
		t.Fatal("invalid JSON was discarded", err)
	}
	if err := writeJSONFile(path, "{\"saved\":true}\n"); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	data, _ := os.ReadFile(path)
	if string(data) != "{\"saved\":true}\n" || info.Mode().Perm() != 0640 {
		t.Fatal("save changed contents or permissions")
	}
	if _, err := readJSONFile(filepath.Dir(path)); err == nil {
		t.Fatal("directory accepted")
	}
	file, err := os.Create(filepath.Join(t.TempDir(), "huge.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(jsonFileLimit + 1); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := readJSONFile(file.Name()); err == nil || err.Error() != "JSONFileLimit" {
		t.Fatal("size limit not enforced", err)
	}
	for _, s := range []string{"", "not JSON", "{broken"} {
		if validClipboardJSON(s) != "" {
			t.Fatal("invalid clipboard accepted")
		}
	}
	if validClipboardJSON("[1,2]") != "[1,2]" {
		t.Fatal("valid clipboard rejected")
	}
}
func TestJSONToggleAndShortcutRecorder(t *testing.T) {
	a := awakeTestApp(t)
	registry := map[string]bool{}
	a.registerShortcut = func(acc string, _ func()) error { registry[acc] = true; return nil }
	a.unregisterShortcut = func(acc string) { delete(registry, acc) }
	a.registerAwakeShortcut()
	a.registerJSONShortcut()
	a.config.JSONEnabled = false
	a.changeJSONEnabled()
	if registry[a.config.JSONShortcut] || len(a.applicationMenu("en").Items()) != 2 {
		t.Fatal("disabled tile kept entry or shortcut")
	}
	if !registry[a.config.AwakeShortcut] {
		t.Fatal("JSON disabled awake shortcut")
	}
	a.config.JSONEnabled = true
	a.changeJSONEnabled()
	a.page = 3
	tt := ui.NewTester(a.view, 720, 340)
	tt.SetLocale("en")
	if err := tt.Click("Record JSON shortcut"); err != nil {
		t.Fatal(err)
	}
	if len(registry) != 0 {
		t.Fatal("recording must suspend all shortcuts")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
	if a.shortcutError != "ShortcutAppConflict" || a.config.JSONShortcut != "Ctrl+Alt+Cmd+J" {
		t.Fatal("modifier ordering bypassed application conflict")
	}
	tt.Key(ui.Cmd|ui.Alt, ui.KeySpace)
	if a.shortcutError != "ShortcutSystemConflict" {
		t.Fatal("modifier ordering bypassed system conflict")
	}
	tt.Key(ui.Ctrl|ui.Alt|ui.Cmd, ui.KeyK)
	if a.shortcutError != "JSONShortcutConflict" || a.config.JSONShortcut != "Ctrl+Alt+Cmd+J" {
		t.Fatal("cross-tile conflict replaced shortcut")
	}
	tt.Key(0, ui.KeyEscape)
	if len(registry) != 3 {
		t.Fatal("escape did not restore shortcuts")
	}
	_ = tt.Click("Record JSON shortcut")
	tt.Key(ui.Ctrl|ui.Alt|ui.Cmd, ui.KeyM)
	if a.shortcutRecording || a.config.JSONShortcut != "Ctrl+Alt+Cmd+M" || len(registry) != 3 {
		t.Fatal("JSON shortcut did not commit")
	}
	if strings.Join(shortcutSymbols(a.config.JSONShortcut), "") != "⌃⌥⌘M" {
		t.Fatal("shortcut display")
	}
	for _, acc := range []string{"Cmd+O", "Cmd+S", "Cmd+Shift+S", "Cmd+Shift+C", "Shift+Cmd+C", "Shift+Cmd+S", "Alt+Cmd+P"} {
		if knownShortcutConflict(acc) == "" {
			t.Fatal("window shortcut accepted globally", acc)
		}
	}
}

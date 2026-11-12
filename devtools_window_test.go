package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/ConteMan/cubbix/internal/cli"
	"github.com/ConteMan/cubbix/internal/devtools"
)

// devtoolsWindowTestApp is a headless app whose devtools seams never touch a
// window, the clipboard, a key event or a worker.
func devtoolsWindowTestApp(t *testing.T) (*application, *[]*fakeDevtoolsWindow) {
	t.Helper()
	a := panelTestApp(t)
	windows := new([]*fakeDevtoolsWindow)
	a.createDevtoolsWindow = func(state *devtoolsState) {
		window := &fakeDevtoolsWindow{}
		state.testFrame = window
		*windows = append(*windows, window)
	}
	a.devtoolsOnMainThread = func(fn func()) { fn() }
	a.devtoolsAsync = false
	a.devtoolsReadClipboard = func() string {
		t.Fatal("the devtools window read the clipboard unasked")
		return ""
	}
	a.devtoolsPasteAccess = func() bool { return false }
	a.devtoolsFrontmost = func() int32 { return 0 }
	a.devtoolsPostKey = func(int32) error {
		t.Fatal("an unauthorized paste posted a key")
		return nil
	}
	return a, windows
}

type fakeDevtoolsWindow struct {
	fakeFrame
	destroyed bool
	pins      []bool
}

func (f *fakeDevtoolsWindow) SetContentSize(width, height int) {
	f.bounds.Width, f.bounds.Height = width, height
}

func (f *fakeDevtoolsWindow) SetAlwaysOnTop(pinned bool) { f.pins = append(f.pins, pinned) }
func (f *fakeDevtoolsWindow) Destroy()                   { f.destroyed, f.visible = true, false }

func TestDevtoolsToolWindowOptions(t *testing.T) {
	a, _ := devtoolsWindowTestApp(t)
	for _, dark := range []bool{false, true} {
		for _, locale := range []string{"zh-CN", "en"} {
			tk := tokens(dark, locale)
			got := a.devtoolsWindowOptions(tk, locale)
			qr := a.qrWindowOptions(tk, locale)
			if got.Panel || got.Frameless || got.Transparent || got.DisableResize || got.DisableMove || got.DisableClose || got.AlwaysOnTop {
				t.Fatalf("unexpected tool window flags: %+v", got)
			}
			if got.TitleBarStyle != qr.TitleBarStyle || *got.TrafficLightPosition != *qr.TrafficLightPosition ||
				got.DisableFullScreen != qr.DisableFullScreen || got.BackgroundColor != qr.BackgroundColor ||
				got.DisableMinimize != qr.DisableMinimize || got.DisableMaximize != qr.DisableMaximize ||
				got.Hidden != qr.Hidden || got.Page.DevTools != qr.Page.DevTools {
				t.Fatal("the devtools window chrome differs from the QR window")
			}
			if got.Content == nil || got.Width != 480 || got.Height != 440 || got.MinWidth != 480 || got.MinHeight != 360 {
				t.Fatalf("unexpected content or geometry: %+v", got)
			}
		}
	}
}

func TestDevtoolsWindowGeometryRoundTrip(t *testing.T) {
	a, windows := devtoolsWindowTestApp(t)
	screens := paletteTestScreens(a)
	screens.cursor = mygo.Point{X: -600, Y: 300}
	a.openDevtools(devtoolsRequest{})
	first := (*windows)[0]
	if first.bounds.X >= 0 || a.devtools.place.display.ID != 2 {
		t.Fatalf("the first window missed the pointer display: %+v", first.bounds)
	}
	first.drag(-1500, 250)
	first.SetContentSize(620, 700)
	a.closeDevtools()
	again := filepath.Join(t.TempDir(), "again.toml")
	if err := saveConfig(again, a.config); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.config.DevtoolsWindows["2"]; !ok {
		t.Fatalf("no seat stored: %+v", a.config.DevtoolsWindows)
	}
	a.openDevtools(devtoolsRequest{})
	second := (*windows)[1]
	if second.bounds.X != -1500 || second.bounds.Y != 250 {
		t.Fatalf("the second window did not return to its seat: %+v", second.bounds)
	}
	if second.bounds.Width != 620 || second.bounds.Height != 700 {
		t.Fatalf("the second window lost its size: %+v", second.bounds)
	}
	// Another display inherits the default size and never the other seat.
	a.closeDevtools()
	screens.cursor = mygo.Point{X: 700, Y: 300}
	a.openDevtools(devtoolsRequest{})
	third := (*windows)[2]
	if third.bounds.Width != 480 || third.bounds.Height != 440 || third.bounds.X < 0 {
		t.Fatalf("another display inherited geometry: %+v", third.bounds)
	}
}

func TestDevtoolsWindowRepairsWithoutSavingOrFocusing(t *testing.T) {
	for _, minimized := range []bool{false, true} {
		a, windows := devtoolsWindowTestApp(t)
		screens := paletteTestScreens(a)
		a.config.DevtoolsWindows = map[string]devtoolsGeometry{"1": {X: 850, Y: 400, Width: 560, Height: 400}}
		a.openDevtools(devtoolsRequest{})
		window := (*windows)[0]
		window.minimized = minimized
		screens.displays[0].WorkArea = mygo.Rectangle{X: 0, Y: 25, Width: 800, Height: 600}
		before := window.bounds
		a.repairWindowPlaces(screens.displays)
		if minimized {
			if window.bounds != before || !a.devtools.place.needsRepair {
				t.Fatal("a minimized window moved before restoration")
			}
			a.showDevtools()
		} else if window.shows != 1 || window.focuses != 1 {
			t.Fatal("the display repair showed or focused the window")
		}
		if window.bounds.X+window.bounds.Width > 800 || window.bounds.Y+window.bounds.Height > 625 {
			t.Fatalf("the window escaped the changed work area: %+v", window.bounds)
		}
		a.closeDevtools()
		if a.config.DevtoolsWindows["1"].X != 850 || a.config.DevtoolsWindows["1"].Y != 400 {
			t.Fatal("an automatic repair overwrote the remembered seat")
		}
	}
	a, windows := devtoolsWindowTestApp(t)
	screens := paletteTestScreens(a)
	screens.cursor = mygo.Point{X: -600, Y: 300}
	a.openDevtools(devtoolsRequest{})
	screens.displays = screens.displays[:1]
	screens.cursor = mygo.Point{X: 500, Y: 300}
	a.repairWindowPlaces(screens.displays)
	if (*windows)[0].bounds.X < 0 || a.devtools.place.display.ID != 1 {
		t.Fatal("a disconnected display left the window unreachable")
	}
}

func TestDevtoolsQuitSavesSeat(t *testing.T) {
	a, windows := devtoolsWindowTestApp(t)
	paletteTestScreens(a)
	a.openDevtools(devtoolsRequest{})
	(*windows)[0].drag(120, 80)
	a.observeDevtoolsMove()
	a.saveWindowPlacesOnQuit()
	if got := a.config.DevtoolsWindows["1"]; got.X != 120 || got.Y != 55 || got.Width != 480 {
		t.Fatalf("quit did not write the seat: %+v", a.config.DevtoolsWindows)
	}
	if a.devtools.place.pending != nil {
		t.Fatal("quit left a pending seat")
	}
	a.closeDevtools()
	if !(*windows)[0].destroyed {
		t.Fatal("closing did not destroy the window")
	}
}

// TestDevtoolsToolWindowsDoNotCloseEachOther pins the mutual rule: opening the
// devtools window closes the overlays, and the overlays never close a tool
// window.
func TestDevtoolsToolWindowsDoNotCloseEachOther(t *testing.T) {
	a, windows := devtoolsWindowTestApp(t)
	a.createQRWindow = func(*qrState) {}
	a.createPaletteWindow = func(*paletteState) {}
	a.openDevtools(devtoolsRequest{})
	if len(*windows) != 1 || !a.devtoolsVisible() {
		t.Fatal("the devtools window did not open")
	}
	a.openQRText("https://example.com")
	if !a.devtoolsVisible() || !a.qrVisible() {
		t.Fatal("the QR window closed the devtools window")
	}
	a.devtoolsReadClipboard = func() string { return "" }
	a.showPalette()
	if !a.devtoolsVisible() || !a.qrVisible() {
		t.Fatal("the palette closed a tool window")
	}
	a.showMenubarPanel()
	if !a.devtoolsVisible() {
		t.Fatal("the card panel closed the devtools window")
	}
	a.hideMenubarPanel()
	a.openDevtools(devtoolsRequest{})
	if a.paletteVisible() || a.menubarPanelVisible() {
		t.Fatal("opening the devtools window left an overlay on screen")
	}
	if !a.qrVisible() {
		t.Fatal("opening the devtools window closed the QR window")
	}
}

func TestDevtoolsModuleSwitch(t *testing.T) {
	a, windows := devtoolsWindowTestApp(t)
	registered := map[string]bool{}
	a.registerShortcut = func(acc string, _ func()) error { registered[acc] = true; return nil }
	a.unregisterShortcut = func(acc string) { delete(registered, acc) }
	a.registerDevtoolsShortcut()
	if !registered[a.config.DevtoolsShortcut] {
		t.Fatal("the devtools shortcut did not register")
	}
	a.openDevtools(devtoolsRequest{})
	a.changeDevtoolsEnabled(false)
	if a.config.DevtoolsEnabled || registered[a.config.DevtoolsShortcut] || a.ownedShortcut("devtools") != "" {
		t.Fatal("disabling kept the flag or the shortcut")
	}
	if a.devtools != nil || !(*windows)[0].destroyed {
		t.Fatal("disabling kept the window")
	}
	if findCommand(a.commands(), "devtools.open") != nil {
		t.Fatal("the command is still available")
	}
	if a.menubarPanelRows().devtools {
		t.Fatal("the card is still drawn")
	}
	request := cli.Request{V: cli.Protocol, ID: "dev1", Op: cli.OpRun, Action: cli.ActionUUID, Count: 2}
	if result := a.applyCLI(request, nil); result.Code != "disabled" {
		t.Fatalf("CLI answer = %+v", result)
	}
	a.changeDevtoolsEnabled(true)
	if !a.config.DevtoolsEnabled || !registered[a.config.DevtoolsShortcut] {
		t.Fatal("re-enabling did not restore the shortcut")
	}
	a.changeDevtoolsEnabled(true)
	if len(registered) != 1 {
		t.Fatalf("a redundant switch registered twice: %v", registered)
	}
}

func TestDevtoolsShortcutRecording(t *testing.T) {
	a, _ := devtoolsWindowTestApp(t)
	before := []string{a.config.AwakeShortcut, a.config.JSONShortcut, a.config.PaletteShortcut, a.config.PanelShortcut, a.config.TranslateShortcut, a.config.QRShortcut}
	a.beginTileShortcutRecording("devtools")
	if !a.shortcutRecording || a.shortcutTarget != "devtools" {
		t.Fatal("the devtools recorder did not start")
	}
	a.recordShortcut(ui.Ctrl|ui.Alt|ui.Cmd, ui.KeyY)
	if a.config.DevtoolsShortcut != "Ctrl+Alt+Cmd+Y" {
		t.Fatalf("devtools shortcut = %q", a.config.DevtoolsShortcut)
	}
	after := []string{a.config.AwakeShortcut, a.config.JSONShortcut, a.config.PaletteShortcut, a.config.PanelShortcut, a.config.TranslateShortcut, a.config.QRShortcut}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("recording devtools overwrote slot %d: %q -> %q", i, before[i], after[i])
		}
	}
	a.config.DevtoolsEnabled = false
	if a.shortcutSlotEnabled("devtools") {
		t.Fatal("a released slot still records")
	}
}

func TestDevtoolsWindowConfigDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	config, err := loadConfig(path)
	if err != nil || !config.DevtoolsEnabled || config.DevtoolsShortcut != "Ctrl+Alt+Cmd+D" || config.DevtoolsUUIDUpper || config.DevtoolsUUIDPlain {
		t.Fatalf("defaults = %+v, %v", config, err)
	}
	config.DevtoolsEnabled = false
	config.DevtoolsShortcut = "Ctrl+Alt+Cmd+Y"
	config.DevtoolsUUIDUpper, config.DevtoolsUUIDPlain = true, true
	config.DevtoolsWindows = map[string]devtoolsGeometry{"1": {X: 10, Y: 20, Width: 500, Height: 460}}
	if err := saveConfig(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil || loaded.DevtoolsEnabled || loaded.DevtoolsShortcut != "Ctrl+Alt+Cmd+Y" ||
		!loaded.DevtoolsUUIDUpper || !loaded.DevtoolsUUIDPlain || loaded.DevtoolsWindows["1"].Width != 500 {
		t.Fatalf("saved = %+v, %v", loaded, err)
	}
	// An empty shortcut reads back as the default.
	legacy := "version = 1\nlanguage = \"zh-CN\"\n"
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err = loadConfig(path)
	if err != nil || loaded.DevtoolsShortcut != "Ctrl+Alt+Cmd+D" {
		t.Fatalf("legacy shortcut = %q, %v", loaded.DevtoolsShortcut, err)
	}
}

func TestDevtoolsClipboardHitRule(t *testing.T) {
	cases := []struct{ name, raw, want string }{
		{"seconds", "1700000000", "1700000000"},
		{"date", " 2026-11-12 \n", "2026-11-12"},
		{"uuid", "00000000-0000-4000-8000-000000000000", "00000000-0000-4000-8000-000000000000"},
		{"sentence", "see 1700000000", ""},
		{"prefixed", "ts 1700000000", ""},
		{"too long", strings.Repeat("1", devtoolsMaxClipboardBytes+1), ""},
		{"empty", "   ", ""},
	}
	for _, c := range cases {
		if got := devtoolsClipboardHit(c.raw); got != c.want {
			t.Fatalf("%s: hit = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDevtoolsShortcutPrefill(t *testing.T) {
	// A found selection is used as it is when it is a whole hit.
	a, _ := devtoolsWindowTestApp(t)
	a.selection = &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "1700000000"}
	a.toggleDevtools()
	if a.devtools.tab != devtoolsTimeTab || a.devtools.input != "1700000000" {
		t.Fatalf("selection prefill = tab %d input %q", a.devtools.tab, a.devtools.input)
	}
	// A selection that is not a hit leaves the window empty instead of reading
	// the clipboard: the intent was the selection.
	a, _ = devtoolsWindowTestApp(t)
	a.selection = &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "an ordinary sentence"}
	a.toggleDevtools()
	if a.devtools.input != "" {
		t.Fatalf("a non-hit selection prefilled %q", a.devtools.input)
	}
	// No selection: only a clipboard hit prefills.
	for _, c := range []struct{ clip, want string }{
		{"2026-11-12", "2026-11-12"},
		{"just a sentence", ""},
	} {
		a, _ = devtoolsWindowTestApp(t)
		a.selection = &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "  "}
		clip := c.clip
		a.devtoolsReadClipboard = func() string { return clip }
		a.toggleDevtools()
		if a.devtools.input != c.want {
			t.Fatalf("clipboard %q: input = %q, want %q", clip, a.devtools.input, c.want)
		}
	}
}

func TestDevtoolsShortcutUUIDPrefillOpensUUIDTab(t *testing.T) {
	const literal = "00000000-0000-4000-8000-000000000000"
	for _, source := range []string{"selection", "clipboard"} {
		a, _ := devtoolsWindowTestApp(t)
		a.selection = &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "  "}
		if source == "selection" {
			a.selection = &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: literal}
		} else {
			a.devtoolsReadClipboard = func() string { return literal }
		}
		a.toggleDevtools()
		if a.devtools.tab != devtoolsUUIDTab || a.devtools.uuidLiteral != literal {
			t.Fatalf("%s UUID prefill = tab %d literal %q", source, a.devtools.tab, a.devtools.uuidLiteral)
		}
		if a.devtools.selected != 0 {
			t.Fatalf("%s UUID prefill selected row %d", source, a.devtools.selected)
		}
	}
}

func TestDevtoolsShortcutDeniedOnce(t *testing.T) {
	a, _ := devtoolsWindowTestApp(t)
	opens := 0
	a.openAccessibilitySettings = func() { opens++ }
	a.selection = &fakeSelectionReader{pid: otherSelectionPID()}
	a.toggleDevtools()
	if !a.devtools.denied || !a.devtoolsSelectionHintShown || a.devtools.input != "" {
		t.Fatalf("a denied read did not show the hint: %+v", a.devtools)
	}
	if opens != 0 {
		t.Fatal("a denied read opened System Settings by itself")
	}
	a.closeDevtools()
	a.toggleDevtools()
	if a.devtools.denied {
		t.Fatal("the hint repeated in the same launch")
	}
	a.devtools.denied = true
	a.grantAccessibility()
	if a.devtools.denied || opens != 1 {
		t.Fatalf("grant = denied %v opens %d", a.devtools.denied, opens)
	}
	a.devtools.denied = true
	a.grantAccessibility()
	if opens != 1 {
		t.Fatalf("System Settings opened %d times in one launch", opens)
	}
}

// TestDevtoolsShortcutBusyOpensEmpty covers the concurrent-read boundary: a
// busy read opens an empty window and never falls back to the clipboard.
func TestDevtoolsShortcutBusyOpensEmpty(t *testing.T) {
	a, _ := devtoolsWindowTestApp(t)
	a.selection = &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "1700000000"}
	a.selectionReading.Store(true)
	defer a.selectionReading.Store(false)
	a.toggleDevtools()
	if a.devtools.input != "" {
		t.Fatalf("a busy read prefilled %q", a.devtools.input)
	}
}

func TestDevtoolsWindowRowsAndCopy(t *testing.T) {
	a, _ := devtoolsWindowTestApp(t)
	var copied []string
	a.devtoolsCopy = func(text string) error { copied = append(copied, text); return nil }
	a.openDevtools(devtoolsRequest{})
	rows := a.devtoolsWindowRows("en")
	if len(rows) != 4 || rows[0].ErrorKey != "" {
		t.Fatalf("an empty field should show now's four rows: %+v", rows)
	}
	if !a.copyDevtoolsSelected() || len(copied) != 1 || copied[0] != rows[0].Payload {
		t.Fatalf("copy = %v, want %q", copied, rows[0].Payload)
	}
	// The error row copies nothing and Enter does not close the window.
	a.devtools.input = "ts 946684800000"
	a.syncDevtoolsInput()
	rows = a.devtoolsWindowRows("en")
	if len(rows) != 1 || rows[0].ErrorKey != "DevtoolsUnit" {
		t.Fatalf("error rows = %+v", rows)
	}
	if a.copyDevtoolsSelected() || len(copied) != 1 {
		t.Fatal("an error row copied something")
	}
	if !a.devtoolsVisible() {
		t.Fatal("an error closed the window")
	}
	// A failing write keeps the window and marks that row.
	a.devtoolsCopy = func(string) error { return errors.New("clipboard is busy") }
	a.devtools.input = "942684800"
	a.syncDevtoolsInput()
	if a.copyDevtoolsSelected() || a.devtools.copyFailedKey == "" {
		t.Fatal("a failed copy was not marked")
	}
}

func TestDevtoolsWindowUUIDRows(t *testing.T) {
	a, _ := devtoolsWindowTestApp(t)
	var copied []string
	a.devtoolsCopy = func(text string) error { copied = append(copied, text); return nil }
	a.openDevtools(devtoolsRequest{Kind: devtools.KindUUID, Version: 0})
	rows := a.devtoolsWindowRows("en")
	if len(rows) != 2 || rows[0].Label != "v4" || rows[1].Label != "v7" {
		t.Fatalf("the bare pair rows = %+v", rows)
	}
	if got := uuid.MustParse(rows[0].Payload); got[6]>>4 != 4 {
		t.Fatalf("first value is version %d", got[6]>>4)
	}
	if got := uuid.MustParse(rows[1].Payload); got[6]>>4 != 7 {
		t.Fatalf("second value is version %d", got[6]>>4)
	}
	// A batch is one row with the values on their own lines.
	a.openDevtools(devtoolsRequest{Kind: devtools.KindUUID, Version: 7, Count: 21})
	rows = a.devtoolsWindowRows("en")
	if len(rows) != 1 || len(rows[0].Lines) != 21 || len(strings.Split(rows[0].Payload, "\n")) != 21 {
		t.Fatalf("batch rows = %d lines %d", len(rows), len(rows[0].Lines))
	}
	if rows[0].Hint != "21 values" {
		t.Fatalf("batch hint = %q", rows[0].Hint)
	}
	if !a.copyDevtoolsSelected() || copied[len(copied)-1] != rows[0].Payload {
		t.Fatal("the batch was not copied whole")
	}
	// The switches rewrite the same values and persist.
	before := append([]string(nil), a.devtools.values...)
	a.writeDevtoolsSwitches(true, true)
	rows = a.devtoolsWindowRows("en")
	want := strings.ToUpper(strings.ReplaceAll(strings.Join(before, "\n"), "-", ""))
	if rows[0].Payload != want {
		t.Fatalf("switches did not rewrite the batch: %q", rows[0].Payload)
	}
	if !a.config.DevtoolsUUIDUpper || !a.config.DevtoolsUUIDPlain {
		t.Fatal("the switches did not reach the config")
	}
	if strings.Join(a.devtools.values, "|") != strings.Join(before, "|") {
		t.Fatal("a switch generated new values")
	}
	// The window itself exposes version and count controls; the palette is not
	// required to turn the default pair into a v7 batch.
	a.openDevtools(devtoolsRequest{Kind: devtools.KindUUID})
	a.selectDevtoolsUUIDVersion(7)
	if a.devtools.uuidVersion != 7 || len(a.devtools.values) != 1 {
		t.Fatalf("v7 control = version %d values %d", a.devtools.uuidVersion, len(a.devtools.values))
	}
	a.devtools.uuidCountText = "3"
	a.syncDevtoolsUUIDCount()
	if a.devtools.uuidCount != 3 || len(a.devtools.values) != 3 || a.devtools.uuidCountErr {
		t.Fatalf("count control = count %d values %d error %v", a.devtools.uuidCount, len(a.devtools.values), a.devtools.uuidCountErr)
	}
	for _, value := range a.devtools.values {
		if id := uuid.MustParse(value); id[6]>>4 != 7 {
			t.Fatalf("count control changed version to %d", id[6]>>4)
		}
	}
	a.devtools.uuidCountText = "101"
	a.syncDevtoolsUUIDCount()
	if rows := a.devtoolsUUIDRows("en", a.devtools); len(rows) != 1 || rows[0].ErrorKey != "DevtoolsCountInvalid" {
		t.Fatalf("invalid count rows = %+v", rows)
	}

	// Generate again makes a fresh batch of the same version and count.
	a.openDevtools(devtoolsRequest{Kind: devtools.KindUUID, Version: 7, Count: 21})
	a.writeDevtoolsSwitches(false, false)
	first := a.devtools.values[0]
	a.regenerateDevtoolsUUID()
	if len(a.devtools.values) != 21 || a.devtools.values[0] == first {
		t.Fatal("generate again did not make a fresh batch")
	}
}

func TestDevtoolsUUIDGenerationControlsUI(t *testing.T) {
	a, _ := devtoolsWindowTestApp(t)
	a.openDevtools(devtoolsRequest{Kind: devtools.KindUUID})
	tk := tokens(false, "en")
	tester := ui.NewTester(a.devtoolsView, int(tk.SizeDevtoolsWidth), int(tk.SizeDevtoolsWindowHeight))
	tester.SetLocale("en")
	tester.Frame()
	if err := tester.Click("UUID v7"); err != nil {
		t.Fatal(err)
	}
	if a.devtools.uuidVersion != 7 {
		t.Fatalf("clicked version = %d", a.devtools.uuidVersion)
	}
	tester.Frame()
	if err := tester.Click("UUID " + a.text("en", "DevtoolsCountLabel")); err != nil {
		t.Fatal(err)
	}
	tester.Key(ui.Cmd, ui.KeyA)
	tester.Type("3")
	if a.devtools.uuidCount != 3 || len(a.devtools.values) != 3 || a.devtools.uuidCountErr {
		t.Fatalf("edited count = %d values %d error %v", a.devtools.uuidCount, len(a.devtools.values), a.devtools.uuidCountErr)
	}
	for _, value := range a.devtools.values {
		if id := uuid.MustParse(value); id[6]>>4 != 7 {
			t.Fatalf("UI controls generated version %d", id[6]>>4)
		}
	}
}

func TestDevtoolsEmptyReopenKeepsSegment(t *testing.T) {
	a, _ := devtoolsWindowTestApp(t)
	a.openDevtools(devtoolsRequest{Kind: devtools.KindUUID, Version: 7, Count: 3})
	a.openDevtools(devtoolsRequest{})
	if a.devtools.tab != devtoolsUUIDTab || a.devtools.uuidVersion != 0 || len(a.devtools.values) != 2 {
		t.Fatalf("empty reopen = tab %d version %d values %d", a.devtools.tab, a.devtools.uuidVersion, len(a.devtools.values))
	}
}

func TestDevtoolsPasteDispatch(t *testing.T) {
	// Unauthorized: copy and close, no key event, no claim.
	a, _ := devtoolsWindowTestApp(t)
	posted := 0
	a.devtoolsPostKey = func(int32) error { posted++; return nil }
	var copied []string
	a.devtoolsCopy = func(text string) error { copied = append(copied, text); return nil }
	a.openDevtools(devtoolsRequest{})
	if !a.pasteDevtoolsSelected() {
		t.Fatal("the paste path did not run")
	}
	if posted != 0 || len(copied) != 1 || a.devtoolsVisible() {
		t.Fatalf("an unauthorized paste = posted %d copied %v visible %v", posted, copied, a.devtoolsVisible())
	}

	// Authorized: copy, close, then one Cmd+V aimed at the recorded app.
	a, _ = devtoolsWindowTestApp(t)
	a.devtoolsPasteAccess = func() bool { return true }
	a.devtoolsFrontmost = func() int32 {
		if !a.devtoolsVisible() {
			t.Fatal("the frontmost target was recorded after the window closed")
		}
		return 4242
	}
	targets := []int32{}
	paletteOpenAtPost := true
	a.devtoolsPostKey = func(target int32) error {
		targets = append(targets, target)
		paletteOpenAtPost = a.devtoolsVisible()
		return nil
	}
	a.openDevtools(devtoolsRequest{})
	a.pasteDevtoolsSelected()
	if len(targets) != 1 || targets[0] != 4242 {
		t.Fatalf("posted to %v", targets)
	}
	if paletteOpenAtPost {
		t.Fatal("the window was still visible when the key event was posted")
	}
}

func TestDevtoolsPasteWithoutTargetOnlyCopies(t *testing.T) {
	a, _ := devtoolsWindowTestApp(t)
	a.devtoolsPasteAccess = func() bool { return true }
	a.devtoolsFrontmost = func() int32 { return 0 }
	posted := 0
	a.devtoolsPostKey = func(int32) error { posted++; return nil }
	copied := 0
	a.devtoolsCopy = func(string) error { copied++; return nil }
	a.openDevtools(devtoolsRequest{})
	if !a.pasteDevtoolsSelected() || copied != 1 || posted != 0 || a.devtoolsVisible() {
		t.Fatalf("missing target = copied %d posted %d visible %v", copied, posted, a.devtoolsVisible())
	}
}

func TestDevtoolsPasteDelayAndCancellationSeams(t *testing.T) {
	a, _ := devtoolsWindowTestApp(t)
	a.devtoolsPasteAccess = func() bool { return true }
	a.devtoolsFrontmost = func() int32 { return 42 }
	a.devtoolsAsync = true
	a.devtoolsPasteDelay = time.Millisecond
	queued := make(chan func(), 1)
	a.devtoolsOnMainThread = func(fn func()) { queued <- fn }
	posted := 0
	a.devtoolsPostKey = func(int32) error { posted++; return nil }
	a.openDevtools(devtoolsRequest{})
	if !a.pasteDevtoolsSelected() {
		t.Fatal("the delayed paste path did not run")
	}
	select {
	case fn := <-queued:
		if posted != 0 {
			t.Fatal("the key event ran before the main-thread handoff")
		}
		fn()
		if posted != 1 {
			t.Fatalf("delayed posts = %d", posted)
		}
	case <-time.After(time.Second):
		t.Fatal("the delayed paste never reached its main-thread seam")
	}

	a.openDevtools(devtoolsRequest{})
	if !a.pasteDevtoolsSelected() {
		t.Fatal("the cancelled paste path did not run")
	}
	a.cancelDevtoolsPaste()
	select {
	case fn := <-queued:
		fn()
		if posted != 1 {
			t.Fatal("a cancelled generation posted a key")
		}
	case <-time.After(time.Second):
		t.Fatal("the cancelled paste never reached its main-thread seam")
	}
}

func TestDevtoolsPalettePasteDispatch(t *testing.T) {
	a, _ := devtoolsPalette(t)
	a.devtoolsPasteAccess = func() bool { return true }
	a.devtoolsFrontmost = func() int32 { return 99 }
	var copied []string
	a.devtoolsCopy = func(text string) error { copied = append(copied, text); return nil }
	targets := []int32{}
	paletteOpenAtPost := true
	a.devtoolsPostKey = func(target int32) error {
		targets = append(targets, target)
		paletteOpenAtPost = a.paletteVisible()
		return nil
	}
	refreshWithQuery(a, a.palette, "1700000000")
	payload := a.palette.rows[resultRowIndex(a.palette)].Result.Payload
	a.pastePaletteResult()
	if len(copied) != 1 || copied[0] != payload {
		t.Fatalf("copied %v, want %q", copied, payload)
	}
	if len(targets) != 1 || targets[0] != 99 || paletteOpenAtPost {
		t.Fatalf("posted %v, palette open at post %v", targets, paletteOpenAtPost)
	}
	if a.paletteVisible() || !a.palette.done {
		t.Fatal("the panel stayed open or lost its guard")
	}
	// A second ⌘回车 for the same row does nothing.
	a.pastePaletteResult()
	if len(targets) != 1 {
		t.Fatal("a second ⌘回车 posted again")
	}
}

func TestDevtoolsPalettePasteActionWithoutPermissionCloses(t *testing.T) {
	a, _ := devtoolsPalette(t)
	a.devtoolsPasteAccess = func() bool { return false }
	a.devtoolsFrontmost = func() int32 { return 99 }
	copied := 0
	a.devtoolsCopy = func(string) error { copied++; return nil }
	posted := 0
	a.devtoolsPostKey = func(int32) error { posted++; return nil }
	refreshWithQuery(a, a.palette, "1700000000")
	a.togglePaletteActions()
	a.refreshPalette(a.palette, "en")
	for index, row := range a.palette.rows {
		if row.Kind == paletteRowAction && row.Key == "devtools.paste" {
			a.palette.selected = index
		}
	}
	a.runPalette()
	if copied != 1 || posted != 0 || a.paletteVisible() {
		t.Fatalf("unauthorized action = copied %d posted %d visible %v", copied, posted, a.paletteVisible())
	}
}

// TestPaletteDevtoolsOpenInWindow pins the complete request: "uuid7 21" opened
// in a window still makes 21 v7 values, and a time query keeps its text.
func TestPaletteDevtoolsOpenInWindow(t *testing.T) {
	a, windows := devtoolsWindowTestApp(t)
	a.createPaletteWindow = func(*paletteState) {}
	a.devtoolsReadClipboard = func() string { return "" }
	a.showPalette()
	refreshWithQuery(a, a.palette, "uuid7 21")
	if got := a.palette.resultRequest; got.Version != 7 || got.Count != 21 {
		t.Fatalf("request = %+v", got)
	}
	a.palette.actionsDevRequest = a.palette.resultRequest
	a.openDevtools(a.palette.actionsDevRequest)
	if len(*windows) != 1 || a.devtools.tab != devtoolsUUIDTab {
		t.Fatal("the window did not open on the UUID tab")
	}
	if len(a.devtools.values) != 21 {
		t.Fatalf("values = %d", len(a.devtools.values))
	}
	for _, value := range a.devtools.values {
		if id := uuid.MustParse(value); id[6]>>4 != 7 {
			t.Fatalf("a value is version %d", id[6]>>4)
		}
	}
	// A time query prefills the time field with the same text.
	a.closeDevtools()
	refreshWithQuery(a, a.palette, "2026-11-12")
	a.openDevtools(devtoolsRequestForQuery("2026-11-12"))
	if a.devtools.tab != devtoolsTimeTab || a.devtools.input != "2026-11-12" {
		t.Fatalf("time open = tab %d input %q", a.devtools.tab, a.devtools.input)
	}
	// A UUID literal opens on its own value rather than as a new batch.
	a.closeDevtools()
	literal := "00000000-0000-4000-8000-000000000000"
	a.openDevtools(devtoolsRequestForQuery(literal))
	if a.devtools.tab != devtoolsUUIDTab || len(a.devtools.values) != 1 || a.devtools.values[0] != literal {
		t.Fatalf("literal open = %+v", a.devtools.values)
	}
}

func TestDevtoolsSettingsPageShortcutRow(t *testing.T) {
	a, _ := devtoolsWindowTestApp(t)
	if got := devtoolsPageHeight(tokens(false, "zh-CN")); got != 144 {
		t.Fatalf("page height = %v, want 144", got)
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			a.page = settingsDevtools
			tester := ui.NewTester(a.view, 720, settingsWindowHeight)
			tester.SetLocale(locale)
			tester.SetDark(dark)
			tester.Frame()
			for _, key := range []string{"Devtools", "DevtoolsPageDesc", "DevtoolsShortcutLabel", "DevtoolsRecordShortcut"} {
				if !tester.HasText(a.text(locale, key)) {
					t.Fatalf("%s dark=%v: missing %s: %v", locale, dark, key, tester.Texts())
				}
			}
		}
	}
}

// TestDevtoolsWindowViewStates draws both segments in both themes: the time
// field with results and an error, and the UUID section with a batch.
func TestDevtoolsWindowViewStates(t *testing.T) {
	a, _ := devtoolsWindowTestApp(t)
	a.openDevtools(devtoolsRequest{Kind: devtools.KindTime, Text: "2026-11-12"})
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			tk := tokens(dark, locale)
			tester := ui.NewTester(a.devtoolsView, int(tk.SizeDevtoolsWidth), int(tk.SizeDevtoolsWindowHeight))
			tester.SetLocale(locale)
			tester.SetDark(dark)
			tester.Frame()
			tester.Advance(700 * time.Millisecond)
			tester.Frame()
			if !tester.HasText("2026-11-12") {
				t.Fatalf("%s: the results are missing: %v", locale, tester.Texts())
			}
		}
	}
	a.devtools.input = "ts 946684800000"
	a.syncDevtoolsInput()
	tk := tokens(false, "zh-CN")
	tester := ui.NewTester(a.devtoolsView, int(tk.SizeDevtoolsWidth), int(tk.SizeDevtoolsWindowHeight))
	tester.SetLocale("en")
	tester.Frame()
	tester.Advance(700 * time.Millisecond)
	tester.Frame()
	if !tester.HasText(a.text("en", "DevtoolsUnit")) {
		t.Fatalf("the error is missing: %v", tester.Texts())
	}
	a.regenerateDevtoolsUUID()
	a.devtools.tab = devtoolsUUIDTab
	tester = ui.NewTester(a.devtoolsView, int(tk.SizeDevtoolsWidth), int(tk.SizeDevtoolsWindowHeight))
	tester.SetLocale("en")
	tester.Frame()
	tester.Advance(700 * time.Millisecond)
	tester.Frame()
	if !tester.HasText("v4") || !tester.HasText("v7") {
		t.Fatalf("the UUID segments are missing: %v", tester.Texts())
	}
	if !tester.HasText(a.text("en", "DevtoolsUpper")) || !tester.HasText(a.text("en", "DevtoolsPlain")) {
		t.Fatalf("the switches are missing: %v", tester.Texts())
	}
	for _, label := range []string{"v4 + v7", "v4", "v7", a.text("en", "DevtoolsCountLabel")} {
		if !tester.HasText(label) {
			t.Fatalf("UUID generation control %q is missing: %v", label, tester.Texts())
		}
	}
}

// TestDevtoolsWindowKeyboard drives the real view: Enter copies the selected
// row, and ⌘回车 copies and pastes it once before closing the window.
func TestDevtoolsWindowKeyboard(t *testing.T) {
	a, _ := devtoolsWindowTestApp(t)
	var copied []string
	a.devtoolsCopy = func(text string) error { copied = append(copied, text); return nil }
	a.openDevtools(devtoolsRequest{Kind: devtools.KindTime, Text: "1700000000"})
	tk := tokens(false, "en")
	tester := ui.NewTester(a.devtoolsView, int(tk.SizeDevtoolsWidth), int(tk.SizeDevtoolsWindowHeight))
	tester.SetLocale("en")
	tester.Frame()
	rows := a.devtoolsWindowRows("en")
	if len(copied) != 0 {
		t.Fatal("opening the window copied something")
	}
	tester.Key(0, ui.KeyEnter)
	if len(copied) != 1 || copied[0] != rows[0].Payload {
		t.Fatalf("Enter copied %v, want %q", copied, rows[0].Payload)
	}
	tester.Key(0, ui.KeyDown)
	tester.Key(ui.Cmd, ui.KeyC)
	if len(copied) != 2 || copied[1] != rows[1].Payload {
		t.Fatalf("arrow and Cmd+C copied %v, want %q", copied, rows[1].Payload)
	}
	// The UUID tab has no field, so its arrows walk the rows and Enter copies
	// the selected one.
	a.openDevtools(devtoolsRequest{Kind: devtools.KindUUID})
	uuidRows := a.devtoolsWindowRows("en")
	tester = ui.NewTester(a.devtoolsView, int(tk.SizeDevtoolsWidth), int(tk.SizeDevtoolsWindowHeight))
	tester.SetLocale("en")
	tester.Frame()
	tester.Key(0, ui.KeyDown)
	tester.Key(0, ui.KeyEnter)
	if len(copied) != 3 || copied[2] != uuidRows[1].Payload {
		t.Fatalf("arrow and Enter copied %v", copied)
	}

	// ⌘回车 on the UUID tab: the overlay has no field to compete with.
	a, _ = devtoolsWindowTestApp(t)
	a.devtoolsPasteAccess = func() bool { return true }
	a.devtoolsFrontmost = func() int32 { return 7 }
	var posted []int32
	a.devtoolsPostKey = func(target int32) error { posted = append(posted, target); return nil }
	a.openDevtools(devtoolsRequest{Kind: devtools.KindUUID, Version: 4, Count: 2})
	tester = ui.NewTester(a.devtoolsView, int(tk.SizeDevtoolsWidth), int(tk.SizeDevtoolsWindowHeight))
	tester.SetLocale("en")
	tester.Frame()
	tester.Key(ui.Cmd, ui.KeyEnter)
	if len(posted) != 1 || posted[0] != 7 {
		t.Fatalf("⌘回车 posted %v", posted)
	}
	if a.devtoolsVisible() {
		t.Fatal("⌘回车 left the window open")
	}
}

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestParsePaletteURLValid(t *testing.T) {
	cases := []struct {
		raw  string
		want paletteURLRequest
	}{
		{"cubbix://awake?minutes=60&mode=tasks", paletteURLRequest{Kind: "awake", Minutes: 60, HasMinutes: true, Mode: "tasks"}},
		{"cubbix://awake?minutes=0", paletteURLRequest{Kind: "awake", Minutes: 0, HasMinutes: true}},
		{"cubbix://awake", paletteURLRequest{Kind: "awake"}},
		{"cubbix://awake?unknown=1", paletteURLRequest{Kind: "awake"}},
		{"cubbix://awake?minutes=60&minutes=999999", paletteURLRequest{Kind: "awake", Minutes: 60, HasMinutes: true}},
		{"cubbix://awake/stop", paletteURLRequest{Kind: "awake.stop"}},
		{"cubbix://awake/stop/", paletteURLRequest{Kind: "awake.stop"}},
		{"cubbix://awake/toggle", paletteURLRequest{Kind: "awake.toggle"}},
		{"cubbix://json/new", paletteURLRequest{Kind: "json.new"}},
		{"cubbix://json/new?blank=1", paletteURLRequest{Kind: "json.new", Blank: true}},
		{"cubbix://json/new?text=abc", paletteURLRequest{Kind: "json.new", Text: "abc"}},
		{"cubbix://json/new?text=abc&blank=1", paletteURLRequest{Kind: "json.new", Text: "abc"}},
		{"cubbix://json/new?text=abcd", paletteURLRequest{Kind: "json.new", Text: "abcd"}},
		{"cubbix://palette", paletteURLRequest{Kind: "palette"}},
		{"cubbix://palette/", paletteURLRequest{Kind: "palette"}},
		{"cubbix://settings", paletteURLRequest{Kind: "settings", Page: 0}},
		{"cubbix://settings?page=general", paletteURLRequest{Kind: "settings", Page: 0}},
		{"cubbix://settings?page=tiles", paletteURLRequest{Kind: "settings", Page: 2}},
		{"cubbix://settings?page=shortcuts", paletteURLRequest{Kind: "settings", Page: 3}},
		{"cubbix://settings?page=about", paletteURLRequest{Kind: "settings", Page: 1}},
		{"cubbix://settings?page=bogus", paletteURLRequest{Kind: "settings", Page: 0}},
		{"CUBBIX://JSON/NEW/", paletteURLRequest{Kind: "json.new"}},
		{"cubbix://Awake/STOP", paletteURLRequest{Kind: "awake.stop"}},
	}
	for _, scenario := range cases {
		got, ok := parsePaletteURL(scenario.raw, true, true, 4)
		if !ok || !reflect.DeepEqual(got, scenario.want) {
			t.Fatalf("parsePaletteURL(%q) = %+v ok=%v; want %+v", scenario.raw, got, ok, scenario.want)
		}
	}
}

func TestParsePaletteURLRejectsUntrustedInput(t *testing.T) {
	cases := []string{
		"",
		"https://cubbix/awake",
		"cubbix2://awake",
		"not a url",
		"cubbix://unknown",
		"cubbix://awake/restart",
		"cubbix://awake/stop/more",
		"cubbix://json",
		"cubbix://json/../../new",
		"cubbix://json/..%2F..%2Fnew",
		"cubbix://json/new/../../awake",
		"cubbix://json/%2e%2e%2fnew",
		"cubbix://../awake",
		"cubbix://palette/extra",
		"cubbix://settings/tiles",
		"cubbix://awake?minutes=abc",
		"cubbix://awake?minutes=-1",
		"cubbix://awake?minutes=525660",
		"cubbix://awake?minutes=1.5",
		"cubbix://awake?mode=SCREEN",
		"cubbix://awake?mode=bogus",
		"cubbix://json/new?text=abcde",
	}
	for _, raw := range cases {
		if got, ok := parsePaletteURL(raw, true, true, 4); ok {
			t.Fatalf("parsePaletteURL(%q) accepted as %+v", raw, got)
		}
	}
	if _, ok := parsePaletteURL("cubbix://awake", false, true, 4); ok {
		t.Fatal("disabled keep awake accepted a deep link")
	}
	if _, ok := parsePaletteURL("cubbix://json/new", true, false, 4); ok {
		t.Fatal("disabled JSON accepted a deep link")
	}
	if _, ok := parsePaletteURL("cubbix://awake?minutes=525659", true, true, 4); !ok {
		t.Fatal("largest valid duration was rejected")
	}
}

func TestURLAwakeDoesNotWriteRememberedSettings(t *testing.T) {
	a := awakeTestApp(t)
	a.config.AwakeMinutes = 45
	a.config.AwakeUntil = ""
	a.config.AwakeMode = "screen"
	now := time.Now()

	request, ok := parsePaletteURL("cubbix://awake?minutes=60", true, true, jsonFileLimit)
	if !ok {
		t.Fatal("valid URL rejected")
	}
	a.runURL(request)
	state := a.awake.snapshot()
	if !state.Active || state.Session.Mode != "screen" {
		t.Fatalf("session = %+v", state)
	}
	if delta := time.Until(state.Session.End); delta < 59*time.Minute || delta > 61*time.Minute {
		t.Fatalf("duration = %v", delta)
	}
	if a.config.AwakeMinutes != 45 || a.config.AwakeUntil != "" || a.config.AwakeMode != "screen" {
		t.Fatalf("URL changed remembered settings: %+v", a.config)
	}

	// Pressing "常亮" afterwards still uses the remembered 45 minutes.
	request, _ = parsePaletteURL("cubbix://awake?mode=tasks", true, true, jsonFileLimit)
	a.runURL(request)
	state = a.awake.snapshot()
	if state.Session.Mode != "tasks" || state.Session.End.Sub(now) > 46*time.Minute {
		t.Fatalf("remembered duration or requested mode lost: %+v", state)
	}
	if a.config.AwakeMinutes != 45 || a.config.AwakeMode != "screen" {
		t.Fatalf("mode request wrote the remembered mode: %+v", a.config)
	}
}

func TestURLJSONNewNeverReadsClipboard(t *testing.T) {
	a := awakeTestApp(t)
	a.jsonReadClipboard = func() string { t.Fatal("json/new must not read the clipboard"); return "" }
	var opened []string
	a.paletteOpenJSON = func(text string) { opened = append(opened, text) }

	blank, _ := parsePaletteURL("cubbix://json/new", true, true, jsonFileLimit)
	a.runURL(blank)
	withText, _ := parsePaletteURL("cubbix://json/new?text=%7B%22a%22%3A1%7D", true, true, jsonFileLimit)
	a.runURL(withText)
	if want := []string{"", `{"a":1}`}; !reflect.DeepEqual(opened, want) {
		t.Fatalf("opened = %q; want %q", opened, want)
	}
	if len(a.jsonDocuments) != 0 {
		t.Fatal("URL must not create documents outside the open path")
	}
}

func TestURLSettingsSelectsPage(t *testing.T) {
	a := commandTestApp(t)
	a.page, a.notesPage = 1, true
	request, ok := parsePaletteURL("cubbix://settings?page=shortcuts", true, true, jsonFileLimit)
	if !ok {
		t.Fatal("valid settings URL rejected")
	}
	a.runURL(request)
	if a.page != 3 || a.notesPage {
		t.Fatalf("settings page = %d notes=%v", a.page, a.notesPage)
	}
	request, _ = parsePaletteURL("cubbix://settings", true, true, jsonFileLimit)
	a.page = 2
	a.runURL(request)
	if a.page != 0 {
		t.Fatalf("default settings page = %d", a.page)
	}
}

func TestPaletteConfigDefaultsAndNormalization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("version=1\nlanguage='zh-CN'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig(path)
	if err != nil || config.PaletteShortcut != defaultPaletteKey || len(config.PaletteFavorites) != 0 || len(config.PaletteRecents) != 0 {
		t.Fatalf("legacy config = %+v %v", config, err)
	}
	raw := "version=1\nlanguage='zh-CN'\npalette_shortcut=''\npalette_favorites=['a','a','','b']\npalette_recents=['1','2','3','4','5','6','7']\n"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	config, err = loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.PaletteShortcut != defaultPaletteKey {
		t.Fatalf("empty palette shortcut = %q", config.PaletteShortcut)
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(config.PaletteFavorites, want) {
		t.Fatalf("favorites = %v", config.PaletteFavorites)
	}
	if want := []string{"1", "2", "3", "4", "5"}; !reflect.DeepEqual(config.PaletteRecents, want) {
		t.Fatalf("recents = %v", config.PaletteRecents)
	}
	config.PaletteFavorites = []string{"app.quit", "settings.page.about"}
	if err := saveConfig(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil || !reflect.DeepEqual(loaded, config) {
		t.Fatalf("palette settings did not round trip: %+v %v", loaded, err)
	}
}

func TestPalettePosition(t *testing.T) {
	display := mygo.Display{WorkArea: mygo.Rectangle{X: 0, Y: 0, Width: 1440, Height: 900}}
	if x, y := palettePosition(display, 640, 400, 96); x != 400 || y != 96 {
		t.Fatalf("position = %d,%d", x, y)
	}
	short := mygo.Display{WorkArea: mygo.Rectangle{X: 0, Y: 0, Width: 600, Height: 420}}
	if x, y := palettePosition(short, 640, 400, 96); x != -20 || y != 20 {
		t.Fatalf("short display = %d,%d", x, y)
	}
	offset := mygo.Display{WorkArea: mygo.Rectangle{X: -1920, Y: 25, Width: 1920, Height: 1055}}
	if x, y := palettePosition(offset, 640, 400, 96); x != -1920+(1920-640)/2 || y != 121 {
		t.Fatalf("offset display = %d,%d", x, y)
	}
}

func TestShortcutSlotsAreMutuallyExclusive(t *testing.T) {
	a := commandTestApp(t)
	if a.config.PaletteShortcut != "Alt+." || sameShortcut(a.config.PaletteShortcut, a.config.AwakeShortcut) || sameShortcut(a.config.PaletteShortcut, a.config.JSONShortcut) {
		t.Fatalf("defaults collide: %+v", a.config)
	}
	if reason := a.shortcutConflict(a.config.PaletteShortcut, "json"); reason != "JSONShortcutConflict" {
		t.Fatalf("palette/json conflict = %q", reason)
	}
	if reason := a.shortcutConflict(a.config.PaletteShortcut, "palette"); reason != "" {
		t.Fatalf("palette should not conflict with itself: %q", reason)
	}
	a.config.PaletteShortcut = a.config.AwakeShortcut
	if reason := a.shortcutConflict(a.config.PaletteShortcut, "palette"); reason == "" {
		t.Fatal("palette/awake conflict not detected")
	}
	if reason := a.shortcutConflict(a.config.PaletteShortcut, "awake"); reason == "" {
		t.Fatal("awake/palette conflict not detected")
	}
	if acc := shortcutKey(ui.Alt, ui.KeyPeriod); acc != "Alt+." {
		t.Fatalf("recording Alt+. = %q", acc)
	}
	if knownShortcutConflict("Alt+.") != "" {
		t.Fatal("Alt+. is listed as a known conflict")
	}
	a.config.AwakeEnabled = false
	a.config.JSONEnabled = false
	if reason := a.shortcutConflict(a.config.AwakeShortcut, "palette"); reason != "" {
		t.Fatalf("disabled slots still conflict: %q", reason)
	}
	slots := a.shortcutSlots()
	if len(slots) != 3 || slots[0].Target != "awake" || slots[1].Target != "json" || slots[2].Target != "palette" {
		t.Fatalf("slots = %+v", slots)
	}
}

func TestPaletteCommandFailureKeepsPanelOpen(t *testing.T) {
	a := awakeTestApp(t)
	a.awake.launch = func([]string) (awakeChild, error) { return nil, errors.New("launch failed") }
	a.settingsError = ""
	command := findCommand(a.commands(), "awake.start")
	if command == nil {
		t.Fatal("missing awake.start")
	}
	a.palette = &paletteState{
		mode:         paletteCommandMode,
		selected:     0,
		selectionKey: command.ID,
		rows:         []paletteRow{{Kind: paletteRowCommand, Key: command.ID, Command: command}},
		request:      request{Command: command, Arg: "30m", Awake: awakeArg{Kind: awakeArgMinutes, Minutes: 30}},
	}
	a.runPalette()
	if a.palette == nil || a.palette.errorKey != "AwakeStartError" {
		t.Fatalf("panel did not keep the error: %+v", a.palette)
	}
	if a.settingsError != "" {
		t.Fatalf("palette failure polluted the settings error: %q", a.settingsError)
	}
}

func TestInvalidParameterNeverRuns(t *testing.T) {
	a := awakeTestApp(t)
	called := false
	command := &command{
		ID:    "awake.start",
		Title: localizedTitle("R05Start"),
		Param: paramAwake,
		Run: func(a *application, req request) error {
			called = true
			return nil
		},
	}
	a.palette = &paletteState{
		mode:     paletteCommandMode,
		selected: 0,
		rows:     []paletteRow{{Kind: paletteRowCommand, Key: command.ID, Command: command}},
		request:  request{Command: command, Arg: "45x", Awake: awakeArg{Error: "DurationError"}},
	}
	a.runPalette()
	if called {
		t.Fatal("invalid parameter executed the command")
	}
	if a.palette.errorKey != "DurationError" {
		t.Fatalf("error key = %q", a.palette.errorKey)
	}
}

func TestFavoritesAndRecents(t *testing.T) {
	a := awakeTestApp(t)
	a.toggleFavorite("app.quit")
	a.toggleFavorite("awake.stop")
	if want := []string{"app.quit", "awake.stop"}; !reflect.DeepEqual(a.config.PaletteFavorites, want) {
		t.Fatalf("favorites = %v", a.config.PaletteFavorites)
	}
	a.toggleFavorite("awake.stop")
	if !a.isFavorite("app.quit") || a.isFavorite("awake.stop") {
		t.Fatalf("favorites after removal = %v", a.config.PaletteFavorites)
	}
	for index := 0; index < 7; index++ {
		a.recordRecent(fmt.Sprintf("command.%d", index))
	}
	if want := []string{"command.6", "command.5", "command.4", "command.3", "command.2"}; !reflect.DeepEqual(a.config.PaletteRecents, want) {
		t.Fatalf("recents = %v; want %v", a.config.PaletteRecents, want)
	}
	a.recordRecent("command.4")
	if a.config.PaletteRecents[0] != "command.4" || len(a.config.PaletteRecents) != 5 {
		t.Fatalf("recents after reuse = %v", a.config.PaletteRecents)
	}
	a.recordRecent("json.window.1")
	if a.config.PaletteRecents[0] != "command.4" {
		t.Fatalf("JSON window entered recents: %v", a.config.PaletteRecents)
	}
	loaded, err := loadConfig(a.path)
	if err != nil || !reflect.DeepEqual(loaded.PaletteFavorites, a.config.PaletteFavorites) || !reflect.DeepEqual(loaded.PaletteRecents, a.config.PaletteRecents) {
		t.Fatalf("palette state not persisted: %+v %v", loaded, err)
	}
}

func TestAwakeErrorsReturnWithoutSettingsSlot(t *testing.T) {
	a := awakeTestApp(t)
	a.settingsError = ""
	if err := a.setMode("bogus"); err == nil {
		t.Fatal("invalid mode accepted")
	}
	if a.settingsError != "" {
		t.Fatalf("setMode wrote the settings error: %q", a.settingsError)
	}
	now := time.Now()
	a.awake.now = func() time.Time { return now }
	if err := a.startAwake(awakeSession{Mode: "screen", End: now.Add(-time.Minute)}); err == nil {
		t.Fatal("expired session accepted")
	}
	if a.settingsError != "" {
		t.Fatalf("startAwake wrote the settings error: %q", a.settingsError)
	}
	a.awake.alive = func(int, string) bool { return false }
	if err := a.startAwake(awakeSession{Mode: "tasks", PID: 12, Identity: "gone"}); err == nil {
		t.Fatal("ended follow target accepted")
	}
	if a.settingsError != "" {
		t.Fatalf("follow failure wrote the settings error: %q", a.settingsError)
	}
	a.awake.launch = func([]string) (awakeChild, error) { return nil, errors.New("launch failed") }
	if err := a.setDuration(30); err == nil {
		t.Fatal("setDuration hid a launch failure")
	}
	if a.settingsError != "" {
		t.Fatalf("setDuration wrote the settings error: %q", a.settingsError)
	}
	// The menu path keeps reporting in the settings page.
	a.reportAwakeError(errors.New("launch failed"))
	if a.settingsError != "AwakeStartError" {
		t.Fatalf("menu path did not report: %q", a.settingsError)
	}
}

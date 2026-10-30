package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// panelTestApp is awakeTestApp with both window factories replaced, so no
// case creates a native flyout or panel.
func panelTestApp(t *testing.T) *application {
	t.Helper()
	a := awakeTestApp(t)
	a.createMenubarPanel = func(*menubarPanelState) {}
	a.createPaletteWindow = func(*paletteState) {}
	return a
}

func menuLabels(menu *mygo.Menu) []string {
	labels := []string{}
	for _, item := range menu.Items() {
		if item.Type == mygo.MenuItemSeparator {
			continue
		}
		labels = append(labels, item.Label)
	}
	return labels
}

// panelView renders the panel at its content height. It opens the panel when
// it is closed, which the height and the view both need.
func panelView(a *application, locale string) *ui.Tester {
	if a.panel == nil {
		a.showMenubarPanel()
	}
	t := tokens(false, locale)
	tt := ui.NewTester(a.menubarPanelView, int(t.SizeMenubarPanelWidth), a.menubarPanelHeight(t))
	tt.SetLocale(locale)
	return tt
}

func TestMenubarPanelToggleAndMutex(t *testing.T) {
	a := panelTestApp(t)
	a.toggleMenubarPanel()
	if !a.menubarPanelVisible() {
		t.Fatal("the tray click did not open the panel")
	}
	a.toggleMenubarPanel()
	if a.menubarPanelVisible() {
		t.Fatal("the second tray click did not close the panel")
	}
	// Opening the palette closes the card panel.
	a.toggleMenubarPanel()
	a.showPalette()
	if a.menubarPanelVisible() || !a.paletteVisible() {
		t.Fatalf("palette and panel stack: panel=%v palette=%v", a.menubarPanelVisible(), a.paletteVisible())
	}
	// Opening the card panel closes the palette, both from the tray click
	// and from the footer button.
	a.toggleMenubarPanel()
	if !a.menubarPanelVisible() || a.paletteVisible() {
		t.Fatalf("panel did not replace the palette: panel=%v palette=%v", a.menubarPanelVisible(), a.paletteVisible())
	}
	a.showPalette()
	a.toggleMenubarPanel()
	if !a.menubarPanelVisible() || a.paletteVisible() {
		t.Fatalf("toggle from the tray kept the palette: panel=%v palette=%v", a.menubarPanelVisible(), a.paletteVisible())
	}
	// ⌥. while the card panel shows closes it and opens the palette; the
	// second ⌥. closes the palette without reopening the card.
	a.togglePalette()
	if a.menubarPanelVisible() || !a.paletteVisible() {
		t.Fatalf("⌥. stacked the panels: panel=%v palette=%v", a.menubarPanelVisible(), a.paletteVisible())
	}
	a.togglePalette()
	if a.menubarPanelVisible() || a.paletteVisible() {
		t.Fatalf("the second ⌥. left a panel open: panel=%v palette=%v", a.menubarPanelVisible(), a.paletteVisible())
	}
}

func TestMenubarPanelClearsAFailedAction(t *testing.T) {
	a := panelTestApp(t)
	a.awake.launch = func([]string) (awakeChild, error) { return nil, errors.New("launch failed") }
	a.showMenubarPanel()
	a.panelSetDuration(30)
	if a.menubarPanelErrorKey() == "" {
		t.Fatal("a failed start left no error")
	}
	if a.settingsError != "" {
		t.Fatalf("the panel wrote the settings error slot: %q", a.settingsError)
	}
	a.awake.launch = func([]string) (awakeChild, error) { return &fakeAwakeChild{done: make(chan error, 1)}, nil }
	a.panelSetDuration(30)
	if !a.awake.snapshot().Active || a.menubarPanelErrorKey() != "" {
		t.Fatalf("the retry did not clear the error: %+v", a.awake.snapshot())
	}
}

func TestMenubarPanelPillStartsWithRememberedMode(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		a := panelTestApp(t)
		a.config.AwakeMode = "tasks"
		a.toggleMenubarPanel()
		tt := panelView(a, locale)
		if err := tt.Click(a.text(locale, "PanelDuration1h")); err != nil {
			t.Fatal(err)
		}
		state := a.awake.snapshot()
		if !state.Active || state.Session.Mode != "tasks" {
			t.Fatalf("%s: pill did not start with the remembered mode: %+v", locale, state)
		}
		if a.config.AwakeMinutes != 60 || a.config.AwakeUntil != "" {
			t.Fatalf("%s: memory = %d %q", locale, a.config.AwakeMinutes, a.config.AwakeUntil)
		}
		// A running session keeps the panel open and now shows its status.
		if !a.menubarPanelVisible() {
			t.Fatal("a pill click closed the panel")
		}
		if !tt.HasText(a.text(locale, "R05Tasks")) {
			t.Fatalf("%s: missing mode in the status: %v", locale, tt.Texts())
		}
		if length := len(tt.Texts()); length == 0 {
			t.Fatal("empty render")
		}
	}
}

func TestMenubarPanelPillResetsAndKeepsSelection(t *testing.T) {
	a := panelTestApp(t)
	tt := panelView(a, "zh-CN")
	if err := tt.Click("1h"); err != nil {
		t.Fatal(err)
	}
	first := a.awake.snapshot().Session.End
	if err := tt.Click("30m"); err != nil {
		t.Fatal(err)
	}
	state := a.awake.snapshot()
	if a.config.AwakeMinutes != 30 {
		t.Fatalf("memory after reset = %d", a.config.AwakeMinutes)
	}
	if left := time.Until(state.Session.End); left > 31*time.Minute || left < 29*time.Minute {
		t.Fatalf("reset did not restart the countdown: %v", left)
	}
	// Clicking the pill that already runs changes nothing.
	again := state.Session.End
	if err := tt.Click("30m"); err != nil {
		t.Fatal(err)
	}
	if !a.awake.snapshot().Session.End.Equal(again) {
		t.Fatal("the running preset restarted its countdown")
	}
	if first.Equal(again) {
		t.Fatal("the reset kept the old deadline")
	}
	// ∞ is zero minutes and leaves no deadline.
	if err := tt.Click("∞"); err != nil {
		t.Fatal(err)
	}
	if a.config.AwakeMinutes != 0 || !a.awake.snapshot().Session.End.IsZero() {
		t.Fatal("∞ did not start an unlimited session")
	}
}

func TestMenubarPanelSwitchStartsAndStops(t *testing.T) {
	a := panelTestApp(t)
	a.config.AwakeMode = "screen"
	a.config.AwakeMinutes = 120
	tt := panelView(a, "zh-CN")
	if err := tt.Click("开启常亮"); err != nil {
		t.Fatal(err)
	}
	state := a.awake.snapshot()
	if !state.Active || state.Session.Mode != "screen" {
		t.Fatalf("switch did not start the remembered session: %+v", state)
	}
	if left := time.Until(state.Session.End); left > 121*time.Minute || left < 119*time.Minute {
		t.Fatalf("switch lost the remembered duration: %v", left)
	}
	if err := tt.Click("停止"); err != nil {
		t.Fatal(err)
	}
	if a.awake.snapshot().Active {
		t.Fatal("the switch did not stop keep awake")
	}
}

func TestMenubarPanelModeHotSwitches(t *testing.T) {
	a := panelTestApp(t)
	if err := a.setDuration(60); err != nil {
		t.Fatal(err)
	}
	before := a.awake.snapshot()
	tt := panelView(a, "zh-CN")
	if err := tt.Click("仅保持任务"); err != nil {
		t.Fatal(err)
	}
	state := a.awake.snapshot()
	if !state.Active || state.Session.Mode != "tasks" || a.config.AwakeMode != "tasks" {
		t.Fatalf("mode did not hot switch: %+v", state)
	}
	if !state.Session.End.Equal(before.Session.End) {
		t.Fatal("mode switch moved the deadline")
	}
	if err := tt.Click("保持屏幕常亮"); err != nil {
		t.Fatal(err)
	}
	if mode := a.awake.snapshot().Session.Mode; mode != "screen" {
		t.Fatalf("mode = %q", mode)
	}
}

func TestMenubarPanelStatusLines(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		a := panelTestApp(t)
		tt := panelView(a, locale)
		if tt.HasText(a.text(locale, "R05Off")) {
			t.Fatal("the off state shows a status line")
		}
		if _, ok := tt.Find(a.text(locale, "R05Screen")); !ok {
			t.Fatal("missing mode switch")
		}
		// Time left.
		if err := a.setDuration(90); err != nil {
			t.Fatal(err)
		}
		tt = panelView(a, locale)
		if !tt.HasText(a.text(locale, "R05Screen") + " · " + fmt.Sprintf(a.text(locale, "AwakeRemaining"), 1, 30)) {
			t.Fatalf("%s: remaining line missing: %v", locale, tt.Texts())
		}
		// A deadline wins over the countdown.
		a.config.AwakeUntil = "18:30"
		if err := a.startAwake(awakeSession{Mode: "screen", End: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
		tt = panelView(a, locale)
		if !tt.HasText(fmt.Sprintf(a.text(locale, "AwakeUntil"), "18:30")) {
			t.Fatalf("%s: deadline line missing: %v", locale, tt.Texts())
		}
		if _, ok := a.panelPreset(time.Now()); ok {
			t.Fatal("a deadline highlighted a preset")
		}
		// A followed process names itself, without a PID.
		a.config.AwakeUntil = ""
		if err := a.startAwake(awakeSession{Mode: "tasks", PID: 42, Identity: "identity", Name: "codex"}); err != nil {
			t.Fatal(err)
		}
		tt = panelView(a, locale)
		if !tt.HasText(fmt.Sprintf(a.text(locale, "AwakeFollowing"), "codex")) {
			t.Fatalf("%s: follow line missing: %v", locale, tt.Texts())
		}
		if _, ok := a.panelPreset(time.Now()); ok {
			t.Fatal("a followed process highlighted a preset")
		}
		// No deadline at all.
		if err := a.setDuration(0); err != nil {
			t.Fatal(err)
		}
		tt = panelView(a, locale)
		if !tt.HasText(a.text(locale, "R05Unlimited")) {
			t.Fatalf("%s: unlimited line missing: %v", locale, tt.Texts())
		}
		if preset, ok := a.panelPreset(time.Now()); !ok || preset != 0 {
			t.Fatal("an unlimited session did not highlight ∞")
		}
	}
}

func TestMenubarPanelProcessEndedWarning(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		a := panelTestApp(t)
		a.config.AwakeMinutes = 60
		a.awake.mu.Lock()
		a.awake.state = awakeState{Error: "ProcessEnded"}
		a.awake.mu.Unlock()
		tt := panelView(a, locale)
		if !tt.HasText(a.text(locale, "ProcessEnded")) {
			t.Fatalf("%s: the ended process is not reported: %v", locale, tt.Texts())
		}
		// The warning sits in its own row, so the card grows by one.
		token := tokens(false, locale)
		if got, want := a.menubarPanelHeight(token), int(2*token.Space12+awakeCardHeight(token, true)+token.Space8+jsonCardHeight(token, 0)+token.Space8+token.StrokeHairline+token.SizeNavHeight); got != want {
			t.Fatalf("%s: height with warning = %d, want %d", locale, got, want)
		}
	}
}

func TestMenubarPanelJSONListAndFold(t *testing.T) {
	a := panelTestApp(t)
	a.config.AwakeEnabled = false
	a.toggleMenubarPanel()
	// No window: the list area folds away and only the action remains.
	rows := a.menubarPanelRows()
	if len(rows.documents) != 0 || rows.listRows != 0 {
		t.Fatalf("empty list rows = %+v", rows)
	}
	tt := panelView(a, "zh-CN")
	if !tt.HasText(a.text("zh-CN", "JSONShortcutLabel")) {
		t.Fatalf("missing the new window action: %v", tt.Texts())
	}
	for index := range 3 {
		a.addJSONDocument("", fmt.Sprintf("items-%d.json", index), true)
	}
	tt.Frame()
	if rows := a.menubarPanelRows(); rows.listRows != 3 || rows.folded {
		t.Fatalf("three windows = %+v", rows)
	}
	for index := range 4 {
		a.addJSONDocument("", fmt.Sprintf("more-%d.json", index), true)
	}
	tt.Frame()
	label := fmt.Sprintf(a.text("zh-CN", "PanelJSONWindowsCount"), 7)
	if !tt.HasText(label) {
		t.Fatalf("missing the folded count: %v", tt.Texts())
	}
	if tt.HasText("items-0.json") {
		t.Fatal("a folded list still shows its windows")
	}
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	// The list grows the panel, so the next frame needs the new height.
	tt = panelView(a, "zh-CN")
	if !a.panelExpanded() || !tt.HasText("items-0.json") {
		t.Fatalf("the fold did not open: %v", tt.Texts())
	}
	// A window row closes the panel and asks for that window.
	if err := tt.Click("items-2.json"); err != nil {
		t.Fatal(err)
	}
	if a.menubarPanelVisible() {
		t.Fatal("a window row left the panel open")
	}
	// With six or more again, a fresh panel starts folded and opens again.
	a.showMenubarPanel()
	if a.panelExpanded() {
		t.Fatal("a new panel opened expanded")
	}
	tt = panelView(a, "zh-CN")
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	if !a.panelExpanded() {
		t.Fatal("the fold did not open from a fresh panel")
	}
}

func TestMenubarPanelNewJSONClosesFirst(t *testing.T) {
	a := panelTestApp(t)
	a.toggleMenubarPanel()
	// The real button creates a native window, which only the native helper
	// may do. Stopping before it keeps the order under test: the panel goes
	// first, so the new window never opens behind it.
	a.config.JSONEnabled = false
	a.panelNewJSON()
	if a.menubarPanelVisible() {
		t.Fatal("the new window action left the panel open")
	}
	if a.settingsError != "" {
		t.Fatalf("the panel wrote the settings error slot: %q", a.settingsError)
	}
}

func TestMenubarPanelPinnedJSONWindows(t *testing.T) {
	a := panelTestApp(t)
	a.config.AwakeEnabled = false
	d := a.addJSONDocument("", "pinned.json", true)
	d.pinned = true
	a.toggleMenubarPanel()
	tt := panelView(a, "zh-CN")
	if !tt.HasText("pinned.json") {
		t.Fatalf("missing window: %v", tt.Texts())
	}
	// The pin mark carries the window name and shows at the small icon size.
	if _, ok := tt.Find("pinned.json"); !ok {
		t.Fatal("missing the pinned row")
	}
	token := tokens(false, "zh-CN")
	if token.SizeIconSmall != 12 {
		t.Fatalf("SizeIconSmall = %v", token.SizeIconSmall)
	}
}

func TestMenubarPanelTileToggles(t *testing.T) {
	a := panelTestApp(t)
	a.config.AwakeEnabled = false
	a.config.JSONEnabled = false
	a.toggleMenubarPanel()
	tt := panelView(a, "en")
	if !tt.HasText(a.text("en", "PanelEmpty")) {
		t.Fatalf("missing the empty placeholder: %v", tt.Texts())
	}
	if tt.HasText(a.text("en", "Awake")) || tt.HasText(a.text("en", "JSONShortcutLabel")) {
		t.Fatal("a disabled tile still draws")
	}
	a.config.AwakeEnabled = true
	a.changeAwakeEnabled()
	tt.Frame()
	if !tt.HasText(a.text("en", "Awake")) || tt.HasText(a.text("en", "PanelEmpty")) {
		t.Fatalf("the awake tile did not come back: %v", tt.Texts())
	}
	a.config.JSONEnabled = true
	a.changeJSONEnabled()
	tt.Frame()
	if !tt.HasText(a.text("en", "JSONShortcutLabel")) {
		t.Fatalf("the JSON tile did not come back: %v", tt.Texts())
	}
}

func TestContextMenuKeepsOnlySystemActions(t *testing.T) {
	a := panelTestApp(t)
	for _, locale := range []string{"zh-CN", "en"} {
		want := []string{a.text(locale, "MenuSettings"), a.text(locale, "Quit")}
		if got := menuLabels(a.contextMenu(locale)); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("%s: menu = %q, want %q", locale, got, want)
		}
		a.updates.state = updateState{Phase: updateReady, Version: "0.0.4"}
		want = append([]string{a.text(locale, "MenuSettings"), a.text(locale, "MenuRestartUpdate")}, a.text(locale, "Quit"))
		if got := menuLabels(a.contextMenu(locale)); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("%s: ready menu = %q, want %q", locale, got, want)
		}
		a.updates.state = updateState{}
	}
}

func TestMenubarPanelHeight(t *testing.T) {
	tt := tokens(false, "zh-CN")
	cases := []struct {
		name        string
		awake, json bool
		want        int
	}{
		{"both tiles, no windows", true, true, 285},
		{"awake only", true, false, 189},
		{"json only", false, true, 153},
		{"all tiles off", false, false, 97},
	}
	for _, c := range cases {
		a := panelTestApp(t)
		a.config.AwakeEnabled, a.config.JSONEnabled = c.awake, c.json
		if got := a.menubarPanelHeight(tt); got != c.want {
			t.Fatalf("%s: height = %d, want %d", c.name, got, c.want)
		}
	}
	a := panelTestApp(t)
	// The status line adds a small row.
	if err := a.setDuration(30); err != nil {
		t.Fatal(err)
	}
	withStatus := a.menubarPanelHeight(tt)
	if withStatus != 313 {
		t.Fatalf("height with status = %d, want 313", withStatus)
	}
	a.awake.stop()
	withoutStatus := a.menubarPanelHeight(tt)
	if withStatus-withoutStatus != int(tt.Space8+tt.Space20) {
		t.Fatalf("status row = %d", withStatus-withoutStatus)
	}
	// Each JSON window adds a row and one small gap.
	three := 285 + 3*int(tt.SizeNavHeight) + 2*int(tt.Space2) + int(tt.Space8)
	for index := range 3 {
		a.addJSONDocument("", fmt.Sprintf("row-%d.json", index), true)
	}
	if got := a.menubarPanelHeight(tt); got != three {
		t.Fatalf("height with three windows = %d, want %d", got, three)
	}
	// Six windows fold to one row until expanded, and gain the fold row after.
	for index := range 4 {
		a.addJSONDocument("", fmt.Sprintf("more-%d.json", index), true)
	}
	folded := 285 + int(tt.SizeNavHeight+tt.Space8)
	if got := a.menubarPanelHeight(tt); got != folded {
		t.Fatalf("folded height = %d, want %d", got, folded)
	}
	a.panel = &menubarPanelState{expanded: true}
	expanded := 285 + 7*int(tt.SizeNavHeight) + 6*int(tt.Space2) + int(tt.Space8) + int(tt.SizeNavHeight+tt.Space2)
	if got := a.menubarPanelHeight(tt); got != expanded {
		t.Fatalf("expanded height = %d, want %d", got, expanded)
	}
	if expanded <= folded {
		t.Fatal("expanding did not grow the panel")
	}
}

func TestMenubarPanelPresets(t *testing.T) {
	a := panelTestApp(t)
	now := time.Now()
	for _, preset := range []int{30, 60, 120, 0} {
		a.config.AwakeMinutes, a.config.AwakeUntil = preset, ""
		if got, ok := a.panelPreset(now); !ok || got != preset {
			t.Fatalf("remembered %d highlighted %d/%v", preset, got, ok)
		}
	}
	// Four hours and custom minutes have no pill.
	for _, minutes := range []int{240, 45} {
		a.config.AwakeMinutes = minutes
		if _, ok := a.panelPreset(now); ok {
			t.Fatalf("%d minutes highlighted a pill", minutes)
		}
	}
	a.config.AwakeMinutes, a.config.AwakeUntil = 60, "18:30"
	if _, ok := a.panelPreset(now); ok {
		t.Fatal("a deadline highlighted a pill")
	}
}

func TestMenubarPanelFooterActions(t *testing.T) {
	a := panelTestApp(t)
	a.toggleMenubarPanel()
	tt := panelView(a, "en")
	if !tt.HasText("Cubbix v" + runningVersion()) {
		t.Fatalf("missing the version: %v", tt.Texts())
	}
	// The command palette button closes the panel and opens the palette.
	if err := tt.Click(a.text("en", "PanelCommandPalette")); err != nil {
		t.Fatal(err)
	}
	if a.menubarPanelVisible() || !a.paletteVisible() {
		t.Fatalf("footer palette button: panel=%v palette=%v", a.menubarPanelVisible(), a.paletteVisible())
	}
	// Settings closes the panel too; no settings window exists in tests.
	a.toggleMenubarPanel()
	tt = panelView(a, "en")
	if err := tt.Click(a.text("en", "MenuSettings")); err != nil {
		t.Fatal(err)
	}
	if a.menubarPanelVisible() {
		t.Fatal("the settings button left the panel open")
	}
}

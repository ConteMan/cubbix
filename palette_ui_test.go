package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func paletteTestApp(t *testing.T) *application {
	t.Helper()
	a := awakeTestApp(t)
	a.palette = &paletteState{}
	return a
}

func TestPaletteViewSearchesAndRuns(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		a := paletteTestApp(t)
		tt := ui.NewTester(a.paletteView, 640, 400)
		tt.SetLocale(locale)
		if !tt.HasText(a.text(locale, "PaletteAll")) || !tt.HasText(a.text(locale, "R05Start")) {
			t.Fatalf("%s: empty palette misses groups or commands: %v", locale, tt.Texts())
		}
		// A whole command name plus a space selects that command.
		tt.Type(a.text(locale, "R05Start") + " ")
		if a.palette.mode != paletteCommandMode || a.palette.selectionKey != "awake.start" {
			t.Fatalf("%s: typing did not select awake.start: %q", locale, a.palette.selectionKey)
		}
	}
}

func TestPaletteViewQueryResetsSelection(t *testing.T) {
	a := paletteTestApp(t)
	a.config.PaletteFavorites = []string{"awake.stop"}
	if err := a.awake.start(awakeSession{Mode: "screen"}); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.paletteView, 640, 400)
	tt.SetLocale("zh-CN")
	if a.palette.selectionKey != "awake.stop" {
		t.Fatalf("initial selection = %q", a.palette.selectionKey)
	}
	tt.Type("常亮")
	if a.palette.selectionKey != "awake.start" {
		t.Fatalf("query did not move the default selection: %q", a.palette.selectionKey)
	}
}

func TestPaletteViewParameterHintAndError(t *testing.T) {
	a := paletteTestApp(t)
	tt := ui.NewTester(a.paletteView, 640, 400)
	tt.SetLocale("zh-CN")
	tt.Type("常亮 45m")
	if a.palette.errorKey != "" {
		t.Fatalf("valid parameter flagged: %q", a.palette.errorKey)
	}
	if !tt.HasText("45 分钟") {
		t.Fatalf("missing duration hint: %v", tt.Texts())
	}
	if a.palette.selectionKey != "awake.start" {
		t.Fatalf("selection = %q", a.palette.selectionKey)
	}
	tt.Type("x")
	if a.palette.errorKey != "DurationError" {
		t.Fatalf("invalid parameter error = %q", a.palette.errorKey)
	}
	if !tt.HasText("45mx") {
		t.Fatalf("invalid parameter should show the raw text: %v", tt.Texts())
	}
	before := a.awake.snapshot()
	tt.Key(0, ui.KeyEnter)
	if a.awake.snapshot().Active || before.Active {
		t.Fatal("Enter ran a command with an invalid parameter")
	}
	if a.palette.errorKey != "DurationError" {
		t.Fatal("Enter cleared the parameter error")
	}
}

func TestPaletteViewRunsAndActions(t *testing.T) {
	a := paletteTestApp(t)
	tt := ui.NewTester(a.paletteView, 640, 400)
	tt.SetLocale("zh-CN")
	tt.Type("常亮 30m")
	tt.Key(ui.Cmd, ui.KeyK)
	if a.palette.mode != paletteActionMode {
		t.Fatal("⌘K did not open the action list")
	}
	for _, text := range []string{"保持屏幕常亮", "仅保持任务", "1 小时", "直到某时刻…", "加入收藏"} {
		if !tt.HasText(text) {
			t.Fatalf("action list missing %q: %v", text, tt.Texts())
		}
	}
	// Esc returns to the command list without closing the panel.
	tt.Key(0, ui.KeyEscape)
	if a.palette.mode != paletteCommandMode {
		t.Fatal("Esc in the action list did not return")
	}
	// The mode action keeps the typed duration.
	tt.Key(ui.Cmd, ui.KeyK)
	if err := tt.Click("仅保持任务"); err != nil {
		t.Fatal(err)
	}
	state := a.awake.snapshot()
	if !state.Active || state.Session.Mode != "tasks" {
		t.Fatalf("mode action = %+v", state)
	}
	if delta := time.Until(state.Session.End); delta < 29*time.Minute || delta > 31*time.Minute {
		t.Fatalf("duration lost by action = %v", delta)
	}
}

func TestPaletteViewArrowKeysSkipHeaders(t *testing.T) {
	a := paletteTestApp(t)
	a.addJSONDocument("", "", true)
	a.addJSONDocument("", "", true)
	tt := ui.NewTester(a.paletteView, 640, 400)
	tt.SetLocale("zh-CN")
	tt.Type("json")
	// json.new is selected even though a document named JSON has a better
	// title score; the default follows registration order.
	if a.palette.selectionKey != "json.new" {
		t.Fatalf("default selection = %q", a.palette.selectionKey)
	}
	tt.Key(0, ui.KeyUp)
	if a.palette.selectionKey != "json.window.2" {
		t.Fatalf("Up selection = %q", a.palette.selectionKey)
	}
	tt.Key(0, ui.KeyUp)
	if a.palette.selectionKey != "json.window.1" {
		t.Fatalf("Up selection = %q", a.palette.selectionKey)
	}
	// It does not land on a group title.
	tt.Key(0, ui.KeyUp)
	if a.palette.selectionKey != "json.window.1" {
		t.Fatalf("selection left the command rows: %q", a.palette.selectionKey)
	}
}

func TestPaletteViewFavoritesStayOpen(t *testing.T) {
	a := paletteTestApp(t)
	tt := ui.NewTester(a.paletteView, 640, 400)
	tt.SetLocale("zh-CN")
	tt.Type("常亮")
	tt.Key(ui.Cmd, ui.KeyK)
	for index, row := range a.palette.rows {
		if row.Key == "palette.favorite" {
			a.palette.list.ScrollTo(index, ui.Start)
		}
	}
	tt.Frame()
	if err := tt.Click("加入收藏"); err != nil {
		t.Fatal(err)
	}
	if !a.isFavorite("awake.start") {
		t.Fatalf("favorites = %v", a.config.PaletteFavorites)
	}
	if a.palette.mode != paletteActionMode {
		t.Fatal("favoriting closed the action list")
	}
	if !tt.HasText("移出收藏") {
		t.Fatalf("missing remove action: %v", tt.Texts())
	}
}

func TestPaletteEmptyState(t *testing.T) {
	a := paletteTestApp(t)
	tt := ui.NewTester(a.paletteView, 640, 400)
	tt.SetLocale("en")
	tt.Type("zzzzzz")
	if !tt.HasText(a.text("en", "PaletteEmpty")) {
		t.Fatalf("missing empty state: %v", tt.Texts())
	}
}

func TestPaletteEnterRunsOnce(t *testing.T) {
	a := paletteTestApp(t)
	launches := 0
	a.awake.launch = func([]string) (awakeChild, error) {
		launches++
		return &fakeAwakeChild{done: make(chan error, 1)}, nil
	}
	tt := ui.NewTester(a.paletteView, 640, 400)
	tt.SetLocale("zh-CN")
	tt.Type("常亮 30m")
	tt.Key(0, ui.KeyEnter)
	if launches != 1 {
		t.Fatalf("Enter ran the command %d times", launches)
	}
}

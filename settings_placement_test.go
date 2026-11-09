package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// placementScreens is the two-display layout these tests draw: a main screen
// with a menu bar and a second one on the left.
func placementScreens() []mygo.Display {
	main := mygo.Display{
		ID: 1, Label: "Built-in", Primary: true,
		Bounds:   mygo.Rectangle{X: 0, Y: 0, Width: 1440, Height: 900},
		WorkArea: mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875},
	}
	left := mygo.Display{
		ID: 2, Label: "Studio Display",
		Bounds:   mygo.Rectangle{X: -1920, Y: 0, Width: 1920, Height: 1080},
		WorkArea: mygo.Rectangle{X: -1920, Y: 25, Width: 1920, Height: 1055},
	}
	return []mygo.Display{main, left}
}

// settingsPlacementApp is a settings app whose view draws the given snapshot.
func settingsPlacementApp(t *testing.T, displays []mygo.Display) *application {
	t.Helper()
	a := awakeTestApp(t)
	_ = evidencePaths(t)
	a.page = settingsGeneral
	// One snapshot serves the window and the view: a display the test cannot
	// see must not be choosable either.
	a.displays = func() []mygo.Display { return displays }
	a.knownDisplays = displays
	return a
}

func TestPlacementRowsFollowTheDisplayCount(t *testing.T) {
	tk := tokens(false, "en")
	displays := placementScreens()
	for _, c := range []struct {
		name          string
		displays      []mygo.Display
		placement     string
		showPlacement bool
		fixed         bool
		content       float32
		natural       int
	}{
		{"one display hides the group", displays[:1], "cursor", false, false, 242, 292},
		{"two displays follow the pointer", displays, "cursor", true, false, 291, 340},
		{"two displays fixed to one", displays, "display", true, true, 340, 388},
	} {
		a := settingsPlacementApp(t, c.displays)
		a.config.WindowPlacement = c.placement
		if c.fixed {
			a.config.WindowDisplayID = 2
			a.config.WindowDisplay = "Studio Display"
			a.config.WindowDisplaySize = "1920x1080"
		}
		l := newSettingsLayout(a, settingsGeneral)
		if l.showPlacement != c.showPlacement || l.fixedDisplay != c.fixed {
			t.Fatalf("%s: layout = %+v", c.name, l)
		}
		if got := pageHeight(tk, settingsGeneral, true, l.showPlacement, l.fixedDisplay, true, false); got != c.content {
			t.Fatalf("%s: content = %v; want %v", c.name, got, c.content)
		}
		if got := settingsNaturalHeight(tk, settingsGeneral, true, l.showPlacement, l.fixedDisplay, true, false); got != c.natural {
			t.Fatalf("%s: natural height = %d; want %d", c.name, got, c.natural)
		}
		// Every state keeps the window floor: a page never shrinks it.
		if got := settingsHeight(tk, settingsGeneral, true, l.showPlacement, l.fixedDisplay, true, false); got != settingsWindowHeight {
			t.Fatalf("%s: window height = %d", c.name, got)
		}
		tt := ui.NewTester(a.view, 720, settingsWindowHeight)
		tt.SetLocale("en")
		tt.Frame()
		if has := tt.HasText(a.text("en", "WindowPlacement")); has != c.showPlacement {
			t.Fatalf("%s: placement row shown = %v", c.name, has)
		}
		if has := tt.HasText(a.text("en", "WindowDisplay")); has && !c.fixed {
			t.Fatalf("%s: the fixed row is drawn in pointer mode", c.name)
		}
		// The whole page fits the minimum window: nothing is clipped.
		if c.fixed {
			if rect, ok := tt.Find("Studio Display"); !ok || rect.Y+rect.H > settingsWindowHeight-24 {
				t.Fatalf("%s: the display row is clipped: %+v %v", c.name, rect, ok)
			}
		}
	}
	// Without the CLI row the card is one row plus its separator shorter, but
	// the window floor stays.
	if got := pageHeight(tk, settingsGeneral, true, true, true, false, false); got != 291 {
		t.Fatalf("no CLI: content = %v; want 291", got)
	}
	if got := settingsHeight(tk, settingsGeneral, true, false, false, false, false); got != settingsWindowHeight {
		t.Fatalf("no CLI, one display: window height = %d", got)
	}
}

func TestPlacementModeSwitchesWithoutMovingTheWindow(t *testing.T) {
	a := settingsPlacementApp(t, placementScreens())
	f := &fakeFrame{bounds: mygo.Rectangle{X: 100, Y: 100, Width: 900, Height: 520}, visible: true}
	a.settingsFrame = func() placementWindow { return f }
	tt := ui.NewTester(a.view, 720, 340)
	tt.SetLocale("en")
	if err := tt.Click("Fixed display"); err != nil {
		t.Fatal(err)
	}
	if a.config.WindowPlacement != "display" {
		t.Fatalf("placement = %q", a.config.WindowPlacement)
	}
	// The first switch adopts the display the window is on.
	if a.config.WindowDisplayID != 1 || a.config.WindowDisplay != "Built-in" || a.config.WindowDisplaySize != "1440x900" {
		t.Fatalf("adopted display = %d %q %q", a.config.WindowDisplayID, a.config.WindowDisplay, a.config.WindowDisplaySize)
	}
	if len(f.moves) != 0 {
		t.Fatalf("changing the mode moved the window: %+v", f.moves)
	}
	// Switching back keeps the choice for a later switch.
	if err := tt.Click("Follow pointer"); err != nil {
		t.Fatal(err)
	}
	if a.config.WindowPlacement != "cursor" || a.config.WindowDisplayID != 1 {
		t.Fatalf("placement = %q, choice = %d", a.config.WindowPlacement, a.config.WindowDisplayID)
	}
	// An existing choice is never silently replaced by a mode switch.
	f.bounds.X = -1500
	tt.Frame()
	if err := tt.Click("Fixed display"); err != nil {
		t.Fatal(err)
	}
	if a.config.WindowDisplayID != 1 {
		t.Fatalf("a mode switch replaced the stored choice: %d", a.config.WindowDisplayID)
	}
}

func TestDisplayChoiceCommitsTheFallback(t *testing.T) {
	a := settingsPlacementApp(t, placementScreens())
	a.config.WindowPlacement = "display"
	// A stored display that is no longer connected falls back to the primary
	// one and says so.
	a.config.WindowDisplayID = 99
	a.config.WindowDisplay = "Gone"
	a.config.WindowDisplaySize = "1280x720"
	tt := ui.NewTester(a.view, 720, settingsWindowHeight)
	tt.SetLocale("en")
	if !tt.HasText(a.text("en", "WindowDisplayMissing")) {
		t.Fatalf("missing the fallback alert: %v", tt.Texts())
	}
	if !tt.HasText("Built-in") {
		t.Fatalf("the fallback display is not shown: %v", tt.Texts())
	}
	// Picking the display that is already shown as the fallback must store it:
	// the value does not change, so no OnChange fires.
	if err := tt.Click("Built-in"); err != nil {
		t.Fatal(err)
	}
	if rect, ok := tt.Find("Studio Display"); !ok {
		t.Fatalf("the menu did not open: %v %v", rect, ok)
	}
	if err := tt.Click("Studio Display"); err != nil {
		t.Fatal(err)
	}
	if a.config.WindowDisplayID != 2 || a.config.WindowDisplay != "Studio Display" || a.config.WindowDisplaySize != "1920x1080" {
		t.Fatalf("chosen display = %d %q %q", a.config.WindowDisplayID, a.config.WindowDisplay, a.config.WindowDisplaySize)
	}
	// The alert goes away once the stored choice is connected again.
	tt.Frame()
	if tt.HasText(a.text("en", "WindowDisplayMissing")) {
		t.Fatal("the alert stayed after a valid choice")
	}
	// Choosing a display does not move the open window.
	if a.settingsPlace.pending != nil {
		t.Fatal("a choice wrote a pending seat")
	}
}

func TestDisplayMenuKeyboardAndScrolling(t *testing.T) {
	displays := placementScreens()
	for index := 0; index < 6; index++ {
		displays = append(displays, mygo.Display{
			ID: int64(10 + index), Label: "External " + string(rune('A'+index)), Primary: false,
			Bounds:   mygo.Rectangle{X: 1440 + index*100, Y: 0, Width: 1280, Height: 800},
			WorkArea: mygo.Rectangle{X: 1440 + index*100, Y: 25, Width: 1280, Height: 775},
		})
	}
	a := settingsPlacementApp(t, displays)
	a.config.WindowPlacement = "display"
	a.config.WindowDisplayID = 1
	a.config.WindowDisplay = "Built-in"
	a.config.WindowDisplaySize = "1440x900"
	tt := ui.NewTester(a.view, 720, settingsWindowHeight)
	tt.SetLocale("en")
	// Opening the menu lays the options out in a bounded, scrolling list.
	if err := tt.Click("Built-in"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("External A"); !ok {
		t.Fatalf("the menu did not open: %v", tt.Texts())
	}
	// The popup shows at most six rows: the seventh option is laid out beyond
	// the scrolling viewport rather than drawn, so the list never grows past
	// the window.
	if last, ok := tt.Find("External F"); !ok || last.H != 0 {
		t.Fatalf("the list is not bounded to six rows: %+v %v", last, ok)
	}
	if fourth, ok := tt.Find("External C"); !ok || fourth.H == 0 {
		t.Fatalf("the fourth option is not drawn: %+v %v", fourth, ok)
	}
	tt.Key(0, ui.KeyEnd)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.config.WindowDisplayID != 15 || a.config.WindowDisplay != "External F" {
		t.Fatalf("keyboard choice = %d %q", a.config.WindowDisplayID, a.config.WindowDisplay)
	}
	// Escape closes the menu without choosing anything.
	tt.Frame()
	if err := tt.Click("External F"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	before := a.config.WindowDisplayID
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.config.WindowDisplayID != before {
		t.Fatalf("escape changed the choice to %d", a.config.WindowDisplayID)
	}
}

func TestDisplayChoicesLabels(t *testing.T) {
	displays := []mygo.Display{
		{ID: 1, Label: "", Bounds: mygo.Rectangle{Width: 1440, Height: 900}},
		{ID: 2, Label: "Studio", Bounds: mygo.Rectangle{Width: 1920, Height: 1080}},
		{ID: 3, Label: "Studio", Bounds: mygo.Rectangle{Width: 2560, Height: 1440}},
		{ID: 4, Label: "Studio", Bounds: mygo.Rectangle{Width: 2560, Height: 1440}},
	}
	choices := displayChoices(displays)
	want := []struct{ value, label string }{
		{"1", "1440x900"},
		{"2", "Studio · 1920x1080"},
		{"3", "Studio · 2560x1440"},
		{"4", "Studio · 2560x1440 (2)"},
	}
	if len(choices) != len(want) {
		t.Fatalf("choices = %+v", choices)
	}
	for index, c := range want {
		if choices[index].Value != c.value || choices[index].Label != c.label {
			t.Errorf("choice %d = %+v; want %+v", index, choices[index], c)
		}
		if choices[index].Title != choices[index].Label {
			t.Errorf("choice %d tooltip = %q", index, choices[index].Title)
		}
	}
}

func TestPlacementInfoIsAccessible(t *testing.T) {
	a := settingsPlacementApp(t, placementScreens())
	a.config.WindowPlacement = "display"
	a.config.WindowDisplayID = 99
	a.config.WindowDisplay = "Gone"
	tt := ui.NewTester(a.view, 720, settingsWindowHeight)
	tt.SetLocale("zh-CN")
	tt.Frame()
	info := a.text("zh-CN", "WindowPlacementInfo")
	rect, ok := tt.Find(info)
	if !ok || rect.W > 16.5 || rect.H > 16.5 {
		t.Fatalf("the info anchor = %+v, %v", rect, ok)
	}
	// The alert is a focusable anchor with the whole sentence.
	missing := a.text("zh-CN", "WindowDisplayMissing")
	if !hasAccessibleText(tt, missing) {
		t.Fatal("the fallback alert has no accessible text")
	}
	// Both languages carry all six strings.
	for _, locale := range []string{"zh-CN", "en"} {
		for _, key := range []string{"WindowPlacement", "WindowPlacementCursor", "WindowPlacementDisplay", "WindowDisplay", "WindowPlacementInfo", "WindowDisplayMissing"} {
			if strings.TrimSpace(a.text(locale, key)) == "" || a.text(locale, key) == key {
				t.Fatalf("%s: missing %q", locale, key)
			}
		}
	}
}

func TestWindowPlacementConfigCompatibility(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	load := func(body string) (Config, error) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return loadConfig(path)
	}
	// A config written before this feature loads with the new defaults and is
	// left on disk untouched: loading never writes.
	old := "version = 1\nlanguage = 'zh-CN'\nwindow_width = 900\nwindow_height = 520\n"
	config, err := load(old)
	if err != nil {
		t.Fatal(err)
	}
	if config.WindowPlacement != "cursor" || config.WindowDisplayID != 0 || config.WindowPositions != nil || config.JSONPositions != nil {
		t.Fatalf("old config = %+v", config)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != old {
		t.Fatalf("loading rewrote the file: %q %v", body, err)
	}
	// An unknown or empty mode falls back to following the pointer, without
	// an error and without an immediate save.
	for _, body := range []string{
		"version = 1\nlanguage = 'zh-CN'\nwindow_placement = 'sideways'\n",
		"version = 1\nlanguage = 'zh-CN'\nwindow_placement = ''\n",
	} {
		config, err := load(body)
		if err != nil || config.WindowPlacement != "cursor" {
			t.Fatalf("mode %q loaded as %q, %v", body, config.WindowPlacement, err)
		}
		if written, _ := os.ReadFile(path); string(written) != body {
			t.Fatalf("loading a mode rewrote the file: %q", written)
		}
	}
	// A fixed mode with a missing choice or a malformed size still loads; the
	// window falls back to the primary display at open.
	config, err = load("version = 1\nlanguage = 'zh-CN'\nwindow_placement = 'display'\nwindow_display = ''\nwindow_display_size = 'wide'\n")
	if err != nil || config.WindowPlacement != "display" || config.WindowDisplay != "" {
		t.Fatalf("fixed mode without a choice = %+v, %v", config, err)
	}
	if display, matched := fixedDisplay(placementScreens(), config); matched || display.ID != 1 {
		t.Fatalf("missing choice resolved to %d, %v", display.ID, matched)
	}
	// A type error is still an error.
	if _, err := load("version = 1\nlanguage = 'zh-CN'\nwindow_placement = 7\n"); err == nil {
		t.Fatal("a wrong type loaded without an error")
	}
	// The six fields round trip through the file with their own tags.
	config = defaultConfig()
	config.WindowPlacement = "display"
	config.WindowDisplayID = 732561
	config.WindowDisplay = "外接显示器一号"
	config.WindowDisplaySize = "1920x1080"
	config.WindowPositions = map[string]seat{"732561": {X: 120, Y: 96}}
	config.JSONPositions = map[string]seat{"732561": {X: 300, Y: 180}}
	config.WindowWidth, config.WindowHeight = 900, 520
	if err := saveConfig(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != 1 || loaded.WindowPlacement != "display" || loaded.WindowDisplayID != 732561 ||
		loaded.WindowDisplay != "外接显示器一号" || loaded.WindowDisplaySize != "1920x1080" ||
		loaded.WindowPositions["732561"] != (seat{X: 120, Y: 96}) || loaded.JSONPositions["732561"] != (seat{X: 300, Y: 180}) {
		t.Fatalf("round trip = %+v", loaded)
	}
	// An old height is raised to the 008 floor; a larger one is kept.
	if width, height := settingsSize(720, 368); width != 720 || height != settingsWindowHeight {
		t.Fatalf("settingsSize(720, 368) = %dx%d", width, height)
	}
	if width, height := settingsSize(900, 520); width != 900 || height != 520 {
		t.Fatalf("settingsSize(900, 520) = %dx%d", width, height)
	}
	if width, height := settingsSize(600, 200); width != 720 || height != settingsWindowHeight {
		t.Fatalf("settingsSize(600, 200) = %dx%d", width, height)
	}
}

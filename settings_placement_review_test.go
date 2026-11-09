package main

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestDisplayPopupClosesWhenTopologyChanges(t *testing.T) {
	a := settingsPlacementApp(t, placementScreens())
	a.config.WindowPlacement, a.config.WindowDisplayID = "display", 1
	tt := ui.NewTester(a.view, 720, settingsWindowHeight)
	tt.SetLocale("en")
	if err := tt.Click("Built-in"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Studio Display") {
		t.Fatal("popup did not open")
	}
	// Both screens remain, but a changed work area still invalidates the menu.
	next := placementScreens()
	next[0].WorkArea.Y += 10
	a.displays = func() []mygo.Display { return next }
	a.displaysChanged()
	tt.Frame()
	if tt.HasText("Studio Display") {
		t.Fatal("old popup survived a changed topology")
	}
	if a.config.WindowDisplayID != 1 {
		t.Fatal("topology refresh selected a display")
	}
	if err := tt.Click("Built-in"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Studio Display") {
		t.Fatal("fresh popup did not open")
	}
	// Disappearing after drawing must also fail the final ID validation.
	a.displays = func() []mygo.Display { return next[:1] }
	a.commitWindowDisplayID("2")
	if a.config.WindowDisplayID != 1 {
		t.Fatal("stale display ID was committed")
	}
	a.displaysChanged()
	tt.Frame()
	if tt.HasText("Studio Display") || tt.HasText("Window position") {
		t.Fatal("single-display topology retained the controls or popup")
	}
}

func TestFallbackDisplayExplicitSelection(t *testing.T) {
	for _, key := range []ui.Key{0, ui.KeyEnter, ui.KeySpace} {
		t.Run(map[ui.Key]string{0: "click", ui.KeyEnter: "enter", ui.KeySpace: "space"}[key], func(t *testing.T) {
			a := settingsPlacementApp(t, placementScreens())
			a.config.WindowPlacement = "display"
			a.config.WindowDisplayID, a.config.WindowDisplay = 99, "Gone"
			tt := ui.NewTester(a.view, 720, settingsWindowHeight)
			tt.SetLocale("en")
			if err := tt.Click("Built-in"); err != nil {
				t.Fatal(err)
			}
			if key == 0 {
				// The trigger and the option share the same text; use its actual
				// row rectangle after opening rather than clicking the trigger.
				rect, ok := tt.Find("Studio Display")
				if !ok {
					t.Fatal("popup did not open")
				}
				tt.ClickAt(rect.X+rect.W/2, rect.Y+rect.H/2-tokens(false, "en").SizeRowHeight)
				tt.Frame()
			} else {
				tt.Key(0, key)
				tt.Frame()
			}
			if a.config.WindowDisplayID != 1 || a.config.WindowDisplay != "Built-in" || a.config.WindowDisplaySize != "1440x900" {
				t.Fatalf("explicit fallback choice not committed: %+v", a.config)
			}
			if tt.HasText(a.text("en", "WindowDisplayMissing")) {
				t.Fatal("fallback alert survived an explicit choice")
			}
		})
	}
}

func TestLegacyPaletteSeatsSurvivePlacementAndNewConfigSave(t *testing.T) {
	a := paletteTestApp(t)
	f := paletteTestScreens(a)
	legacy := "version=1\nlanguage='zh-CN'\n[palette_positions.1]\nx=40\ny=96\n[palette_positions.'-42']\nx=-8\ny=-16\n[palette_positions.abc]\nx=5\ny=6\n"
	if err := os.WriteFile(a.path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig(a.path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]seat{"1": {X: 40, Y: 96}, "-42": {X: -8, Y: -16}, "abc": {X: 5, Y: 6}}
	if !reflect.DeepEqual(config.PalettePositions, want) {
		t.Fatalf("legacy seats dropped: %+v", config.PalettePositions)
	}
	if body, err := os.ReadFile(a.path); err != nil || string(body) != legacy {
		t.Fatal("load modified the legacy file")
	}
	a.config = config
	a.placePalette()
	if f.window != (screenPoint{X: 40 - a.paletteMargin(), Y: 121 - a.paletteMargin()}) {
		t.Fatalf("legacy panel coordinate or shadow margin changed: %+v", f.window)
	}
	// Saving the new settings must leave all legacy keys and signed offsets.
	a.changeWindowPlacement("display")
	loaded, err := loadConfig(a.path)
	if err != nil || !reflect.DeepEqual(loaded.PalettePositions, want) {
		t.Fatalf("new config save lost legacy positions: %+v, %v", loaded.PalettePositions, err)
	}
}

func TestPlacementKeyboardReachesFullDescriptions(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		a := settingsPlacementApp(t, placementScreens())
		a.knownDisplays[1].Label = strings.Repeat("External Display ", 8)
		a.config.WindowPlacement, a.config.WindowDisplayID = "display", 2
		tt := ui.NewTester(a.view, 720, settingsWindowHeight)
		tt.SetLocale(locale)
		info := a.text(locale, "WindowPlacementInfo")
		fullChoice := a.text(locale, "WindowDisplay") + ": " + strings.TrimSpace(a.knownDisplays[1].Label)
		seenInfo, seenChoice := false, false
		for i := 0; i < 30; i++ {
			tt.Key(0, ui.KeyTab)
			seenInfo = seenInfo || tt.Focused(info)
			seenChoice = seenChoice || tt.Focused(fullChoice)
		}
		if !seenInfo || !seenChoice {
			t.Fatalf("%s: keyboard cannot reach full info/name: info=%v choice=%v", locale, seenInfo, seenChoice)
		}
	}
}

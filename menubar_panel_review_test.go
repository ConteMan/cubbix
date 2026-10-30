package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestPanelURLSessionOverridesMemory(t *testing.T) {
	for _, minutes := range []int{0, 30, 45, 60, 120, 240} {
		t.Run(fmt.Sprint(minutes), func(t *testing.T) {
			a := panelTestApp(t)
			a.config.AwakeMinutes, a.config.AwakeUntil = 60, "18:30"
			a.showMenubarPanel()
			if err := a.runURL(paletteURLRequest{Kind: "awake", HasMinutes: true, Minutes: minutes}); err != nil {
				t.Fatal(err)
			}
			preset, ok := a.panelPreset(time.Now())
			want := minutes == 0 || minutes == 30 || minutes == 60 || minutes == 120
			if ok != want || ok && preset != minutes {
				t.Fatalf("preset=%d/%v, requested=%d", preset, ok, minutes)
			}
			if a.config.AwakeMinutes != 60 || a.config.AwakeUntil != "18:30" {
				t.Fatal("URL rewrote memory")
			}
			if status := a.menubarPanelStatusLine("en", time.Now()); strings.Contains(status, "18:30") {
				t.Fatalf("stale deadline: %s", status)
			}
			before := a.awake.snapshot().Session.End
			a.panelSetDuration(60)
			if minutes != 60 && (a.config.AwakeUntil != "" || a.config.AwakeMinutes != 60) {
				t.Fatal("pill did not replace deadline memory")
			}
			if minutes != 60 && a.awake.snapshot().Session.End.Equal(before) {
				t.Fatal("a different running duration suppressed the pill action")
			}
			if p, ok := a.panelPreset(time.Now().Add(40 * time.Minute)); !ok || p != 60 {
				t.Fatalf("elapsed time changed preset: %d/%v", p, ok)
			}
		})
	}
}

func TestPanelErrorClearsAfterExternalRecovery(t *testing.T) {
	a := panelTestApp(t)
	a.showMenubarPanel()
	a.awake.launch = func([]string) (awakeChild, error) { return nil, errors.New("launch failed") }
	tt := panelView(a, "en")
	if err := tt.Click("1h"); err != nil {
		t.Fatal(err)
	}
	if a.menubarPanelErrorKey() != "AwakeStartError" || !a.menubarPanelVisible() || a.settingsError != "" {
		t.Fatal("failed action did not stay in the card")
	}
	tt.Frame()
	if _, ok := tt.Find(a.text("en", "AwakeStartError")); !ok {
		t.Fatal("missing accessible error icon")
	}
	a.awake.launch = func([]string) (awakeChild, error) { return &fakeAwakeChild{done: make(chan error, 1)}, nil }
	if err := a.runURL(paletteURLRequest{Kind: "awake", HasMinutes: true, Minutes: 30}); err != nil {
		t.Fatal(err)
	}
	if a.menubarPanelErrorKey() != "" || !a.menubarPanelRows().status {
		t.Fatal("external successful action left a stale error")
	}
}

func TestPanelKeyboardNavigation(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a := panelTestApp(t)
			a.config.AwakeMinutes = 60
			tt := panelView(a, locale)
			tt.Key(0, ui.KeyTab)
			if !tt.Focused(a.text(locale, "R05Start")) {
				t.Fatal("first stop is not the switch")
			}
			tt.Key(0, ui.KeyTab)
			if !tt.Focused(a.text(locale, "R05Screen")) {
				t.Fatal("second stop is not the mode")
			}
			tt.Key(0, ui.KeyRight)
			if a.config.AwakeMode != "tasks" || a.awake.snapshot().Active {
				t.Fatal("mode arrows must change memory without starting")
			}
			tt.Key(0, ui.KeyTab)
			if !tt.Focused("1h") {
				t.Fatal("pill group did not enter at remembered selection")
			}
			tt.Key(0, ui.KeyLeft)
			if !tt.Focused("30m") || a.awake.snapshot().Active {
				t.Fatal("pill arrow executed an action")
			}
			tt.Key(0, ui.KeyLeft)
			if !tt.Focused("∞") {
				t.Fatal("pill arrows did not wrap")
			}
			tt.Key(0, ui.KeyEnter)
			if s := a.awake.snapshot(); !s.Active || !s.Session.End.IsZero() || s.Session.Mode != "tasks" {
				t.Fatal("Enter did not execute the focused pill")
			}
			tt.Key(0, ui.KeyRight)
			tt.Key(0, ui.KeySpace)
			if s := a.awake.snapshot(); !s.Active || s.Session.Minutes != 30 {
				t.Fatal("Space did not execute the focused pill")
			}
			tt.Key(0, ui.KeyTab)
			if !tt.Focused(a.text(locale, "JSONShortcutLabel")) {
				t.Fatal("Tab stayed within the pill group")
			}
			tt.Key(ui.Shift, ui.KeyTab)
			if !tt.Focused("30m") {
				t.Fatal("reverse Tab lost the last focused pill")
			}
		})
	}
}

func TestPanelJSONButtonsAndWindowTarget(t *testing.T) {
	for _, label := range []string{"panel-json-new", "panel-json-button"} {
		t.Run(label, func(t *testing.T) {
			a := panelTestApp(t)
			a.config.AwakeEnabled = false
			a.showMenubarPanel()
			var created, shown *jsonDocument
			a.jsonReadClipboard = func() string {
				if a.menubarPanelVisible() {
					t.Fatal("clipboard read before closing panel")
				}
				return `{"sample":true}`
			}
			a.createJSONWindow = func(d *jsonDocument) { created = d }
			a.presentJSONWindow = func(d *jsonDocument) {
				if a.menubarPanelVisible() {
					t.Fatal("window shown before closing panel")
				}
				shown = d
			}
			tt := panelView(a, "en")
			tt.Key(0, ui.KeyTab)
			if label == "panel-json-button" {
				tt.Key(0, ui.KeyTab)
			}
			if !tt.Focused(a.text("en", "JSONShortcutLabel")) {
				t.Fatal("missing focused new-window button")
			}
			tt.Key(0, ui.KeyEnter)
			if created == nil || shown != created || created.text != `{"sample":true}` {
				t.Fatalf("new-window chain: created=%v shown=%v", created, shown)
			}
			other := a.addJSONDocument("", "other.json", true)
			a.showMenubarPanel()
			tt = panelView(a, "en")
			if err := tt.Click("other.json"); err != nil {
				t.Fatal(err)
			}
			if shown != other || a.menubarPanelVisible() {
				t.Fatal("window row did not dispatch its own document")
			}
		})
	}
}

func TestPanelSessionMetadataSurvivesRestore(t *testing.T) {
	a := panelTestApp(t)
	if err := a.setDuration(120); err != nil {
		t.Fatal(err)
	}
	saved, err := loadConfig(a.path)
	if err != nil || saved.AwakeSession == nil || saved.AwakeSession.Minutes != 120 {
		t.Fatalf("duration metadata did not round trip: %+v %v", saved.AwakeSession, err)
	}
	if err := a.runAwakeStart(request{Awake: awakeArg{Kind: awakeArgUntil, Until: "18:30"}}); err != nil {
		t.Fatal(err)
	}
	saved, err = loadConfig(a.path)
	if err != nil || saved.AwakeSession == nil || saved.AwakeSession.Until != "18:30" {
		t.Fatalf("deadline metadata did not round trip: %+v %v", saved.AwakeSession, err)
	}
	// Legacy sessions still run, but unknown duration must never claim a pill.
	legacy := awakeSession{Mode: "screen", End: time.Now().Add(30 * time.Minute)}
	if err := a.startAwake(legacy); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.panelPreset(time.Now()); ok {
		t.Fatal("legacy session guessed a preset from defaults")
	}
}

func TestPanelPresetGeometryAndShortcut(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			a := panelTestApp(t)
			a.config.PaletteShortcut = "Ctrl+Alt+P"
			tt := panelView(a, locale)
			tt.SetDark(dark)
			var previous ui.Rect
			for i, label := range []string{"30m", "1h", "2h", "∞"} {
				rect, ok := tt.Find(label)
				if !ok || rect.W < 60 || rect.X < 0 || rect.X+rect.W > 320 {
					t.Fatalf("%s dark=%v %s bounds=%+v", locale, dark, label, rect)
				}
				if i > 0 && (rect.W != previous.W || rect.X < previous.X+previous.W) {
					t.Fatalf("unequal/overlapping pills: %+v %+v", previous, rect)
				}
				previous = rect
			}
			tooltip := a.menubarPanelTooltip(locale, "PaletteShortcutLabel", a.config.PaletteShortcut)
			if strings.Contains(tooltip, "⌥.") {
				t.Fatal("tooltip kept default shortcut")
			}
			if err := tt.Click(tooltip); err != nil {
				t.Fatal(err)
			}
			if a.menubarPanelVisible() || !a.paletteVisible() {
				t.Fatal("custom-shortcut footer failed")
			}
		}
	}
}

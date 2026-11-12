package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestDevtoolsScreenshots renders the palette results and the settings page in
// both languages and both themes. It is an in-memory render of the real views:
// no window is created, nothing is activated, no shortcut is registered and no
// clipboard is read. CUBBIX_DEVTOOLS_EVIDENCE names the output directory,
// which stays outside the repository.
func TestDevtoolsScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_DEVTOOLS_EVIDENCE")
	if dir == "" {
		t.Skip("devtools evidence not requested")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	a := awakeTestApp(t)
	capture := func(name, locale string, dark bool, prepare func(p *paletteState)) {
		t.Helper()
		a.palette = &paletteState{}
		if prepare != nil {
			prepare(a.palette)
		}
		tt := ui.NewTester(a.paletteView, paletteWindowWidth(), paletteWindowHeight())
		tt.SetScale(2)
		tt.SetLocale(locale)
		tt.SetDark(dark)
		tt.Frame()
		// Let a theme or navigation transition settle before drawing.
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
		mode := "light"
		if dark {
			mode = "dark"
		}
		file, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%s-%s.png", name, locale, mode)))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, tt.Image()); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	states := []struct {
		name  string
		query string
	}{
		{"palette-timestamp", "1700000000"},
		{"palette-date", "2026-11-12"},
		{"palette-now", "now"},
		{"palette-uuid", "uuid"},
		{"palette-uuid7-batch", "uuid7 8"},
		{"palette-uuid-literal", "550e8400-e29b-41d4-a716-446655440000"},
		{"palette-error", "ts 946684800000"},
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			for _, state := range states {
				state := state
				capture(state.name, locale, dark, func(p *paletteState) { p.query = state.query })
			}
			capture("palette-clipboard-suggestion", locale, dark, func(p *paletteState) {
				p.suggestion = devtoolsSuggestion("1700000000")
			})
		}
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			a := awakeTestApp(t)
			if locale == "en" {
				a.language = 2
			} else {
				a.language = 1
			}
			a.page = settingsDevtools
			l := newSettingsLayout(a, a.page)
			height := settingsHeight(tokens(dark, locale), a.page, l.awakeOn, l.showPlacement, l.fixedDisplay, l.hasCLI, l.ready)
			tt := ui.NewTester(a.view, 720, height)
			tt.SetScale(2)
			tt.SetLocale(locale)
			tt.SetDark(dark)
			tt.Frame()
			tt.Advance(300 * time.Millisecond)
			tt.Frame()
			mode := "light"
			if dark {
				mode = "dark"
			}
			file, err := os.Create(filepath.Join(dir, fmt.Sprintf("settings-devtools-%s-%s.png", locale, mode)))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(file, tt.Image()); err != nil {
				file.Close()
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

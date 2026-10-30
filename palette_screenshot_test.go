package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// These are in-memory renders of the real palette view. No native window is
// created and no screen is touched.
func TestPaletteScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_PALETTE_UI_EVIDENCE")
	if dir == "" {
		t.Skip("palette UI evidence not requested")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	a := awakeTestApp(t)
	a.config.PaletteFavorites = []string{"awake.stop", "json.new"}
	a.config.PaletteRecents = []string{"awake.mode.tasks", "settings.page.shortcuts"}
	a.addJSONDocument("", "", true)
	a.addJSONDocument("", "", true)
	if err := a.awake.start(awakeSession{Mode: "screen"}); err != nil {
		t.Fatal(err)
	}
	capture := func(name, locale string, dark bool, view func(*ui.Context), width, height int, setup func(*ui.Tester)) {
		t.Helper()
		a.palette = &paletteState{}
		tt := ui.NewTester(view, width, height)
		tt.SetScale(2)
		tt.SetLocale(locale)
		tt.SetDark(dark)
		if setup != nil {
			setup(tt)
		}
		tt.Frame()
		file, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			mode := "light"
			if dark {
				mode = "dark"
			}
			suffix := "-" + locale + "-" + mode
			capture("palette-empty"+suffix, locale, dark, a.paletteView, 640, 400, func(tt *ui.Tester) {})
			capture("palette-search"+suffix, locale, dark, a.paletteView, 640, 400, func(tt *ui.Tester) {
				if locale == "en" {
					tt.Type("awake")
				} else {
					tt.Type("常亮")
				}
			})
			capture("palette-params"+suffix, locale, dark, a.paletteView, 640, 400, func(tt *ui.Tester) {
				if locale == "en" {
					tt.Type("Start keep awake 45m")
				} else {
					tt.Type("常亮 45m")
				}
			})
			capture("palette-invalid"+suffix, locale, dark, a.paletteView, 640, 400, func(tt *ui.Tester) {
				if locale == "en" {
					tt.Type("Start keep awake 45x")
				} else {
					tt.Type("常亮 45x")
				}
			})
			capture("palette-actions"+suffix, locale, dark, a.paletteView, 640, 400, func(tt *ui.Tester) {
				if locale == "en" {
					tt.Type("awake")
				} else {
					tt.Type("常亮")
				}
				tt.Key(ui.Cmd, ui.KeyK)
			})
			a.page = 3
			capture("shortcuts"+suffix, locale, dark, a.view, 720, 340, nil)
			a.page = 0
		}
	}
}

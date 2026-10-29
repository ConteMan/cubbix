package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// These are in-memory renders of the real view. No native window is created.
func TestSettingsScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_UI_EVIDENCE")
	if dir == "" {
		t.Skip("UI screenshots not requested")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	a, err := newApplication(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.updates.close()
	capture := func(name, locale string, dark bool, w, h int) {
		t.Helper()
		tt := ui.NewTester(a.view, w, h)
		tt.SetScale(2)
		tt.SetLocale(locale)
		tt.SetDark(dark)
		f, err := os.Create(filepath.Join(dir, "ui-"+name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	for _, locale := range []string{"zh-CN", "en"} {
		a.page, a.notesPage = 1, true
		for _, dark := range []bool{false, true} {
			mode := "light"
			if dark {
				mode = "dark"
			}
			capture("notes-"+locale+"-"+mode, locale, dark, 720, 560)
		}
	}
	a.page, a.notesPage = 1, false
	a.updates.state = updateState{Phase: updateReady, Version: "0.0.4"}
	capture("settings-enlarged-zh-CN-light", "zh-CN", false, 1000, 600)
}

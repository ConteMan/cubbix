package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestTranslateScreenshots renders the floating panel and its settings page
// in both languages and both themes. It is an in-memory ui.Tester render: no
// window is created, nothing is activated and no shortcut is registered.
// CUBBIX_TRANSLATE_EVIDENCE names an output directory outside the repository;
// without it the test does nothing.
func TestTranslateScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_TRANSLATE_EVIDENCE")
	if dir == "" {
		t.Skip("translation evidence not requested")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	save := func(name string, img *image.RGBA) {
		t.Helper()
		file, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, img); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			mode := "light"
			if dark {
				mode = "dark"
			}
			suffix := locale + "-" + mode
			// Settings page with a saved key and a fetched model list.
			a := translateTestApp(t)
			a.page = settingsTranslate
			a.translateKeySaved = true
			a.translateModelList = []string{"group/one", "provider/two"}
			tt := ui.NewTester(a.view, 720, settingsWindowHeight)
			tt.SetScale(2)
			tt.SetLocale(locale)
			tt.SetDark(dark)
			tt.Frame()
			save(fmt.Sprintf("translate-settings-%s", suffix), tt.Image())
			// The floating panel in its finished state.
			b := translateTestApp(t)
			s := b.ensureTranslateState()
			s.input = "Hello, world"
			s.output = "你好，世界"
			if locale == "en" {
				s.output = "Hello, world — translated"
			}
			s.phase = translateDone
			tk := tokens(dark, locale)
			width := int(tk.SizeTranslateWidth) + 2*int(tk.SizePaletteShadowMargin)
			height := int(b.translateHeight(tk, s)) + 2*int(tk.SizePaletteShadowMargin)
			panel := ui.NewTester(b.translateView, width, height)
			panel.SetScale(2)
			panel.SetLocale(locale)
			panel.SetDark(dark)
			panel.Frame()
			save(fmt.Sprintf("translate-panel-%s", suffix), panel.Image())
		}
	}
}

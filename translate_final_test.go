package main

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// finalPixel returns one DIP of the last settings frame. The tester renders
// at 2x, so the physical pixel is looked up through the image's scale.
func finalPixel(tt *ui.Tester, width int, x, y float32) color.RGBA {
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / float32(width)
	r, g, b, a := img.At(int(x*scale), int(y*scale)).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

// finalColorRun is the longest vertical run of one colour in a column. A
// scroll thumb is at least 24 DIPs tall; text glyphs and hairlines are not,
// so this separates the bar from the same colour used by secondary text.
func finalColorRun(tt *ui.Tester, width int, x float32, want color.RGBA) float32 {
	var best, run float32
	for y := float32(0); y < float32(settingsWindowHeight); y += 0.5 {
		if finalPixel(tt, width, x, y) == want {
			run += 0.5
		} else {
			best = max(best, run)
			run = 0
		}
	}
	return max(best, run)
}

// scrolledTranslateSettings frames the real settings view, hovers the scroll
// container and scrolls it. Waiting past the theme transition keeps the dark
// captures fully dark before any pixel is inspected.
func scrolledTranslateSettings(a *application, locale string, dark bool, widths ...int) *ui.Tester {
	width := 720
	if len(widths) > 0 {
		width = widths[0]
	}
	tt := ui.NewTester(a.view, width, settingsWindowHeight)
	tt.SetScale(2)
	tt.SetLocale(locale)
	tt.SetDark(dark)
	tt.Frame()
	tt.Advance(700 * time.Millisecond)
	tt.Frame()
	tk := tokens(dark, locale)
	scrollX := (float32(width) + tk.SizeSidebarWidth) / 2
	tt.Move(scrollX, 350)
	tt.Scroll(scrollX, 350, 0, 1000)
	tt.Frame()
	tt.Advance(700 * time.Millisecond)
	tt.Frame()
	return tt
}

// The floating scroll bar sits in the window's right padding, past the
// card's border, instead of covering it while the page scrolls.
func TestTranslateSettingsScrollbarOutsideCard(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			for _, width := range []int{720, 840, 1024} {
				a := translateTestApp(t)
				a.page, a.translateKeySaved = settingsTranslate, true
				tk := tokens(dark, locale)
				tt := scrolledTranslateSettings(a, locale, dark, width)
				barColor := ui.Hex(tk.ColorTextSecondary)
				want := color.RGBA{barColor.R, barColor.G, barColor.B, 255}
				contentWidth := min(float32(width)-tk.SizeSidebarWidth-2*tk.Space24, tk.SizeContentMaxWidth)
				containerRight := (float32(width) + tk.SizeSidebarWidth + contentWidth) / 2
				cardRight := containerRight - translateScrollbarGutter(tk)
				barLeft := containerRight - tk.SizeScrollbar - 3
				if run := finalColorRun(tt, width, barLeft+tk.SizeScrollbar/2, want); run < 24 {
					t.Fatalf("%s dark=%v width=%d: scroll bar not visible in the gutter (run %.1f)", locale, dark, width, run)
				}
				for _, x := range []float32{cardRight - 1, cardRight + 1, barLeft - 1} {
					if run := finalColorRun(tt, width, x, want); run >= 24 {
						t.Fatalf("%s dark=%v: scroll bar covers the card at x=%.1f (run %.1f)", locale, dark, x, run)
					}
				}
				if barLeft+tk.SizeScrollbar > containerRight {
					t.Fatal("the scroll bar left its container")
				}
				// The gutter keeps the bar inside the scroll container, so it is
				// still draggable rather than becoming a decorative overlay.
				barX := barLeft + tk.SizeScrollbar/2
				label := a.text(locale, "TranslateRecordShortcut")
				before, ok := tt.Find(label)
				if !ok {
					t.Fatal("shortcut row missing before dragging")
				}
				tt.Move(barX, 300)
				tt.Frame()
				tt.Press(barX, 300)
				tt.Move(barX, 200)
				tt.Move(barX, 100)
				tt.Move(barX, 40)
				tt.Release(barX, 40)
				tt.Frame()
				after, ok := tt.Find(label)
				if !ok || after.Y <= before.Y {
					t.Fatalf("%s dark=%v: the scroll bar cannot be dragged: before=%+v after=%+v", locale, dark, before, after)
				}
			}
		}
	}
}

// The bare command opens an empty floating panel, and the module switch hides
// the command together with the card.
func TestTranslatePaletteBareEntry(t *testing.T) {
	a := translateTestApp(t)
	var entry *command
	for _, cmd := range a.commands() {
		if cmd.ID == "translate.open" {
			entry = cmd
		}
	}
	if entry == nil || entry.Param != paramText || entry.Icon != "languages" {
		t.Fatalf("translate command = %+v", entry)
	}
	if err := entry.Run(a, request{Command: entry, Arg: ""}); err != nil {
		t.Fatal(err)
	}
	if !a.translateVisible() || strings.TrimSpace(a.translate.input) != "" {
		t.Fatal("the bare command did not open an empty floating panel")
	}
	a.changeTranslateEnabled(false)
	for _, cmd := range a.commands() {
		if cmd.ID == "translate.open" {
			t.Fatal("a disabled module still offers the translate command")
		}
	}
}

// One single-row card: no status, no recent translation, hidden with the
// module. Its height is in the panel's measurement as well as its view.
func TestMenubarTranslateCard(t *testing.T) {
	a := panelTestApp(t)
	tk := tokens(false, "en")
	if !a.menubarPanelRows().translate {
		t.Fatal("the enabled module has no card row")
	}
	with := a.menubarPanelHeight(tk)
	a.config.TranslateEnabled = false
	if a.menubarPanelRows().translate {
		t.Fatal("a disabled module still has a card row")
	}
	without := a.menubarPanelHeight(tk)
	if with-without != int(translateCardHeight(tk)+tk.Space8) {
		t.Fatalf("card height = %d, want %d", with-without, int(translateCardHeight(tk)+tk.Space8))
	}
	a.config.TranslateEnabled = true
	a.showMenubarPanel()
	tt := ui.NewTester(a.menubarPanelView, int(tk.SizeMenubarPanelWidth), a.menubarPanelHeight(tk))
	tt.SetLocale("en")
	tt.Frame()
	if _, ok := tt.Find(a.text("en", "Translate")); !ok {
		t.Fatalf("card title missing: %v", tt.Texts())
	}
	if _, ok := tt.Find(a.text("en", "TranslateCardAction")); !ok {
		t.Fatalf("card action missing: %v", tt.Texts())
	}
	if err := tt.Click(a.text("en", "TranslateCardAction")); err != nil {
		t.Fatal(err)
	}
	if a.menubarPanelVisible() || !a.translateVisible() {
		t.Fatal("the card did not open the floating panel")
	}
	a.config.TranslateEnabled = false
	a.showMenubarPanel()
	tt = ui.NewTester(a.menubarPanelView, int(tk.SizeMenubarPanelWidth), a.menubarPanelHeight(tk))
	tt.SetLocale("en")
	tt.Frame()
	if tt.HasText(a.text("en", "TranslateCardAction")) {
		t.Fatal("a disabled module still draws the card")
	}
}

// TestTranslateFinalScreenshots writes the step 5/6 evidence outside the
// repository. It is an in-memory ui.Tester render: no window, no voice, no
// shortcut, no keychain, no network and no sound.
func TestTranslateFinalScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_TRANSLATE_FINAL_EVIDENCE")
	if dir == "" {
		t.Skip("final translation evidence not requested")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			mode := "light"
			if dark {
				mode = "dark"
			}
			suffix := locale + "-" + mode
			// Word state: the speak button beside copy, with the dictionary below.
			word := translateTestApp(t)
			ws := word.ensureTranslateState()
			ws.visible, ws.input = true, "serendipity"
			ws.dictionary, ws.dictionaryExpanded = dictionaryScreenshotDefinition, true
			ws.phase = translateDone
			ws.output = "机缘巧合；意外发现美好事物的运气"
			if locale == "en" {
				ws.output = "a fortunate discovery made by chance"
			}
			saveTranslateEvidence(t, dir, "translate-word-speak-"+suffix,
				renderTranslatePanelScreenshot(word, locale, dark).Image())
			// Sentence state: no dictionary, the source is what gets read.
			sentence := translateTestApp(t)
			ss := sentence.ensureTranslateState()
			ss.visible, ss.input, ss.phase = true, "Hello, world", translateDone
			ss.output = "你好，世界"
			if locale == "en" {
				ss.output = "Hello, world — translated"
			}
			saveTranslateEvidence(t, dir, "translate-sentence-speak-"+suffix,
				renderTranslatePanelScreenshot(sentence, locale, dark).Image())
			// The card panel with its single-row translate card.
			panel := panelTestApp(t)
			panel.showMenubarPanel()
			tk := tokens(dark, locale)
			pt := ui.NewTester(panel.menubarPanelView, int(tk.SizeMenubarPanelWidth), panel.menubarPanelHeight(tk))
			pt.SetScale(2)
			pt.SetLocale(locale)
			pt.SetDark(dark)
			pt.Frame()
			pt.Advance(700 * time.Millisecond)
			pt.Frame()
			saveTranslateEvidence(t, dir, "menubar-translate-card-"+suffix, pt.Image())
			// The translate settings page, scrolled with the bar visible.
			settings := translateTestApp(t)
			settings.page, settings.translateKeySaved = settingsTranslate, true
			saveTranslateEvidence(t, dir, "translate-settings-scrolled-"+suffix,
				scrolledTranslateSettings(settings, locale, dark).Image())
			for _, width := range []int{840, 1024} {
				saveTranslateEvidence(t, dir, fmt.Sprintf("translate-settings-scrolled-%d-%s", width, suffix),
					scrolledTranslateSettings(settings, locale, dark, width).Image())
			}
		}
	}
}

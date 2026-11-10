package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// A synthetic plain-text definition with the shape the local dictionary
// returns (head, phonetics, parts of speech, senses, origin). No screenshot
// or assertion depends on the wording of an installed dictionary.
const dictionaryScreenshotDefinition = "serendipity ser·en·dip·i·ty | ˌserənˈdipədē | noun 1 an unexpected but fortunate discovery: a fortunate stroke of serendipity. • a series of small discoveries made by chance. 2 the faculty of making such discoveries. ORIGIN coined in the eighteenth century, after a tale of three princes."

func renderTranslateSettingsScreenshot(a *application, locale string, dark bool) *ui.Tester {
	tester := ui.NewTester(a.view, 720, settingsWindowHeight)
	tester.SetScale(2)
	tester.SetLocale(locale)
	tester.SetDark(dark)
	tester.Frame()
	tester.Advance(300 * time.Millisecond)
	tester.Frame()
	return tester
}

// renderTranslatePanelScreenshot advances virtual time past the theme colour
// transition before the captured frame, so a dark capture is fully dark.
func renderTranslatePanelScreenshot(a *application, locale string, dark bool) *ui.Tester {
	tk := tokens(dark, locale)
	width := int(tk.SizeTranslateWidth)
	height := int(a.translateHeight(tk, a.translate))
	tester := ui.NewTester(a.translateView, width, height)
	tester.SetScale(2)
	tester.SetLocale(locale)
	tester.SetDark(dark)
	tester.Frame()
	tester.Advance(700 * time.Millisecond)
	tester.Frame()
	return tester
}

func saveTranslateEvidence(t *testing.T, dir, name string, img *image.RGBA) {
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

func TestTranslateToolScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_TRANSLATE_TOOL_EVIDENCE")
	if dir == "" {
		t.Skip("tool window evidence not requested")
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
			for _, state := range []string{"empty", "sentence", "word", "failed", "pinned"} {
				a := translateTestApp(t)
				s := a.ensureTranslateState()
				s.input, s.output, s.phase = "Good tools make everyday work simpler.", "好工具让日常工作更简单。", translateDone
				switch state {
				case "empty":
					s.input, s.output, s.phase = "", "", translateEmpty
				case "word":
					s.input, s.output, s.dictionary = "serendipity", "机缘巧合；意外发现美好事物的运气", dictionaryScreenshotDefinition
				case "failed":
					s.output, s.phase, s.errorKey = "", translateFailed, "TranslateNetworkError"
				case "pinned":
					s.pinned = true
				}
				tester := ui.NewTester(a.translateView, 480, 360)
				tester.SetScale(2)
				tester.SetLocale(locale)
				tester.SetDark(dark)
				tester.Frame()
				tester.Advance(700 * time.Millisecond)
				tester.Frame()
				saveTranslateEvidence(t, dir, "translate-tool-"+state+"-"+locale+"-"+mode, tester.Image())
			}
		}
	}
}

// renderSelectionSettingsScreenshot advances well past the theme transition
// so a dark capture is fully dark before it is saved.
func renderSelectionSettingsScreenshot(a *application, locale string, dark bool) *ui.Tester {
	tester := ui.NewTester(a.view, 720, settingsWindowHeight)
	tester.SetScale(2)
	tester.SetLocale(locale)
	tester.SetDark(dark)
	tester.Frame()
	tester.Advance(700 * time.Millisecond)
	tester.Frame()
	return tester
}

// TestTranslateSelectionScreenshots captures the accessibility hint and the
// selection row in both languages and both themes. It is in-memory only: no
// window, no AX call, no permission prompt and no system window is opened.
// CUBBIX_TRANSLATE_SELECTION_EVIDENCE names a directory outside the repo.
func TestTranslateSelectionScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_TRANSLATE_SELECTION_EVIDENCE")
	if dir == "" {
		t.Skip("selection evidence not requested")
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
			panel := translateTestApp(t)
			s := panel.ensureTranslateState()
			s.denied = true
			s.phase = translateEmpty
			saveTranslateEvidence(t, dir, "translate-selection-denied-"+suffix,
				renderTranslatePanelScreenshot(panel, locale, dark).Image())
			for _, trusted := range []bool{false, true} {
				state := "unauthorized"
				if trusted {
					state = "authorized"
				}
				settings := selectionTestApp(t, nil)
				settings.page = settingsTranslate
				settings.translateSelectionTrusted = trusted
				settings.translateKeySaved = true
				tester := renderSelectionSettingsScreenshot(settings, locale, dark)
				saveTranslateEvidence(t, dir, "translate-selection-settings-"+state+"-"+suffix, tester.Image())
				tester.Scroll(650, 350, 0, 1000)
				tester.Advance(700 * time.Millisecond)
				tester.Frame()
				saveTranslateEvidence(t, dir, "translate-selection-settings-scrolled-"+state+"-"+suffix, tester.Image())
			}
		}
	}
}

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
			tt := renderTranslateSettingsScreenshot(a, locale, dark)
			saveTranslateEvidence(t, dir, fmt.Sprintf("translate-settings-%s", suffix), tt.Image())
			// The floating panel in its finished state.
			b := translateTestApp(t)
			s := b.ensureTranslateState()
			s.input = "Hello, world"
			s.output = "你好，世界"
			if locale == "en" {
				s.output = "Hello, world — translated"
			}
			s.phase = translateDone
			saveTranslateEvidence(t, dir, fmt.Sprintf("translate-panel-%s", suffix),
				renderTranslatePanelScreenshot(b, locale, dark).Image())
		}
	}
}

// TestTranslateDictionaryScreenshots captures the three word states in both
// languages and both themes. CUBBIX_TRANSLATE_DICT_EVIDENCE points at a
// directory outside the repository; without it the test does nothing.
func TestTranslateDictionaryScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_TRANSLATE_DICT_EVIDENCE")
	if dir == "" {
		t.Skip("dictionary evidence not requested")
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
			for _, state := range []struct {
				name     string
				failed   bool
				expanded bool
			}{
				{"expanded", false, true},
				{"collapsed", false, false},
				{"failed", true, true},
				{"capped", false, true},
			} {
				a := translateTestApp(t)
				s := a.ensureTranslateState()
				s.input = "serendipity"
				s.dictionary = dictionaryScreenshotDefinition
				s.dictionaryExpanded = state.expanded
				if state.failed {
					s.phase, s.errorKey, s.output = translateFailed, "TranslateNetworkError", ""
				} else {
					s.phase = translateDone
					s.output = "机缘巧合；意外发现美好事物的运气"
					if locale == "en" {
						s.output = "a fortunate discovery made by chance"
					}
				}
				if state.name == "capped" {
					s.output = strings.Repeat(s.output+"\n", 100)
				}
				name := fmt.Sprintf("translate-word-%s-%s", state.name, suffix)
				tester := renderTranslatePanelScreenshot(a, locale, dark)
				saveTranslateEvidence(t, dir, name, tester.Image())
				if state.name == "capped" {
					header, ok := tester.Find(a.text(locale, "TranslateDictionary"))
					if !ok {
						t.Fatal("dictionary header missing")
					}
					tester.Scroll(header.X+header.W/2, header.Y+header.H+8, 0, 10000)
					tester.Frame()
					saveTranslateEvidence(t, dir, fmt.Sprintf("translate-word-capped-scrolled-%s", suffix), tester.Image())
				}
			}
		}
	}
}

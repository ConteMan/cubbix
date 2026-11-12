package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/ConteMan/cubbix/internal/devtools"
)

func saveDevtoolsEvidence(t *testing.T, dir, name string, img *image.RGBA) {
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

// devtoolsScreenshotApp is a headless app whose QR and devtools seams work
// without a main-thread loop: the side-by-side picture renders the QR symbol at
// once, and no window is created.
func devtoolsScreenshotApp(t *testing.T) *application {
	t.Helper()
	a := qrTestApp(t)
	a.createDevtoolsWindow = func(*devtoolsState) {}
	a.devtoolsOnMainThread = func(fn func()) { fn() }
	a.devtoolsAsync = false
	return a
}

// renderDevtoolsWindow advances past the theme transition, so a dark capture is
// fully dark before it is saved.
func renderDevtoolsWindow(a *application, locale string, dark bool) *ui.Tester {
	tk := tokens(dark, locale)
	tester := ui.NewTester(a.devtoolsView, int(tk.SizeDevtoolsWidth), int(tk.SizeDevtoolsWindowHeight))
	tester.SetScale(2)
	tester.SetLocale(locale)
	tester.SetDark(dark)
	tester.Frame()
	tester.Advance(700 * time.Millisecond)
	tester.Frame()
	return tester
}

// TestDevtoolsToolWindowScreenshots renders the tool window's states, the card
// panel with the devtools row and the settings page. It is an in-memory render
// of the real views: no window is created, no screen is read and nothing is
// activated. CUBBIX_DEVTOOLS_TOOL_EVIDENCE names the output directory.
func TestDevtoolsToolWindowScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_DEVTOOLS_TOOL_EVIDENCE")
	if dir == "" {
		t.Skip("devtools tool window evidence not requested")
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
			states := []struct {
				name    string
				prepare func(a *application)
			}{
				{"time", func(a *application) {
					a.openDevtools(devtoolsRequest{Kind: devtools.KindTime})
				}},
				{"date", func(a *application) {
					a.openDevtools(devtoolsRequest{Kind: devtools.KindTime, Text: "2026-11-12 09:30"})
				}},
				{"uuid-pair", func(a *application) {
					a.openDevtools(devtoolsRequest{Kind: devtools.KindUUID})
				}},
				{"uuid-batch", func(a *application) {
					a.openDevtools(devtoolsRequest{Kind: devtools.KindUUID, Version: 7, Count: 4})
				}},
				{"uuid-options", func(a *application) {
					a.openDevtools(devtoolsRequest{Kind: devtools.KindUUID})
					a.writeDevtoolsSwitches(true, true)
					a.setDevtoolsPinned(true)
					a.devtools.copiedKey = "devtools.uuid.1"
					a.devtools.copiedUntil = time.Now().Add(time.Minute)
				}},
				{"error", func(a *application) {
					a.openDevtools(devtoolsRequest{Kind: devtools.KindTime, Text: "ts 946684800000"})
				}},
			}
			for _, state := range states {
				a, _ := devtoolsWindowTestApp(t)
				state.prepare(a)
				tester := renderDevtoolsWindow(a, locale, dark)
				saveDevtoolsEvidence(t, dir, "devtools-window-"+state.name+"-"+locale+"-"+mode, tester.Image())
			}
			// The settings page with the shortcut row.
			a, _ := devtoolsWindowTestApp(t)
			a.page = settingsDevtools
			tester := ui.NewTester(a.view, 720, settingsWindowHeight)
			tester.SetScale(2)
			tester.SetLocale(locale)
			tester.SetDark(dark)
			tester.Frame()
			tester.Advance(700 * time.Millisecond)
			tester.Frame()
			saveDevtoolsEvidence(t, dir, "settings-devtools-"+locale+"-"+mode, tester.Image())
			// The card panel with the devtools row.
			b, _ := devtoolsWindowTestApp(t)
			b.showMenubarPanel()
			panelTokens := tokens(dark, locale)
			panel := ui.NewTester(b.menubarPanelView, int(panelTokens.SizeMenubarPanelWidth), b.menubarPanelHeight(panelTokens))
			panel.SetScale(2)
			panel.SetLocale(locale)
			panel.SetDark(dark)
			panel.Frame()
			panel.Advance(700 * time.Millisecond)
			panel.Frame()
			saveDevtoolsEvidence(t, dir, "panel-devtools-"+locale+"-"+mode, panel.Image())
		}
	}
}

// TestDevtoolsToolWindowsSideBySide draws the JSON, Translate, QR and Devtools
// windows in one picture, so their shared chrome can be compared at a glance.
// CUBBIX_DEVTOOLS_COMPARE_EVIDENCE names the output directory.
func TestDevtoolsToolWindowsSideBySide(t *testing.T) {
	dir := os.Getenv("CUBBIX_DEVTOOLS_COMPARE_EVIDENCE")
	if dir == "" {
		t.Skip("tool window comparison not requested")
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
			a := devtoolsScreenshotApp(t)
			a.openDevtools(devtoolsRequest{Kind: devtools.KindTime, Text: "2026-11-12"})
			dev := renderDevtoolsWindow(a, locale, dark)
			a.openQRText("https://example.com/orders/2026")
			a.qr.due = time.Now().Add(-time.Second)
			qr := renderQRWindow(a, locale, dark)
			s := a.ensureTranslateState()
			s.input, s.output, s.phase = "Good tools make everyday work simpler.", "好工具让日常工作更简单。", translateDone
			tr := ui.NewTester(a.translateView, 480, 360)
			tr.SetScale(2)
			tr.SetLocale(locale)
			tr.SetDark(dark)
			tr.Frame()
			tr.Advance(700 * time.Millisecond)
			tr.Frame()
			jsonWindow := ui.NewTester(func(c *ui.Context) {
				locale, t := applyTheme(c)
				ui.Column(c).Fill().Background(ui.Hex(t.ColorCard)).Children(func() {
					ui.Row(c).FillWidth().Height(t.SizeJSONToolbarHeight).Padding(t.Space8, t.Space12, t.Space8, t.SizeJSONTitleInset).Children(func() {
						ui.Row(c).FillHeight().DragWindow().AlignItems(ui.Center).Children(func() {
							ui.Text(c, a.text(locale, "JSON")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
						})
					})
					ui.Box(c).FillWidth().Height(t.StrokeHairline).Background(ui.Hex(t.ColorSeparator))
					ui.Column(c).FillWidth().Grow(1).Padding(t.Space12).Children(func() {
						ui.Text(c, a.text(locale, "JSONPageDesc")).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorTextSecondary))
					})
				})
			}, 480, 360)
			jsonWindow.SetScale(2)
			jsonWindow.SetLocale(locale)
			jsonWindow.SetDark(dark)
			jsonWindow.Frame()
			jsonWindow.Advance(700 * time.Millisecond)
			jsonWindow.Frame()
			card := ui.Hex(tokens(dark, locale).ColorCard)
			background := color.RGBA{card.R, card.G, card.B, card.A}
			saveDevtoolsEvidence(t, dir, fmt.Sprintf("tool-windows-%s-%s", locale, mode),
				sideBySide(background, jsonWindow.Image(), tr.Image(), qr.Image(), dev.Image()))
		}
	}
}

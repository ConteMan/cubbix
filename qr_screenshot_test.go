package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	qrlib "github.com/ConteMan/cubbix/internal/qr"
)

// qrScreenshotText is a long Chinese sentence: enough to reach a symbol with
// many modules without hitting the capacity.
const qrScreenshotText = "二维码把一段文字变成一张可以扫的图：网址、短句、地址、单号都行。Cubbix 的二维码窗口随输入实时更新，可以复制图片、存成 PNG，也可以直接拖到别的应用里。"

func saveQREvidence(t *testing.T, dir, name string, img *image.RGBA) {
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

// renderQRWindow advances past the theme transition, so a dark capture is
// fully dark before it is saved.
func renderQRWindow(a *application, locale string, dark bool) *ui.Tester {
	tk := tokens(dark, locale)
	tester := ui.NewTester(a.qrView, int(tk.SizeQRWidth), int(tk.SizeQRWindowHeight))
	tester.SetScale(2)
	tester.SetLocale(locale)
	tester.SetDark(dark)
	tester.Frame()
	tester.Advance(700 * time.Millisecond)
	tester.Frame()
	return tester
}

// TestQRToolScreenshots captures the QR window in its five states, the card
// panel with the QR row, the settings page, and one picture of the three tool
// windows side by side. Everything is rendered in memory: no window is
// created, no screen is read and nothing is activated.
func TestQRToolScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_QR_TOOL_EVIDENCE")
	if dir == "" {
		t.Skip("QR tool window evidence not requested")
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
			for _, state := range []string{"empty", "url", "chinese", "toolong", "pinned"} {
				a := qrTestApp(t)
				a.openQRText("")
				switch state {
				case "url":
					a.openQRText("https://example.com/orders/2026?utm=mail")
					a.qr.due = time.Now().Add(-time.Second)
				case "chinese":
					a.openQRText(qrScreenshotText)
					a.qr.due = time.Now().Add(-time.Second)
				case "toolong":
					a.openQRText(strings.Repeat("a", qrlib.MaxBytes+1))
					a.qr.due = time.Now().Add(-time.Second)
				case "pinned":
					a.openQRText("https://example.com/orders/2026?utm=mail")
					a.qr.due = time.Now().Add(-time.Second)
					a.qr.pinned = true
				}
				tester := renderQRWindow(a, locale, dark)
				saveQREvidence(t, dir, "qr-window-"+state+"-"+locale+"-"+mode, tester.Image())
			}
			// The settings page with the QR row selected.
			a := qrTestApp(t)
			a.page = settingsQR
			tester := ui.NewTester(a.view, 720, settingsWindowHeight)
			tester.SetScale(2)
			tester.SetLocale(locale)
			tester.SetDark(dark)
			tester.Frame()
			tester.Advance(700 * time.Millisecond)
			tester.Frame()
			saveQREvidence(t, dir, "settings-qr-"+locale+"-"+mode, tester.Image())
		}
	}
}

// TestQRPanelCardScreenshot captures the card panel with the QR row.
func TestQRPanelCardScreenshot(t *testing.T) {
	dir := os.Getenv("CUBBIX_QR_PANEL_EVIDENCE")
	if dir == "" {
		t.Skip("QR card panel evidence not requested")
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
			a := panelTestApp(t)
			a.showMenubarPanel()
			tk := tokens(dark, locale)
			tester := ui.NewTester(a.menubarPanelView, int(tk.SizeMenubarPanelWidth), a.menubarPanelHeight(tk))
			tester.SetScale(2)
			tester.SetLocale(locale)
			tester.SetDark(dark)
			tester.Frame()
			tester.Advance(700 * time.Millisecond)
			tester.Frame()
			saveQREvidence(t, dir, fmt.Sprintf("panel-qr-%s-%s", locale, mode), tester.Image())
		}
	}
}

// TestQRToolWindowsSideBySide draws the JSON, Translate and QR tool windows in
// one picture, so their shared chrome can be compared at a glance. Each window
// is rendered by its own view and copied into one canvas; nothing is shown on
// screen.
func TestQRToolWindowsSideBySide(t *testing.T) {
	dir := os.Getenv("CUBBIX_QR_COMPARE_EVIDENCE")
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
			a := qrTestApp(t)
			a.openQRText("https://example.com/orders/2026")
			a.qr.due = time.Now().Add(-time.Second)
			qr := renderQRWindow(a, locale, dark)
			// The translate panel shows a finished translation.
			s := a.ensureTranslateState()
			s.input, s.output, s.phase = "Good tools make everyday work simpler.", "好工具让日常工作更简单。", translateDone
			tr := ui.NewTester(a.translateView, 480, 360)
			tr.SetScale(2)
			tr.SetLocale(locale)
			tr.SetDark(dark)
			tr.Frame()
			tr.Advance(700 * time.Millisecond)
			tr.Frame()
			// A JSON window is a web view; its own page is out of scope here, so
			// the comparison uses the tool chrome the app draws itself.
			json := ui.NewTester(func(c *ui.Context) {
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
			json.SetScale(2)
			json.SetLocale(locale)
			json.SetDark(dark)
			json.Frame()
			json.Advance(700 * time.Millisecond)
			json.Frame()
			card := ui.Hex(tokens(dark, locale).ColorCard)
			background := color.RGBA{card.R, card.G, card.B, card.A}
			saveQREvidence(t, dir, fmt.Sprintf("tool-windows-%s-%s", locale, mode), sideBySide(background, qr.Image(), tr.Image(), json.Image()))
		}
	}
}

// sideBySide copies window pictures into one canvas with a small gap, filling
// the area a shorter picture leaves with the card colour. Each picture keeps
// its own size, so the comparison shows the real geometry.
func sideBySide(background color.RGBA, images ...*image.RGBA) *image.RGBA {
	gap := 16
	width, height := 2*gap*(len(images)-1), 0
	for _, img := range images {
		width += img.Bounds().Dx()
		height = max(height, img.Bounds().Dy())
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			canvas.SetRGBA(x, y, background)
		}
	}
	offsetX := 0
	for index, img := range images {
		if index > 0 {
			offsetX += gap
		}
		bounds := img.Bounds()
		for y := 0; y < bounds.Dy(); y++ {
			for x := 0; x < bounds.Dx(); x++ {
				canvas.SetRGBA(offsetX+x, y, img.RGBAAt(bounds.Min.X+x, bounds.Min.Y+y))
			}
		}
		offsetX += bounds.Dx()
	}
	return canvas
}

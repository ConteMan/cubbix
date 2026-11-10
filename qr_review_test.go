package main

import (
	"bytes"
	"errors"
	"image"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	codeqr "github.com/piglig/go-qr/v2"

	qrlib "github.com/ConteMan/cubbix/internal/qr"
)

func TestQRFixedEncodingParameters(t *testing.T) {
	for _, text := range []string{"https://example.com", "漢字", "https://例.jp/漢字", "🙂", strings.Repeat("a", 2331), strings.Repeat("汉", 776), strings.Repeat("1", 3000)} {
		code, err := qrlib.Encode(text)
		if err != nil {
			t.Fatal(err)
		}
		img, err := code.Image(4)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := codeqr.Decode(img)
		if err != nil || decoded.Text != text || decoded.ECC != codeqr.ECCMedium || decoded.Version != code.Version() {
			t.Fatalf("round trip of %d bytes: %+v, %v", len(text), decoded, err)
		}
		segments := decoded.Segments
		if strings.ContainsAny(text, "漢字例🙂汉") {
			if len(segments) != 2 || segments[0].Mode != codeqr.ModeECI || segments[0].ECI != 26 || segments[1].Mode != codeqr.ModeByte || segments[1].ECI != 26 {
				t.Fatalf("non-ASCII segments = %+v", segments)
			}
		} else if len(segments) != 1 {
			t.Fatalf("ASCII segments = %+v", segments)
		}
	}
	_, err := qrlib.Encode(strings.Repeat("a", 2332))
	if !errors.Is(err, qrlib.ErrTooLong) || !errors.Is(err, codeqr.ErrDataTooLong) {
		t.Fatalf("capacity sentinel chain = %v", err)
	}
}

func TestQRDisplayScaleAndGeometry(t *testing.T) {
	a, windows := qrWindowTestApp(t)
	screens := paletteTestScreens(a)
	a.openQRText(strings.Repeat("a", 1500))
	a.encodeQR(a.qr.input)
	symbol := a.qr.code
	for _, scale := range []float64{1, 1.25, 1.5, 2, 3} {
		screens.displays[0].ScaleFactor = scale
		a.reconcileQRDisplay()
		for _, height := range []int{440, 441, 520, 700} {
			(*windows)[0].SetContentSize(480, height)
			a.refreshQRBitmap()
			side := float64(symbol.Modules() + 8)
			want := max(1, int(math.Floor(float64(a.qr.box)*scale/side)))
			if a.qr.scale != scale || a.qr.px != want || a.qr.code != symbol {
				t.Fatalf("scale %v height %d: scale %v px %d want %d", scale, height, a.qr.scale, a.qr.px, want)
			}
			width, height := a.qr.bitmap.Size()
			if width != int(side)*want || height != width || float64(width)/scale > float64(a.qr.box) {
				t.Fatalf("bitmap %dx%d exceeds box %v at scale %v", width, height, a.qr.box, scale)
			}
		}
	}
	screens.displays[1].ScaleFactor = 1
	(*windows)[0].drag(-1500, 200)
	a.observeQRMove()
	a.refreshQRScale()
	if a.qr.scale != 1 {
		t.Fatal("moving displays kept the old scale")
	}
}

func TestQRRenderedPixelsAndQuietZone(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		a := qrTestApp(t)
		screens := paletteTestScreens(a)
		screens.displays[0].ScaleFactor = scale
		a.openQRText("https://example.com/a")
		a.encodeQR(a.qr.input)
		tester := qrWindowTester(a, "en", false)
		tester.SetScale(float32(scale))
		tester.Frame()
		rect, found := tester.Find(a.text("en", "QRDragHint"))
		if !found {
			t.Fatal("image element missing")
		}
		expected, err := a.qr.code.Image(a.qr.px)
		if err != nil {
			t.Fatal(err)
		}
		size := expected.Bounds().Dx()
		if math.Abs(float64(rect.W)*scale-float64(size)) > 0.001 {
			t.Fatalf("image stretched: %v DIPs, %d pixels, scale %v", rect.W, size, scale)
		}
		actual := tester.Image()
		originX, originY := int(math.Round(float64(rect.X)*scale)), int(math.Round(float64(rect.Y)*scale))
		for row := 0; row < size; row++ {
			for column := 0; column < size; column++ {
				red, green, blue, _ := actual.At(originX+column, originY+row).RGBA()
				want, _, _, _ := expected.At(column, row).RGBA()
				if red != want || green != want || blue != want {
					t.Fatalf("scale %v image pixel (%d,%d) = %d/%d/%d, want %d", scale, column, row, red, green, blue, want)
				}
				margin := qrlib.QuietZone * a.qr.px
				if (column < margin || row < margin || column >= size-margin || row >= size-margin) && want != 65535 {
					t.Fatal("quiet zone contains a dark pixel")
				}
			}
		}
	}
}

func TestQRSelectionSecondPressAndBusyTranslation(t *testing.T) {
	a := qrTestApp(t)
	a.selection = &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "old selection"}
	var queued []func()
	a.qrRunOnMain = func(finish func()) { queued = append(queued, finish) }
	a.toggleQR()
	a.toggleQR()
	if a.qr == nil || a.qr.input != "" || a.qrSelectionPending {
		t.Fatal("second press did not open empty")
	}
	for _, finish := range queued {
		finish()
	}
	if a.qr.input != "" {
		t.Fatal("late selection replaced second press")
	}
	translation := selectionTestApp(t, &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "ignored"})
	translation.translateSelectionTrusted = true
	translation.selectionReading.Store(true)
	translation.toggleTranslate()
	if !translation.translateVisible() || translation.translate.input != "" || !translation.translateSelectionTrusted || translation.translateSelectionHintShown {
		t.Fatal("busy marker changed translation behavior or permission")
	}
}

func TestQRClipboardSaveAndCopyExpiry(t *testing.T) {
	a := qrTestApp(t)
	a.openQRText("hello 🙂")
	a.encodeQR(a.qr.input)
	var copied []byte
	a.qrCopyPNG = func(data []byte) error { copied = bytes.Clone(data); return nil }
	a.copyQRImage()
	img, _, err := image.Decode(bytes.NewReader(copied))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := codeqr.Decode(img)
	if err != nil || decoded.Text != a.qr.input || img.Bounds().Dx() != a.qr.code.SidePixels(16) {
		t.Fatalf("copied PNG = %+v, %v", decoded, err)
	}
	tester := qrWindowTester(a, "en", false)
	tester.Frame()
	if _, found := tester.Find(a.text("en", "QRCopied")); !found {
		t.Fatal("copy marker missing")
	}
	tester.Advance(2 * time.Second)
	tester.Frame()
	if !a.qr.copiedUntil.IsZero() {
		t.Fatal("copy marker did not expire")
	}
	for _, scenario := range []string{"cancel", "dialog-error", "write-error", "success"} {
		t.Run(scenario, func(t *testing.T) {
			app := qrTestApp(t)
			app.openQRText("saved")
			app.encodeQR(app.qr.input)
			finished := make(chan func(), 1)
			chosen := make(chan struct{})
			writes := 0
			app.qrRunOnMain = func(finish func()) { finished <- finish }
			app.qrChooseSave = func(*mygo.Window) (string, error) {
				defer close(chosen)
				if scenario == "cancel" {
					return "", nil
				}
				if scenario == "dialog-error" {
					return "", errors.New("dialog failed")
				}
				return "code.png", nil
			}
			app.qrWriteFile = func(_ string, data []byte) error {
				writes++
				if scenario == "write-error" {
					return errors.New("write failed")
				}
				if !bytes.HasPrefix(data, []byte("\x89PNG")) {
					return errors.New("invalid image")
				}
				return nil
			}
			app.saveQRImage()
			select {
			case <-chosen:
			case <-time.After(time.Second):
				t.Fatal("save seam not called")
			}
			if scenario == "cancel" {
				if writes != 0 || app.qr.saveFailed {
					t.Fatal("cancel wrote or reported failure")
				}
				return
			}
			select {
			case finish := <-finished:
				finish()
			case <-time.After(time.Second):
				t.Fatal("save result not dispatched")
			}
			if app.qr.saveFailed != (scenario != "success") || writes != map[string]int{"dialog-error": 0, "write-error": 1, "success": 1}[scenario] {
				t.Fatalf("save failed=%v writes=%d", app.qr.saveFailed, writes)
			}
		})
	}
}

func TestQRShortcutFailureAndUnknownTarget(t *testing.T) {
	a := qrTestApp(t)
	a.registerShortcut = func(string, func()) error { return errors.New("unavailable") }
	a.registerQRShortcut()
	if a.slotError("qr") != "ShortcutUnavailable" || a.ownedShortcut("qr") != "" {
		t.Fatal("failed registration lost its diagnostic")
	}
	a.page = settingsQR
	tester := ui.NewTester(a.view, 720, settingsWindowHeight)
	tester.SetLocale("en")
	tester.Frame()
	if !tester.HasText(a.text("en", "ShortcutUnavailable")) {
		t.Fatal("settings did not expose failed registration")
	}
	a.changeQREnabled(false)
	tester.Frame()
	if err := tester.Click(a.text("en", "QRRecordShortcut")); err != nil {
		t.Fatal(err)
	}
	if a.shortcutRecording || a.slotError("qr") != "" {
		t.Fatal("disabled recorder accepted input or kept error")
	}
	a.registerShortcut = func(string, func()) error { return nil }
	before := a.config
	a.beginTileShortcutRecording("unknown")
	a.recordShortcut(ui.Ctrl|ui.Alt|ui.Cmd, ui.KeyY)
	if !reflect.DeepEqual(before, a.config) {
		t.Fatal("unknown recorder target changed config")
	}
}

func TestCLIQRWireMetadata(t *testing.T) {
	_, path := testServer(t, qrTestApp(t))
	text := "https://example.com/中文🙂"
	result := do(t, dial(t, path), cliQRRequest(text), []byte(text))
	if !result.OK || result.QR == nil || result.QR.Bytes != len(text) || result.Bytes != 0 || result.Translate != nil {
		t.Fatalf("wire result = %+v", result)
	}
}

func TestQRSingleWindowAndCloseKeys(t *testing.T) {
	for _, scenario := range []struct {
		mods ui.Modifiers
		key  ui.Key
	}{{0, ui.KeyEscape}, {ui.Cmd, ui.KeyW}} {
		app, windows := qrWindowTestApp(t)
		paletteTestScreens(app)
		app.openQRText("first")
		state := app.qr
		app.openQRText("second")
		if len(*windows) != 1 || app.qr != state || app.qr.input != "second" {
			t.Fatal("opening replaced the single instance")
		}
		tester := qrWindowTester(app, "en", false)
		tester.Frame()
		tester.Key(ui.Cmd|ui.Alt, ui.KeyP)
		if !state.pinned || !reflect.DeepEqual((*windows)[0].pins, []bool{true}) {
			t.Fatal("pin shortcut did not update the native seam")
		}
		tester.Key(scenario.mods, scenario.key)
		if app.qr != nil || !(*windows)[0].destroyed {
			t.Fatal("close shortcut left a window")
		}
	}
}

func TestQRChangedTextCannotExportOldSymbol(t *testing.T) {
	app := qrTestApp(t)
	app.openQRText("old")
	app.encodeQR("old")
	app.qrCopyPNG = func([]byte) error { t.Fatal("copied old symbol"); return nil }
	app.qrChooseSave = func(*mygo.Window) (string, error) { t.Error("saved old symbol"); return "", nil }
	app.qr.input = "new"
	app.syncQRInput()
	if _, err := app.qrPNG(); err == nil {
		t.Fatal("export accepted stale symbol")
	}
	app.copyQRImage()
	app.saveQRImage()
	tester := qrWindowTester(app, "en", false)
	tester.Frame()
	if err := tester.Click(app.text("en", "QRCopy")); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click(app.text("en", "QRSave")); err != nil {
		t.Fatal(err)
	}
}

func TestQRAndTranslationPendingSelectionSurvivesOverlays(t *testing.T) {
	for _, tool := range []string{"qr", "translate"} {
		for _, overlay := range []string{"palette", "panel"} {
			t.Run(tool+"/"+overlay, func(t *testing.T) {
				app, submitted := configuredSelectionApp(t, &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "selected text"})
				app.createQRWindow = nil
				app.qrAsync = false
				app.createPaletteWindow = func(*paletteState) {}
				var queued []func()
				dispatch := func(finish func()) { queued = append(queued, finish) }
				app.qrRunOnMain = dispatch
				app.translateRunOnMain = dispatch
				if tool == "qr" {
					app.toggleQR()
				} else {
					app.toggleTranslate()
				}
				if len(queued) != 1 {
					t.Fatal("selection did not wait for dispatch")
				}
				if overlay == "palette" {
					app.showPalette()
				} else {
					app.showMenubarPanel()
				}
				for _, finish := range queued {
					finish()
				}
				if tool == "qr" {
					if !app.qrVisible() || app.qr.input != "selected text" {
						t.Fatal("overlay cancelled pending QR selection")
					}
				} else if !app.translateVisible() || app.translate.input != "selected text" || len(*submitted) != 1 {
					t.Fatal("overlay cancelled pending translation selection")
				}
			})
		}
	}
}

package main

import (
	"bytes"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	codeqr "github.com/piglig/go-qr/v2"

	"github.com/ConteMan/cubbix/internal/cli"
	qrlib "github.com/ConteMan/cubbix/internal/qr"
)

// qrTestApp is a headless app whose QR seams never touch a window, the
// clipboard, a save dialog or a worker.
func qrTestApp(t *testing.T) *application {
	t.Helper()
	a := panelTestApp(t)
	// No native window in any test: the real builder asks MyGo for the theme,
	// which only exists inside a running app.
	a.createQRWindow = nil
	a.qrRunOnMain = func(fn func()) { fn() }
	a.qrAsync = false
	a.qrReadClipboard = func() string {
		t.Fatal("the QR module read the clipboard unasked")
		return ""
	}
	a.qrCopyPNG = func([]byte) error { return nil }
	a.qrChooseSave = func(*mygo.Window) (string, error) { return "", nil }
	a.qrWriteFile = func(string, []byte) error { return nil }
	return a
}

func TestQRRoundTrip(t *testing.T) {
	corpus := []string{
		"https://example.com/a",
		"https://example.com/orders/2026?utm=mail&id=1234567890",
		"https://example.com/" + strings.Repeat("segment/", 30),
		"你好，世界",
		"中文长文：" + strings.Repeat("二维码", 40),
		"emoji 🎉🙂 and text",
		"a",
		strings.Repeat("汉", 776),
	}
	for _, text := range corpus {
		code, err := qrlib.Encode(text)
		if err != nil {
			t.Fatalf("encode %d bytes: %v", len(text), err)
		}
		img, err := code.Image(4)
		if err != nil {
			t.Fatal(err)
		}
		got, err := codeqr.Decode(img)
		if err != nil {
			t.Fatalf("decode %d bytes: %v", len(text), err)
		}
		if got.Text != text {
			t.Fatalf("round trip differs: %q", got.Text)
		}
	}
}

func TestQRPNGDecodes(t *testing.T) {
	text := "https://example.com/orders/2026"
	code, err := qrlib.Encode(text)
	if err != nil {
		t.Fatal(err)
	}
	png, err := code.PNG(qrlib.ExportModule)
	if err != nil {
		t.Fatal(err)
	}
	file, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	if got := file.Bounds().Dx(); got != (code.Modules()+2*qrlib.QuietZone)*qrlib.ExportModule {
		t.Fatalf("png side = %d", got)
	}
	decoded, err := codeqr.Decode(file)
	if err != nil || decoded.Text != text {
		t.Fatalf("png decode = %+v, %v", decoded, err)
	}
}

func TestQRQuietZone(t *testing.T) {
	code, err := qrlib.Encode("https://example.com/orders/2026")
	if err != nil {
		t.Fatal(err)
	}
	rows := strings.Split(strings.TrimSuffix(code.Matrix(4), "\n"), "\n")
	if want := (code.Modules() + 8 + 1) / 2; len(rows) != want {
		t.Fatalf("rows = %d, want %d", len(rows), want)
	}
	for index, row := range rows {
		if got, want := len([]rune(row)), code.Modules()+8; got != want {
			t.Fatalf("columns = %d, want %d", got, want)
		}
		for _, glyph := range row {
			if !strings.ContainsRune("█▀▄ ", glyph) {
				t.Fatalf("unexpected glyph %q", glyph)
			}
		}
		if !strings.HasPrefix(row, "    ") || !strings.HasSuffix(row, "    ") || ((index < 2 || index >= len(rows)-2) && strings.TrimSpace(row) != "") {
			t.Fatal("terminal quiet zone contains a dark module")
		}
	}
}

// TestQRDisplayPixels pins the integer device-pixel rule: the bitmap's side is
// (modules+8)·px device pixels, shown as that side divided by the display
// scale, and never stretched to the box.
func TestQRDisplayPixels(t *testing.T) {
	for _, modules := range []int{21, 25, 33, 69, 177} {
		for _, scale := range []float32{1, 2, 3} {
			px := qrDisplayPixels(320, float32(modules), scale)
			if px < 1 {
				t.Fatalf("modules %d scale %v: px = %d", modules, scale, px)
			}
			side := qrBitmapSide(modules, px)
			if side != (modules+2*qrlib.QuietZone)*px {
				t.Fatalf("modules %d: side = %d", modules, side)
			}
			dip := float32(side) / scale
			// The symbol fills the box but never overflows it by more than one
			// module: the floor is what keeps the edges whole.
			if dip > 320+float32(qrlib.QuietZone)*2/scale {
				t.Fatalf("modules %d scale %v: %v DIPs overflows the box", modules, scale, dip)
			}
			if dip < 320-float32(modules+2*qrlib.QuietZone)/scale {
				t.Fatalf("modules %d scale %v: %v DIPs leaves the box half empty", modules, scale, dip)
			}
		}
	}
	// A tiny box still gives one pixel per module, never a zero.
	if got := qrDisplayPixels(20, 177, 1); got != 1 {
		t.Fatalf("tiny box = %d", got)
	}
}

func TestQRTooLong(t *testing.T) {
	// Lower case is byte mode: 2331 bytes fit and 2332 do not. Upper case and
	// digits go to the alphanumeric and numeric segments, which hold more, so
	// MaxBytes is the byte-mode capacity and never a limit of every payload.
	if _, err := qrlib.Encode(strings.Repeat("a", qrlib.MaxBytes)); err != nil {
		t.Fatalf("%d bytes: %v", qrlib.MaxBytes, err)
	}
	_, err := qrlib.Encode(strings.Repeat("a", qrlib.MaxBytes+1))
	if !errors.Is(err, qrlib.ErrTooLong) {
		t.Fatalf("%d bytes = %v, want ErrTooLong", qrlib.MaxBytes+1, err)
	}
	if _, err := qrlib.Encode(strings.Repeat("A", qrlib.MaxBytes+1)); err != nil {
		t.Fatalf("upper case holds more than the byte-mode capacity: %v", err)
	}
	if _, err := qrlib.Encode(strings.Repeat("汉", qrlib.MaxChinese)); err != nil {
		t.Fatalf("776 characters: %v", err)
	}
	if _, err := qrlib.Encode(strings.Repeat("汉", qrlib.MaxChinese+1)); !errors.Is(err, qrlib.ErrTooLong) {
		t.Fatalf("777 characters = %v, want ErrTooLong", err)
	}
	// The window enters tooLong and keeps the input intact.
	a := qrTestApp(t)
	a.openQRText(strings.Repeat("a", qrlib.MaxBytes+1))
	a.qr.due = time.Now().Add(-time.Second)
	tester := qrWindowTester(a, "en", false)
	tester.Frame()
	if a.qr.phase != qrTooLong || len(a.qr.input) != qrlib.MaxBytes+1 {
		t.Fatalf("too-long state = %v, %d bytes", a.qr.phase, len(a.qr.input))
	}
	if !tester.HasText(a.text("en", "QRTooLongHint")) {
		t.Fatalf("missing the capacity hint: %v", tester.Texts())
	}
}

func TestQRPrefill(t *testing.T) {
	cases := []struct {
		name     string
		clip     string
		expected string
	}{
		{"https", "https://example.com/a", "https://example.com/a"},
		{"http with spaces", "  http://example.com/a\n", "http://example.com/a"},
		{"upper case", "HTTPS://example.com/a", "HTTPS://example.com/a"},
		{"ftp", "ftp://example.com/a", ""},
		{"no scheme", "example.com", ""},
		{"sentence", "look at example.com", ""},
		{"empty", "", ""},
		{"image", "", ""},
		{"no host", "https://", ""},
		{"too long", "https://example.com/" + strings.Repeat("a", qrMaxClipboardBytes), ""},
	}
	for _, c := range cases {
		a := qrTestApp(t)
		clip := c.clip
		a.qrReadClipboard = func() string { return clip }
		if got := a.qrPrefill(); got != c.expected {
			t.Fatalf("%s: prefill = %q, want %q", c.name, got, c.expected)
		}
	}
}

func TestQRSelectionPrecedesClipboard(t *testing.T) {
	a := qrTestApp(t)
	reader := &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "https://selected.example"}
	a.selection = reader
	a.qrReadClipboard = func() string {
		t.Fatal("a found selection still read the clipboard")
		return ""
	}
	a.toggleQR()
	if a.qr.input != "https://selected.example" {
		t.Fatalf("input = %q", a.qr.input)
	}
	// No selection: a link on the clipboard prefills, anything else does not.
	for _, c := range []struct{ clip, want string }{
		{"https://clipboard.example/x", "https://clipboard.example/x"},
		{"just a sentence", ""},
	} {
		a = qrTestApp(t)
		a.selection = &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "  "}
		clip := c.clip
		a.qrReadClipboard = func() string { return clip }
		a.toggleQR()
		if a.qr.input != c.want {
			t.Fatalf("clipboard %q: input = %q, want %q", clip, a.qr.input, c.want)
		}
	}
}

// TestQRSelectionDeniedOnce checks the module's own marker: a denied read shows
// the hint once per launch, never opens a system window by itself, and never
// writes the translate state.
func TestQRSelectionDeniedOnce(t *testing.T) {
	a := qrTestApp(t)
	opens := 0
	a.openAccessibilitySettings = func() { opens++ }
	a.selection = &fakeSelectionReader{pid: otherSelectionPID()}
	a.toggleQR()
	if !a.qr.denied || !a.qrSelectionHintShown || a.qr.input != "" {
		t.Fatalf("denied read did not show the hint: %+v", a.qr)
	}
	if a.translateSelectionHintShown {
		t.Fatal("the QR hint used the translate marker")
	}
	if opens != 0 {
		t.Fatal("a denied read opened System Settings by itself")
	}
	a.closeQR()
	a.toggleQR()
	if a.qr.denied {
		t.Fatal("the hint repeated in the same launch")
	}
	a.qr.denied = true
	a.grantAccessibility()
	if a.qr.denied || opens != 1 {
		t.Fatalf("grant = denied %v opens %d", a.qr.denied, opens)
	}
	a.qr.denied = true
	a.grantAccessibility()
	if opens != 1 {
		t.Fatalf("System Settings opened %d times in one launch", opens)
	}
}

// TestQRSelectionBusyReadOpensEmpty covers the concurrent-read boundary: while
// another read owns the single flight, a QR press opens an empty window instead
// of falling back to the clipboard.
func TestQRSelectionBusyReadOpensEmpty(t *testing.T) {
	a := qrTestApp(t)
	a.selection = &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "https://selected.example"}
	a.qrReadClipboard = func() string {
		t.Fatal("busy read accessed clipboard")
		return ""
	}
	a.selectionReading.Store(true)
	defer a.selectionReading.Store(false)
	a.toggleQR()
	if a.qr.input != "" {
		t.Fatalf("a busy read prefilled %q", a.qr.input)
	}
}

func TestQRToolWindowOptions(t *testing.T) {
	a := qrTestApp(t)
	for _, dark := range []bool{false, true} {
		for _, locale := range []string{"zh-CN", "en"} {
			tk := tokens(dark, locale)
			got := a.qrWindowOptions(tk, locale)
			json := a.jsonWindowOptions(&jsonDocument{}, tk, locale)
			if got.Panel || got.Frameless || got.Transparent || got.DisableResize || got.DisableMove || got.DisableClose || got.AlwaysOnTop {
				t.Fatalf("unexpected tool window flags: %+v", got)
			}
			if got.TitleBarStyle != json.TitleBarStyle || *got.TrafficLightPosition != *json.TrafficLightPosition || got.DisableFullScreen != json.DisableFullScreen || got.BackgroundColor != json.BackgroundColor || got.DisableMinimize != json.DisableMinimize || got.DisableMaximize != json.DisableMaximize || got.Hidden != json.Hidden || got.Page.DevTools != json.Page.DevTools {
				t.Fatal("QR and JSON window chrome differ")
			}
			if got.Content == nil || got.Width != 480 || got.Height != 520 || got.MinWidth != 480 || got.MinHeight != 440 {
				t.Fatalf("unexpected content or geometry: %+v", got)
			}
		}
	}
}

func TestQRWindowGeometryRoundTrip(t *testing.T) {
	a, windows := qrWindowTestApp(t)
	screens := paletteTestScreens(a)
	screens.cursor = mygo.Point{X: -600, Y: 300}
	a.openQRText("")
	first := (*windows)[0]
	if first.bounds.X >= 0 || a.qr.place.display.ID != 2 {
		t.Fatalf("first window missed the pointer display: %+v", first.bounds)
	}
	first.drag(-1500, 250)
	first.SetContentSize(620, 700)
	a.closeQR()
	again := filepath.Join(t.TempDir(), "again.toml")
	if err := saveConfig(again, a.config); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.config.QRWindows["2"]; !ok {
		t.Fatalf("no seat stored: %+v", a.config.QRWindows)
	}
	a.openQRText("")
	second := (*windows)[1]
	if second.bounds.X != -1500 || second.bounds.Y != 250 {
		t.Fatalf("second window did not return to its seat: %+v", second.bounds)
	}
	if second.bounds.Width != 620 || second.bounds.Height != 700 {
		t.Fatalf("second window lost its size: %+v", second.bounds)
	}
}

func TestQRDebounce(t *testing.T) {
	a := qrTestApp(t)
	encodes := 0
	a.qrEncodeFn = func(text string) (*qrlib.Code, error) {
		encodes++
		return qrlib.Encode(text)
	}
	a.openQRText("first")
	a.qr.input = "second"
	a.syncQRInput()
	a.qr.input = "third"
	a.syncQRInput()
	tester := qrWindowTester(a, "en", false)
	// The debounce has not expired: no encode yet, and a frame is requested.
	tester.Frame()
	if encodes != 0 {
		t.Fatalf("encoded %d times before the debounce expired", encodes)
	}
	tester.Advance(qrDebounce + 10*time.Millisecond)
	tester.Frame()
	if encodes != 1 {
		t.Fatalf("encoded %d times, want one", encodes)
	}
	if a.qr.encoded != "third" || a.qr.phase != qrReady {
		t.Fatalf("encoded %q in phase %v", a.qr.encoded, a.qr.phase)
	}
}

// TestQRStaleEncodeDropped keeps a slow encode of an older text from replacing
// a newer symbol.
func TestQRStaleEncodeDropped(t *testing.T) {
	a := qrTestApp(t)
	a.qrAsync = false
	a.qrEncodeFn = qrlib.Encode
	a.openQRText("old")
	a.qr.due = time.Now().Add(-time.Second)
	tester := qrWindowTester(a, "en", false)
	tester.Frame()
	if a.qr.encoded != "old" {
		t.Fatalf("encoded %q", a.qr.encoded)
	}
	var queued []func()
	a.qrRunOnMain = func(fn func()) { queued = append(queued, fn) }
	a.openQRText("new")
	a.encodeQR("new")
	a.qr.input = "newer"
	a.syncQRInput()
	if a.qr.encoded != "" {
		t.Fatal("an open reset the encoded text")
	}
	for _, fn := range queued {
		fn()
	}
	if a.qr.encoded != "" {
		t.Fatalf("stale result replaced the symbol: %q", a.qr.encoded)
	}
	a.cancelQREncode()
	for _, fn := range queued {
		fn()
	}
	if a.qr.encoded != "" {
		t.Fatal("a cancelled encode still applied")
	}
}

func TestQRModuleSwitch(t *testing.T) {
	a, windows := qrWindowTestApp(t)
	registered := map[string]bool{}
	a.registerShortcut = func(acc string, _ func()) error { registered[acc] = true; return nil }
	a.unregisterShortcut = func(acc string) { delete(registered, acc) }
	a.registerQRShortcut()
	if !registered[a.config.QRShortcut] {
		t.Fatal("QR shortcut did not register")
	}
	a.openQRText("https://example.com")
	a.changeQREnabled(false)
	if a.config.QREnabled || registered[a.config.QRShortcut] || a.ownedShortcut("qr") != "" {
		t.Fatal("disabling kept the flag or the shortcut")
	}
	if a.qr != nil || !(*windows)[0].destroyed {
		t.Fatal("disabling kept the window")
	}
	if findCommand(a.commands(), "qr.open") != nil {
		t.Fatal("the command is still available")
	}
	if a.menubarPanelRows().qr {
		t.Fatal("the card is still drawn")
	}
	if result := a.applyCLI(cliQRRequest("https://example.com"), []byte("https://example.com")); result.Code != "disabled" {
		t.Fatalf("CLI answer = %+v", result)
	}
	a.changeQREnabled(true)
	if !a.config.QREnabled || !registered[a.config.QRShortcut] {
		t.Fatal("re-enabling did not restore the shortcut")
	}
	a.changeQREnabled(true)
	if len(registered) != 1 {
		t.Fatalf("a redundant switch registered twice: %v", registered)
	}
}

func TestQRShortcutRecording(t *testing.T) {
	a := qrTestApp(t)
	before := []string{a.config.AwakeShortcut, a.config.JSONShortcut, a.config.PaletteShortcut, a.config.PanelShortcut, a.config.TranslateShortcut}
	a.beginTileShortcutRecording("qr")
	if !a.shortcutRecording || a.shortcutTarget != "qr" {
		t.Fatal("QR recorder did not start")
	}
	a.recordShortcut(ui.Ctrl|ui.Alt|ui.Cmd, ui.KeyY)
	if a.config.QRShortcut != "Ctrl+Alt+Cmd+Y" {
		t.Fatalf("QR shortcut = %q", a.config.QRShortcut)
	}
	after := []string{a.config.AwakeShortcut, a.config.JSONShortcut, a.config.PaletteShortcut, a.config.PanelShortcut, a.config.TranslateShortcut}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("recording QR overwrote slot %d: %q -> %q", i, before[i], after[i])
		}
	}
	a.config.QREnabled = false
	if a.shortcutSlotEnabled("qr") {
		t.Fatal("a released slot still records")
	}
}

func TestQRConfigDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	config, err := loadConfig(path)
	if err != nil || !config.QREnabled || config.QRShortcut != "Ctrl+Alt+Cmd+R" {
		t.Fatalf("defaults = %+v, %v", config, err)
	}
	config.QREnabled = false
	config.QRShortcut = "Ctrl+Alt+Cmd+Y"
	if err := saveConfig(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil || loaded.QREnabled || loaded.QRShortcut != "Ctrl+Alt+Cmd+Y" {
		t.Fatalf("saved = %+v, %v", loaded, err)
	}
	// An empty shortcut reads back as the default.
	legacy := "version = 1\nlanguage = \"zh-CN\"\n"
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err = loadConfig(path)
	if err != nil || loaded.QRShortcut != "Ctrl+Alt+Cmd+R" {
		t.Fatalf("legacy shortcut = %q, %v", loaded.QRShortcut, err)
	}
}

func TestSettingsQRPage(t *testing.T) {
	a := qrTestApp(t)
	tk := tokens(false, "zh-CN")
	if got := qrPageHeight(tk); got != 144 {
		t.Fatalf("QR page height = %v, want 144", got)
	}
	if got := pageHeight(tk, settingsQR, true, false, false, false, false); got != 144 {
		t.Fatalf("pageHeight = %v", got)
	}
	if len(settingsNav) != 8 {
		t.Fatalf("nav has %d entries", len(settingsNav))
	}
	want := []int{settingsGeneral, settingsAwake, settingsJSON, settingsTranslate, settingsQR, settingsDevtools, settingsShortcuts, settingsAbout}
	for i, page := range want {
		if settingsNav[i].page != page {
			t.Fatalf("nav %d = %d, want %d", i, settingsNav[i].page, page)
		}
	}
	if settingsGeneral != 0 || settingsAbout != 1 || settingsAwake != 2 || settingsShortcuts != 3 || settingsJSON != 4 || settingsTranslate != 5 {
		t.Fatal("existing page numbers moved")
	}
	if got := settingsPageName(settingsQR); got != "qr" {
		t.Fatalf("page name = %q", got)
	}
	if got, ok := settingsPageIndex("qr", a); !ok || got != settingsQR {
		t.Fatalf("CLI page index = %d %v", got, ok)
	}
	if request, ok := parsePaletteURL("cubbix://settings?page=qr", true, true, 0); !ok || request.Page != settingsQR {
		t.Fatalf("URL page = %+v %v", request, ok)
	}
	if w, h := settingsMinimumSize(); w != 720 || h != settingsWindowHeight {
		t.Fatalf("minimum window = %dx%d; want 720x%d", w, h, settingsWindowHeight)
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			a.page = settingsQR
			tester := ui.NewTester(a.view, 720, settingsWindowHeight)
			tester.SetLocale(locale)
			tester.SetDark(dark)
			tester.Frame()
			for _, key := range []string{"QR", "QRPageDesc", "QRShortcutLabel"} {
				if !tester.HasText(a.text(locale, key)) {
					t.Fatalf("%s dark=%v: missing %s: %v", locale, dark, key, tester.Texts())
				}
			}
			if tester.HasText(a.text(locale, "GroupSettings")) {
				t.Fatalf("%s: the QR page grew a settings group", locale)
			}
		}
	}
}

func TestCLIQRResult(t *testing.T) {
	a := qrTestApp(t)
	request := cliQRRequest("https://example.com/orders/2026")
	body := []byte("https://example.com/orders/2026")
	if code := request.Validate(4096); code != "" {
		t.Fatalf("Validate = %q", code)
	}
	result := a.applyCLI(request, body)
	if !result.OK || result.QR == nil {
		t.Fatalf("result = %+v", result)
	}
	if result.Bytes != 0 {
		t.Fatalf("top-level bytes = %d; the length belongs to qr.bytes", result.Bytes)
	}
	if result.QR.Bytes != len(body) || result.QR.Version == 0 || result.QR.Modules == 0 || result.QR.ECC != "M" {
		t.Fatalf("qr = %+v", result.QR)
	}
	// The metadata travels through Response.Result(), and the line stays far
	// below MaxLine.
	response := cliResponse(result)
	if got := response.Result(); got.QR == nil || got.QR.Modules != result.QR.Modules {
		t.Fatalf("Result() lost the QR field: %+v", got)
	}
	if _, err := encodeResultLine(result); err != nil {
		t.Fatal(err)
	}
	// A text that does not fit is rejected, never cut.
	long := strings.Repeat("a", qrlib.MaxBytes+1)
	failure := a.applyCLI(cliQRRequest(long), []byte(long))
	if failure.OK || failure.Code != "QRTooLong" || failure.QR != nil {
		t.Fatalf("too long = %+v", failure)
	}
	if failure.Message == "" {
		t.Fatal("the too-long answer carries no sentence")
	}
	// --notify is a usage error: a QR code never shows a hint.
	notify := cliQRRequest("hello")
	notify.Notify = true
	if code := notify.Validate(4096); code == "" {
		t.Fatal("--notify was accepted")
	}
}

func TestQRViewStates(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			for _, state := range []string{"empty", "ready", "toolong", "failed", "pinned"} {
				a := qrTestApp(t)
				a.openQRText("")
				switch state {
				case "ready":
					a.openQRText("https://example.com/orders/2026")
					a.qr.due = time.Now().Add(-time.Second)
				case "toolong":
					a.openQRText(strings.Repeat("a", qrlib.MaxBytes+1))
					a.qr.due = time.Now().Add(-time.Second)
				case "failed":
					a.qr.input = "x"
					a.qr.phase = qrFailed
					a.qr.errorKey = "QRFailed"
				case "pinned":
					a.openQRText("https://example.com/orders/2026")
					a.qr.due = time.Now().Add(-time.Second)
					a.qr.pinned = true
				}
				tester := qrWindowTester(a, locale, dark)
				tester.Frame()
				tk := tokens(dark, locale)
				// The toolbar's three buttons and the symbol box stay inside
				// the 480-wide window in every state.
				pin := a.text(locale, "R07Pin")
				if state == "pinned" {
					pin = a.text(locale, "R07Unpin")
				}
				for _, label := range []string{pin, a.text(locale, "QRCopy"), a.text(locale, "QRSave")} {
					rect, ok := tester.Find(label)
					if !ok || rect.X < -0.5 || rect.X+rect.W > tk.SizeQRWidth+0.5 {
						t.Fatalf("%s %v %s: %q is outside the window: %+v %v", locale, dark, state, label, rect, ok)
					}
				}
				box := a.qr.boxBounds
				if box.W <= 0 || box.X < -0.5 || box.X+box.W > tk.SizeQRWidth+0.5 || box.Y+box.H > 520.5 {
					t.Fatalf("%s %v %s: symbol box = %+v", locale, dark, state, box)
				}
				if state == "empty" && !tester.HasText(a.text(locale, "QREmpty")) {
					t.Fatalf("%s %v: empty state lost its placeholder: %v", locale, dark, tester.Texts())
				}
				if state == "toolong" && !tester.HasText(a.text(locale, "QRTooLong")) {
					t.Fatalf("%s %v: too-long state lost its message: %v", locale, dark, tester.Texts())
				}
				if state == "failed" && !tester.HasText(a.text(locale, "QRFailed")) {
					t.Fatalf("%s %v: failed state lost its message: %v", locale, dark, tester.Texts())
				}
			}
		}
	}
}

func TestQRDarkKeepsBlackOnWhite(t *testing.T) {
	a := qrTestApp(t)
	a.openQRText("https://example.com/orders/2026")
	a.qr.due = time.Now().Add(-time.Second)
	tester := qrWindowTester(a, "en", true)
	tester.Frame()
	if a.qr.bitmap == nil {
		t.Fatal("no bitmap")
	}
	box := a.qr.boxBounds
	if box.W <= 0 {
		t.Fatal("no symbol box")
	}
	// The top centre of the box is quiet zone: white, whatever the theme.
	img := tester.Image()
	pixel := img.RGBAAt(int((box.X+box.W/2)*2), int((box.Y+4)*2))
	if pixel.R < 0xF0 || pixel.G < 0xF0 || pixel.B < 0xF0 {
		t.Fatalf("dark theme symbol background = %v", pixel)
	}
}

func qrWindowTester(a *application, locale string, dark bool) *ui.Tester {
	tester := ui.NewTester(a.qrView, 480, 520)
	tester.SetScale(2)
	tester.SetLocale(locale)
	tester.SetDark(dark)
	return tester
}

func qrWindowTestApp(t *testing.T) (*application, *[]*fakeQRWindow) {
	t.Helper()
	a := qrTestApp(t)
	windows := new([]*fakeQRWindow)
	a.createQRWindow = func(state *qrState) {
		window := &fakeQRWindow{}
		state.testFrame = window
		*windows = append(*windows, window)
	}
	return a, windows
}

type fakeQRWindow struct {
	fakeFrame
	destroyed bool
	pins      []bool
}

func (f *fakeQRWindow) SetContentSize(width, height int) {
	f.bounds.Width, f.bounds.Height = width, height
}

func (f *fakeQRWindow) SetAlwaysOnTop(pinned bool) { f.pins = append(f.pins, pinned) }
func (f *fakeQRWindow) Destroy()                   { f.destroyed, f.visible = true, false }

// cliQRRequest is a `cubbix qr` request carrying text in a body frame.
func cliQRRequest(text string) cli.Request {
	return cli.Request{V: cli.Protocol, ID: "qr1", Op: cli.OpRun, Action: cli.ActionQR, Bytes: len(text)}
}

// cliResponse is the wire response of a result, as cli_server.go builds it.
func cliResponse(result cli.Result) cli.Response {
	return cli.Response{V: cli.Protocol, ID: "qr1", OK: result.OK, Code: result.Code,
		Message: result.Message, Lines: result.Lines, State: result.State,
		Bytes: result.Bytes, Translate: result.Translate, QR: result.QR}
}

func encodeResultLine(result cli.Result) ([]byte, error) {
	return cli.EncodeLine(result)
}

// TestQRToolWindowsDoNotCloseEachOther pins the mutual rule: opening the QR
// window closes the overlays, and the overlays never close a tool window.
func TestQRToolWindowsDoNotCloseEachOther(t *testing.T) {
	a, windows := qrWindowTestApp(t)
	a.createPaletteWindow = func(*paletteState) {}
	a.openQRText("https://example.com")
	if len(*windows) != 1 || !a.qrVisible() {
		t.Fatal("QR window did not open")
	}
	a.showPalette()
	if !a.qrVisible() {
		t.Fatal("the palette closed the QR window")
	}
	a.showMenubarPanel()
	if !a.qrVisible() {
		t.Fatal("the card panel closed the QR window")
	}
	a.hideMenubarPanel()
	// Opening QR closes the overlays instead.
	a.showPalette()
	a.openQRText("https://example.com/2")
	if a.paletteVisible() || a.menubarPanelVisible() {
		t.Fatal("opening QR left an overlay on screen")
	}
}

// TestQRPanelCardAndCommand checks the two remaining entries: the card row
// opens an empty window without reading the clipboard, and the command carries
// its text with the original case.
func TestQRPanelCardAndCommand(t *testing.T) {
	a := qrTestApp(t)
	a.config.QREnabled = true
	a.config.AwakeEnabled = false
	a.config.JSONEnabled = false
	a.config.TranslateEnabled = false
	rows := a.menubarPanelRows()
	if !rows.qr || rows.empty {
		t.Fatalf("rows = %+v", rows)
	}
	a.panelOpenQR()
	if a.qr == nil || a.qr.input != "" {
		t.Fatal("the card read the clipboard or left no window")
	}
	command := findCommand(a.commands(), "qr.open")
	if command == nil || command.Param != paramText {
		t.Fatal("missing the qr.open command")
	}
	if err := command.Run(a, request{Command: command, Arg: "https://Example.com/Path"}); err != nil {
		t.Fatal(err)
	}
	if a.qr.input != "https://Example.com/Path" {
		t.Fatalf("command text = %q", a.qr.input)
	}
	a.config.QREnabled = false
	if findCommand(a.commands(), "qr.open") != nil {
		t.Fatal("a closed module still offers the command")
	}
	if a.menubarPanelRows().qr {
		t.Fatal("a closed module still draws the card")
	}
	if !a.menubarPanelRows().empty {
		t.Fatal("all modules disabled without empty state")
	}
}

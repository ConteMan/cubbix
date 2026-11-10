package main

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	qrlib "github.com/ConteMan/cubbix/internal/qr"
)

const (
	// qrDebounce is how long typing settles before the symbol is rebuilt.
	qrDebounce = 200 * time.Millisecond
	// qrMaxClipboardBytes bounds what the clipboard may prefill. A longer
	// text is usually not a link but a document, and a document is not a
	// reason to open a QR code.
	qrMaxClipboardBytes = 2048
)

type qrPhase int

const (
	qrEmpty qrPhase = iota
	qrReady
	qrTooLong
	qrFailed
)

// qrState is the tool window's own state. It lives on the main thread, like
// translateState and jsonDocument, and keeps nothing between launches.
type qrState struct {
	window    *mygo.Window
	testFrame qrWindow
	place     windowPlaceState
	// initialSize is the frame size the window opened with, so a resize is
	// told apart from an automatic placement.
	initialSize screenSize
	pinned      bool
	visible     bool
	closing     bool
	focus       bool
	// input is what the field holds, encoded the text the symbol was built
	// from, and code that symbol. A code outliving its text is impossible.
	input   string
	encoded string
	code    *qrlib.Code
	phase   qrPhase
	// errorKey names the failure the interface shows, never a library error.
	errorKey string
	bitmap   *ui.Bitmap
	// px is the side of one module in device pixels and scale the factor of
	// the display the window is on: together they make the bitmap's DIP size.
	px    int
	scale float64
	box   float32
	// boxBounds is the symbol box of the last frame, in window DIPs. A view
	// cannot read its element once the frame is over, and the tests assert
	// what was really drawn.
	boxBounds ui.Rect
	// due is the debounce deadline; the zero value means nothing is pending.
	due         time.Time
	generation  uint64
	copiedUntil time.Time
	saveFailed  bool
	// denied is the per-launch accessibility hint under the input. It is an
	// overlay, not a phase: typing anything dismisses it.
	denied bool
}

func (a *application) ensureQRState() *qrState {
	if a.qr == nil {
		a.qr = &qrState{scale: 1}
	}
	return a.qr
}

// qrOnMainThread applies worker results to the UI. Tests replace the seam so
// no case needs a running application loop.
func (a *application) qrOnMainThread(fn func()) {
	if a.qrRunOnMain != nil {
		a.qrRunOnMain(fn)
		return
	}
	fn()
}

func (a *application) runQRWorker(fn func()) {
	if a.qrAsync {
		go fn()
		return
	}
	fn()
}

func (a *application) qrEncodeFunction() func(string) (*qrlib.Code, error) {
	if a.qrEncodeFn != nil {
		return a.qrEncodeFn
	}
	return qrlib.Encode
}

func (a *application) registerQRShortcut() {
	a.registerSlot("qr", a.config.QRShortcut, a.config.QREnabled, a.toggleQR)
}

// changeQREnabled follows changeTranslateEnabled step for step: the flag is
// written first, an opening registers its slot, and only a close releases the
// combination, clears its diagnostic and destroys the window.
func (a *application) changeQREnabled(target bool) {
	a.cancelShortcutRecording()
	if target == a.config.QREnabled {
		a.invalidateWindow()
		return
	}
	a.config.QREnabled = target
	if target {
		a.registerQRShortcut()
	} else {
		a.releaseOwnedShortcut("qr")
		a.clearSlotError("qr")
		a.closeQR()
	}
	a.save()
	a.invalidateWindow()
	// The card panel drops or restores its row at once, as the JSON module's
	// close does; an open panel is the only surface a module switch can leave
	// stale.
	a.refreshMenubarPanel()
	a.invalidatePaletteAfterModuleChange()
}

func (a *application) qrVisible() bool {
	return a.qr != nil && a.qr.visible
}

// toggleQR is the global shortcut. The AX read runs on a worker, so a stuck
// application can never hold the shortcut callback.
func (a *application) toggleQR() {
	if a.shortcutRecording || !a.config.QREnabled {
		return
	}
	a.beginQRSelection()
}

// openQRText is the one UI entry: the shortcut, the card panel and the command
// palette all end here. It opens no other tool window and closes none.
func (a *application) openQRText(text string) {
	if !a.config.QREnabled {
		return
	}
	a.cancelQRSelection()
	a.hideMenubarPanel()
	a.hidePalette()
	s := a.ensureQRState()
	a.cancelQREncode()
	s.input = text
	s.encoded = ""
	s.code = nil
	s.bitmap = nil
	s.px = 0
	s.phase = qrEmpty
	s.errorKey = ""
	s.copiedUntil = time.Time{}
	s.saveFailed = false
	s.denied = false
	s.focus = true
	s.due = time.Time{}
	if s.frame() == nil && a.createQRWindow != nil {
		a.createQRWindow(s)
		a.placeQR()
	}
	a.showQR()
	if strings.TrimSpace(text) != "" {
		a.scheduleQR()
	} else {
		a.refreshQR()
	}
}

func (a *application) showQR() {
	s := a.qr
	if s == nil {
		return
	}
	if window := s.frame(); window != nil {
		if window.IsMinimized() {
			window.Restore()
		}
		a.reconcileQRDisplay()
		a.visibility(true)
		s.visible = true
		window.Show()
		window.Focus()
	}
	s.visible = true
	a.refreshQRScale()
	if s.window != nil {
		s.window.Invalidate()
	}
}

func (a *application) closeQR() {
	a.cancelQRSelection()
	s := a.qr
	if s == nil || s.closing {
		return
	}
	s.closing = true
	if a.rememberQRPlace() {
		a.save()
	}
	a.cancelQREncode()
	s.visible = false
	if window := s.frame(); window != nil {
		window.Destroy()
	}
	if a.qr == s {
		a.qr = nil
	}
	a.visibility(false)
}

// cancelQREncode drops a pending debounce and every in-flight encode: a late
// result sees a generation it no longer owns.
func (a *application) cancelQREncode() {
	s := a.qr
	if s == nil {
		return
	}
	s.due = time.Time{}
	s.generation++
}

func (a *application) refreshQR() {
	if s := a.qr; s != nil && s.window != nil {
		s.window.Invalidate()
	}
}

// syncQRInput reads what the user typed and schedules the rebuild. Typing is
// also what dismisses the accessibility hint.
func (a *application) syncQRInput() {
	s := a.qr
	if s == nil {
		return
	}
	s.denied = false
	s.saveFailed = false
	if strings.TrimSpace(s.input) == "" {
		a.cancelQREncode()
		s.code = nil
		s.bitmap = nil
		s.encoded = ""
		s.px = 0
		s.phase = qrEmpty
		s.errorKey = ""
		a.refreshQR()
		return
	}
	a.scheduleQR()
}

// scheduleQR arms the debounce. The next frame waits for the deadline, and the
// one that arrives after it starts the encode.
func (a *application) scheduleQR() {
	s := a.qr
	if s == nil {
		return
	}
	a.cancelQREncode()
	s.due = time.Now().Add(qrDebounce)
	a.refreshQRBitmap()
	a.refreshQR()
}

// encodeQR builds the symbol on a worker and applies it on the main thread.
// Every result carries the generation it was started with, so a slow encode of
// an older text can never replace a newer one.
func (a *application) encodeQR(text string) {
	s := a.qr
	if s == nil {
		return
	}
	s.generation++
	generation := s.generation
	encode := a.qrEncodeFunction()
	a.runQRWorker(func() {
		code, err := encode(text)
		a.qrOnMainThread(func() {
			if a.qr != s || generation != s.generation || s.input != text || s.closing || a.quitting {
				return
			}
			if err != nil {
				s.code = nil
				s.bitmap = nil
				s.encoded = ""
				s.px = 0
				if errors.Is(err, qrlib.ErrTooLong) {
					s.phase = qrTooLong
					s.errorKey = ""
				} else {
					s.phase = qrFailed
					s.errorKey = "QRFailed"
				}
				a.refreshQR()
				return
			}
			s.code = code
			s.encoded = text
			s.phase = qrReady
			s.errorKey = ""
			a.refreshQRBitmap()
			a.refreshQR()
		})
	})
}

// refreshQRScale reads the scale factor of the display the window is on. It is
// called when the window opens, moves or the displays change, never from a
// frame: a frame must not wait for the screen.
func (a *application) refreshQRScale() {
	s := a.qr
	if s == nil {
		return
	}
	scale := a.qrDisplayScaleFactor()
	if scale <= 0 {
		scale = 1
	}
	s.scale = scale
	a.refreshQRBitmap()
	a.refreshQR()
}

// refreshQRBitmap rebuilds the bitmap from the symbol already encoded, at whole
// device pixels per module for the current box and display scale. A screen or
// size change therefore needs no second encode.
func (a *application) refreshQRBitmap() {
	s := a.qr
	if s == nil || s.code == nil {
		return
	}
	s.box = a.qrBoxSide()
	s.px = qrDisplayPixels(s.box, float32(s.code.Modules()), float32(s.scale))
	img, err := s.code.Image(s.px)
	if err != nil {
		s.bitmap = nil
		s.phase = qrFailed
		s.errorKey = "QRFailed"
		return
	}
	s.bitmap = ui.NewBitmap(img).Pixelated()
}

// qrBoxSide is the side of the white square in DIPs: the smaller of the room
// the window has and SizeQRCodeMax.
func (a *application) qrBoxSide() float32 {
	s := a.qr
	if s == nil {
		return 0
	}
	t := tokens(false, "zh-CN")
	width, height := int(t.SizeQRWidth), int(t.SizeQRWindowHeight)
	if frame := s.frame(); frame != nil {
		width, height = windowSize(frame).Width, windowSize(frame).Height
	}
	return qrBoxSide(t, width, height, s.denied, s.saveFailed)
}

// qrBoxSide is the geometry rule of spec 011 §4.1, free of window state: the
// square takes the smaller of the window's room and SizeQRCodeMax, and the
// extra rows a failure adds are taken out of the room first.
func qrBoxSide(t tokenValues, width, height int, denied, saveFailed bool) float32 {
	chrome := t.SizeJSONToolbarHeight + t.StrokeHairline + 2*t.Space12 + t.SizeQRInputHeight + t.Space8
	if denied {
		chrome += t.SizeSegmentHeight + t.Space8
	}
	if saveFailed {
		chrome += t.FontSizeSmall*t.LineHeightBody + t.Space6 + t.Space8
	}
	box := min(t.SizeQRCodeMax, min(float32(width)-2*t.Space12, float32(height)-chrome))
	return max(box, t.Space40)
}

// qrDisplayPixels is how many device pixels one module takes. The symbol's
// side in modules includes the quiet zone, so the whole bitmap is
// (modules+8)·px device pixels, shown as that many pixels divided by the
// display scale. Whole pixels per module keep every module edge on a pixel
// boundary; a fractional multiple would blur them.
func qrDisplayPixels(box, modules, scaleFactor float32) int {
	if modules <= 0 || scaleFactor <= 0 {
		return 1
	}
	side := modules + 2*qrlib.QuietZone
	return max(1, int(float32(box)*scaleFactor/side))
}

// qrBitmapSide is the bitmap's side in device pixels.
func qrBitmapSide(modules, px int) int { return (modules + 2*qrlib.QuietZone) * px }

// qrPrefill is the clipboard rule: only an http or https address, at most
// qrMaxClipboardBytes long, prefills the field. Anything else leaves it empty,
// so an unrelated sentence never becomes a QR code.
func (a *application) qrPrefill() string {
	if a.qrReadClipboard == nil {
		return ""
	}
	text := strings.TrimSpace(a.qrReadClipboard())
	if text == "" || len(text) > qrMaxClipboardBytes {
		return ""
	}
	lower := strings.ToLower(text)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return ""
	}
	parsed, err := url.Parse(text)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return text
}

// beginQRSelection is the shortcut path: the AX read runs on a worker with its
// own generation, so a second press cancels a pending read instead of queueing
// another window.
func (a *application) beginQRSelection() {
	a.hideMenubarPanel()
	a.hidePalette()
	if a.qrSelectionPending {
		a.qrSelectionGeneration++
		a.qrSelectionPending = false
		a.openQRText("")
		return
	}
	a.qrSelectionGeneration++
	generation := a.qrSelectionGeneration
	a.qrSelectionPending = true
	a.runQRWorker(func() {
		answer := a.captureSelection()
		a.qrOnMainThread(func() {
			if generation != a.qrSelectionGeneration || !a.config.QREnabled || a.quitting {
				return
			}
			a.qrSelectionPending = false
			switch {
			case answer.status == selectionFound:
				a.openQRText(answer.text)
			case answer.status == selectionDenied:
				a.openQRText("")
				a.showQRSelectionHintOnce()
			case answer.busy:
				// Another read owns the single flight. That is not "nothing is
				// selected", so the clipboard must not stand in for it: the
				// window opens empty and the next press reads again.
				a.openQRText("")
			default:
				a.openQRText(a.qrPrefill())
			}
		})
	})
}

func (a *application) cancelQRSelection() {
	a.qrSelectionGeneration++
	a.qrSelectionPending = false
}

// showQRSelectionHintOnce is the QR module's own per-launch marker: it never
// reuses the translate hint, and it never opens a system window by itself.
func (a *application) showQRSelectionHintOnce() {
	if a.qrSelectionHintShown {
		return
	}
	a.qrSelectionHintShown = true
	s := a.ensureQRState()
	s.denied = true
	a.refreshQRScale()
	a.refreshQR()
}

// qrPNG renders the export image: 16 pixels per module with the standard quiet
// zone, whatever the window shows.
func (a *application) qrPNG() ([]byte, error) {
	s := a.qr
	if s == nil || s.code == nil || s.encoded != s.input {
		return nil, errors.New("qr: nothing encoded")
	}
	return s.code.PNG(qrlib.ExportModule)
}

func (a *application) copyQRImage() {
	s := a.qr
	if s == nil || s.code == nil || a.qrCopyPNG == nil {
		return
	}
	png, err := a.qrPNG()
	if err != nil {
		return
	}
	if err := a.qrCopyPNG(png); err != nil {
		return
	}
	s.copiedUntil = time.Now().Add(1500 * time.Millisecond)
	a.refreshQR()
}

// saveQRImage asks for a path and writes the PNG on a worker. A cancelled
// dialog writes nothing and reports nothing.
func (a *application) saveQRImage() {
	s := a.qr
	if s == nil || s.code == nil || s.encoded != s.input || a.qrChooseSave == nil {
		return
	}
	window := s.window
	code := s.code
	s.saveFailed = false
	a.refreshQRBitmap()
	a.refreshQR()
	go func() {
		path, err := a.qrChooseSave(window)
		if err == nil && path == "" {
			return
		}
		if err == nil {
			var png []byte
			png, err = code.PNG(qrlib.ExportModule)
			if err == nil {
				err = a.qrWriteFile(path, png)
			}
		}
		a.qrOnMainThread(func() {
			if a.qr != s || s.closing {
				return
			}
			s.saveFailed = err != nil
			a.refreshQRBitmap()
			a.refreshQR()
		})
	}()
}

// qrCopyExpired drops a stale copy marker without a timer: the view asks for a
// frame while the marker shows and clears it later.
func (a *application) qrCopyExpired(now time.Time) bool {
	s := a.qr
	if s == nil || s.copiedUntil.IsZero() {
		return false
	}
	return !now.Before(s.copiedUntil)
}

func (a *application) setQRPinned(pinned bool) {
	if s := a.qr; s != nil {
		s.pinned = pinned
		if window := s.frame(); window != nil {
			window.SetAlwaysOnTop(pinned)
		}
		a.refreshQR()
	}
}

func (a *application) qrWindowOptions(t tokenValues, locale string) mygo.WindowOptions {
	return mygo.WindowOptions{
		Title:                a.text(locale, "QR"),
		Width:                int(t.SizeQRWidth),
		Height:               int(t.SizeQRWindowHeight),
		MinWidth:             int(t.SizeQRWidth),
		MinHeight:            int(t.SizeQRMinHeight),
		Hidden:               true,
		TitleBarStyle:        mygo.TitleBarHidden,
		TrafficLightPosition: &mygo.Point{X: int(t.Space12), Y: int(t.Space16)},
		DisableFullScreen:    true,
		BackgroundColor:      t.ColorCard,
		Page:                 mygo.PageOptions{DevTools: mygo.DevToolsDisabled},
		Content:              ui.View(a.qrView),
	}
}

func (a *application) buildQRWindow(s *qrState) {
	t := tokens(mygo.Theme.IsDark(), mygo.App.Locale())
	window := mygo.NewWindow(a.qrWindowOptions(t, mygo.App.Locale()))
	s.window = window
	s.scale = a.qrDisplayScaleFactor()
	window.OnClose(func(*mygo.CloseEvent) {
		if a.rememberQRPlace() {
			a.save()
		}
		a.cancelQRSelection()
		a.cancelQREncode()
		s.visible = false
	})
	window.OnClosed(func() {
		if a.qr == s {
			a.qr = nil
		}
		a.visibility(false)
	})
	window.OnHide(func() { s.visible = false; a.visibility(false) })
	window.OnMove(func() {
		a.observeQRMove()
		a.refreshQRScale()
	})
	window.OnRestore(a.reconcileQRDisplay)
	window.OnResize(func() {
		a.refreshQRBitmap()
		a.refreshQR()
	})
}

// copyQRImageData is the clipboard write of the PNG, kept apart so the seam has
// one shape.
func copyQRImageData(png []byte) error {
	return mygo.Clipboard.Write(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, png))))
}

// writeQRFile writes the exported PNG beside its destination and renames it
// into place, as the JSON documents do: a failed write never leaves half an
// image behind and never truncates a file the user already had.
func writeQRFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".cubbix-qr-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if info, statErr := os.Stat(path); statErr == nil {
		if err := file.Chmod(info.Mode().Perm()); err != nil {
			return err
		}
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

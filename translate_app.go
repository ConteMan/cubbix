package main

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

type translatePhase int

const (
	translateEmpty translatePhase = iota
	translateLoading
	translateStreaming
	translateDone
	translateFailed
	translateNotConfigured
)

// translateState is the floating window's own state. It lives on the main
// thread, like paletteState and menubarPanelState. Dictionary, speech and
// selection belong to later steps; this state is only the translation main
// path.
type translateState struct {
	window           *mygo.Window
	visible          bool
	input            string
	directionInput   string
	phase            translatePhase
	directionSwapped bool
	output           string
	errorKey         string
	request          uint64
	cancel           context.CancelFunc
	clipped          int
	copiedUntil      time.Time
	focus            bool
}

type translateStreamFunc func(context.Context, translateConfig, string, string, string, func(string)) (string, error)
type translateModelsFunc func(context.Context, translateConfig) ([]string, error)
type translateTestFunc func(context.Context, translateConfig) (string, error)

func defaultTranslateCopy(text string) error {
	return mygo.Clipboard.Write(transfer.TextData(text))
}

func (a *application) ensureTranslateState() *translateState {
	if a.translate == nil {
		a.translate = &translateState{}
	}
	return a.translate
}

// translateOnMain applies worker results to the UI. The seam is replaced by
// headless tests so no case needs a running application loop.
func (a *application) translateOnMain(fn func()) {
	if a.translateRunOnMain != nil {
		a.translateRunOnMain(fn)
		return
	}
	fn()
}

func (a *application) runTranslateWorker(fn func()) {
	if a.translateAsync {
		go fn()
		return
	}
	fn()
}

func (a *application) translateStreamFunction() translateStreamFunc {
	if a.translateStreamFn != nil {
		return a.translateStreamFn
	}
	return streamTranslation
}

func (a *application) translateModelsFunction() translateModelsFunc {
	if a.translateModelsFn != nil {
		return a.translateModelsFn
	}
	return fetchTranslateModels
}

func (a *application) translateTestFunction() translateTestFunc {
	if a.translateTestFn != nil {
		return a.translateTestFn
	}
	return testTranslateConnection
}

func (a *application) registerTranslateShortcut() {
	a.registerSlot("translate", a.config.TranslateShortcut, a.config.TranslateEnabled, a.toggleTranslate)
}

// changeTranslateEnabled is the module switch. Turning it off releases only
// the combination this slot actually registered and closes the panel.
func (a *application) changeTranslateEnabled(target bool) {
	a.cancelShortcutRecording()
	if target == a.config.TranslateEnabled {
		a.invalidateWindow()
		return
	}
	a.config.TranslateEnabled = target
	if target {
		a.registerTranslateShortcut()
	} else {
		a.releaseOwnedShortcut("translate")
		a.clearSlotError("translate")
		a.closeTranslate()
	}
	a.save()
	a.invalidateWindow()
	a.invalidatePaletteAfterModuleChange()
}

func (a *application) changeTranslateAutoSpeak(target bool) {
	if a.config.TranslateAutoSpeak == target {
		return
	}
	a.config.TranslateAutoSpeak = target
	a.save()
	a.invalidateWindow()
}

func (a *application) translateVisible() bool {
	return a.translate != nil && a.translate.visible
}

func (a *application) toggleTranslate() {
	if a.shortcutRecording || !a.config.TranslateEnabled {
		return
	}
	if a.translateVisible() {
		a.closeTranslate()
		return
	}
	a.openTranslateText("")
}

// openTranslateText is the one panel entry point. Later steps feed it the
// selected text; this step opens an empty input and focuses it.
func (a *application) openTranslateText(text string) {
	a.hideMenubarPanel()
	a.hidePalette()
	s := a.ensureTranslateState()
	a.cancelTranslate()
	s.input = text
	s.directionInput = text
	s.output = ""
	s.errorKey = ""
	s.clipped = 0
	s.directionSwapped = false
	s.copiedUntil = time.Time{}
	s.phase = translateEmpty
	s.focus = true
	if s.window == nil && a.createTranslateWindow != nil {
		a.createTranslateWindow(s)
	}
	a.showTranslate()
	if strings.TrimSpace(text) != "" {
		a.submitTranslate()
	} else {
		a.refreshTranslate()
	}
}

// showTranslate places and shows the nonactivating panel. It deliberately
// does not call visibility: that would switch the activation policy and pull
// the user out of the application they were reading.
func (a *application) showTranslate() {
	s := a.translate
	if s == nil {
		return
	}
	a.placeTranslate()
	s.visible = true
	if s.window != nil {
		s.window.Show()
		s.window.Invalidate()
	}
}

func (a *application) placeTranslate() {
	s := a.translate
	if s == nil || s.window == nil {
		return
	}
	t := tokens(false, "zh-CN")
	margin := int(t.SizePaletteShadowMargin)
	width := int(t.SizeTranslateWidth) + 2*margin
	height := int(a.translateHeight(t, s)) + 2*margin
	displays := a.paletteDisplaysNow()
	if len(displays) == 0 {
		return
	}
	display := displayNearest(displays, a.paletteCursorPoint())
	x, y := palettePosition(display, width, height, t.SizeTranslateTopOffset)
	s.window.SetPosition(x, y)
}

func (a *application) hideTranslate() {
	s := a.translate
	if s == nil {
		return
	}
	s.visible = false
	if s.window != nil {
		s.window.Hide()
	}
}

func (a *application) closeTranslate() {
	a.cancelTranslate()
	a.hideTranslate()
}

// onTranslateBlur keeps a result on screen for comparing with the source;
// an empty input closes as soon as the panel loses focus.
func (a *application) onTranslateBlur() {
	s := a.translate
	if s == nil {
		return
	}
	hasResult := strings.TrimSpace(s.output) != ""
	if hasResult && strings.TrimSpace(s.input) != "" {
		return
	}
	a.cancelTranslate()
	a.hideTranslate()
}

func (a *application) cancelTranslate() {
	s := a.translate
	if s == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
		s.request++
	}
}

// translateDirection is the auto-detected direction with the one-shot swap
// applied. The row only shows the target; the source stays auto after a swap.
func (a *application) translateDirection(s *translateState) (source, target string) {
	source, target = detectDirection(s.input)
	if s.directionSwapped {
		source = "auto"
		if target == "zh-Hans" {
			target = "en"
		} else {
			target = "zh-Hans"
		}
	}
	return source, target
}

func (a *application) swapTranslateDirection() {
	s := a.translate
	if s == nil || strings.TrimSpace(s.input) == "" {
		return
	}
	a.syncTranslateInput()
	s.directionSwapped = !s.directionSwapped
	if strings.TrimSpace(s.input) != "" {
		a.submitTranslate()
	} else {
		a.refreshTranslate()
	}
}

func (a *application) syncTranslateInput() {
	s := a.translate
	if s == nil || s.input == s.directionInput {
		return
	}
	s.directionInput = s.input
	s.directionSwapped = false
	s.clipped = 0
	a.refreshTranslate()
}

// submitTranslate starts one request. A generation number makes late deltas
// and late completions harmless, and every UI update goes through the main
// thread seam.
func (a *application) submitTranslate() {
	s := a.ensureTranslateState()
	a.syncTranslateInput()
	a.cancelTranslate()
	text := s.input
	if strings.TrimSpace(text) == "" {
		s.phase = translateEmpty
		s.output = ""
		s.errorKey = ""
		s.clipped = 0
		a.refreshTranslate()
		return
	}
	clipped, dropped := clipTranslateInput(text)
	if dropped == 0 {
		dropped = s.clipped
	}
	s.input = clipped
	s.directionInput = clipped
	s.clipped = dropped
	s.output = ""
	s.copiedUntil = time.Time{}
	s.errorKey = ""
	s.phase = translateLoading
	s.request++
	generation := s.request
	source, target := a.translateDirection(s)
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	cfg := translateConfig{BaseURL: a.config.TranslateBaseURL, Model: strings.TrimSpace(a.config.TranslateModel)}
	stream := a.translateStreamFunction()
	a.refreshTranslate()

	apply := func(fn func()) { a.translateOnMain(fn) }
	finish := func(phase translatePhase, key string) {
		apply(func() {
			if generation != s.request {
				return
			}
			s.phase = phase
			s.errorKey = key
			a.refreshTranslate()
		})
	}
	a.runTranslateWorker(func() {
		defer cancel()
		key, err := a.translateAPIKey()
		if err != nil {
			finish(translateFailed, "TranslateKeyError")
			return
		}
		cfg.Key = key
		if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Model) == "" || key == "" {
			finish(translateNotConfigured, "")
			return
		}
		if ctx.Err() != nil {
			return
		}
		out, streamErr := stream(ctx, cfg, clipped, source, target, func(delta string) {
			apply(func() {
				if generation != s.request {
					return
				}
				if s.output == "" {
					s.phase = translateStreaming
				}
				s.output += delta
				a.refreshTranslate()
			})
		})
		if streamErr != nil {
			finish(translateFailed, translateErrorKey(streamErr))
			return
		}
		apply(func() {
			if generation != s.request {
				return
			}
			s.output = out
			s.phase = translateDone
			s.errorKey = ""
			a.refreshTranslate()
		})
	})
}

func (a *application) copyTranslateResult() {
	s := a.translate
	if s == nil || strings.TrimSpace(s.output) == "" || a.translateCopy == nil {
		return
	}
	if err := a.translateCopy(s.output); err != nil {
		return
	}
	s.copiedUntil = time.Now().Add(1500 * time.Millisecond)
	a.refreshTranslate()
}

func (a *application) openTranslateSettings() {
	a.closeTranslate()
	a.selectSettingsPage(settingsTranslate)
	a.show()
}

func (a *application) refreshTranslate() {
	s := a.translate
	if s == nil || s.window == nil {
		return
	}
	t := tokens(false, "zh-CN")
	margin := int(t.SizePaletteShadowMargin)
	s.window.SetContentSize(int(t.SizeTranslateWidth)+2*margin, int(a.translateHeight(t, s))+2*margin)
	s.window.Invalidate()
}

// buildTranslateWindow creates the hidden nonactivating panel. It is created
// on first use, never at launch, and never shown by a test.
func (a *application) buildTranslateWindow(s *translateState) {
	t := tokens(false, "zh-CN")
	margin := int(t.SizePaletteShadowMargin)
	window := mygo.NewWindow(mygo.WindowOptions{
		Title:             "Cubbix",
		Width:             int(t.SizeTranslateWidth) + 2*margin,
		Height:            int(t.SizeTranslateMinHeight) + 2*margin,
		Hidden:            true,
		Frameless:         true,
		Transparent:       true,
		DisableShadow:     true,
		DisableResize:     true,
		DisableMove:       true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		Panel:             true,
		Content:           ui.View(a.translateView),
	})
	window.OnClose(func(event *mygo.CloseEvent) {
		if !a.quitting {
			event.PreventDefault()
			a.closeTranslate()
		}
	})
	window.OnBlur(a.onTranslateBlur)
	s.window = window
}

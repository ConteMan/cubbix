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

type translateState struct {
	window             *mygo.Window
	testFrame          translateWindow
	place              windowPlaceState
	initialSize        screenSize
	pinned             bool
	closing            bool
	visible            bool
	input              string
	directionInput     string
	phase              translatePhase
	directionSwapped   bool
	output             string
	dictionary         string
	dictionaryExpanded bool
	errorKey           string
	request            uint64
	cancel             context.CancelFunc
	clipped            int
	copiedUntil        time.Time
	focus              bool
	// speaking, speakFailed and autoSpoke are the pronunciation overlays.
	// speechToken makes a completion from a stopped utterance harmless.
	speaking    bool
	speakFailed bool
	autoSpoke   bool
	speechToken uint64
	// denied is the per-launch accessibility hint under the input. It is an
	// overlay, not a phase: submitting text must still stream normally, and
	// typing anything dismisses it.
	denied bool
}

type translateStreamFunc func(context.Context, translateConfig, string, string, string, func(string)) (string, error)
type translateModelsFunc func(context.Context, translateConfig) ([]string, error)
type translateTestFunc func(context.Context, translateConfig) (string, error)

func defaultTranslateCopy(text string) error {
	return mygo.Clipboard.Write(transfer.TextData(text))
}

func (a *application) ensureTranslateState() *translateState {
	if a.translate == nil {
		a.translate = &translateState{dictionaryExpanded: true}
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

// translateDictionaryFunction is the local dictionary seam. Headless tests
// replace it so no case needs the system dictionaries.
func (a *application) translateDictionaryFunction() func(string) string {
	if a.translateDictionaryFn != nil {
		return a.translateDictionaryFn
	}
	return dictionaryDefinition
}

func (a *application) registerTranslateShortcut() {
	a.registerSlot("translate", a.config.TranslateShortcut, a.config.TranslateEnabled, a.toggleTranslate)
}

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
	if !target {
		a.stopTranslateSpeech()
	}
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
	// The AX read runs on a worker with a 200 ms deadline, so a stuck
	// application can never hold the shortcut callback.
	a.beginTranslateSelection()
}

func (a *application) openTranslateText(text string) {
	a.cancelTranslateSelection()
	a.hideMenubarPanel()
	a.hidePalette()
	s := a.ensureTranslateState()
	if (s.visible || s.frame() != nil) && strings.TrimSpace(text) == "" {
		s.focus = true
		a.showTranslate()
		a.refreshTranslate()
		return
	}
	a.stopTranslateSpeech()
	a.cancelTranslate()
	s.input = text
	s.directionInput = text
	s.output = ""
	s.dictionary = ""
	s.dictionaryExpanded = true
	s.errorKey = ""
	s.clipped = 0
	s.directionSwapped = false
	s.copiedUntil = time.Time{}
	s.phase = translateEmpty
	s.autoSpoke = false
	s.speakFailed = false
	s.denied = false
	s.focus = true
	if s.frame() == nil && a.createTranslateWindow != nil {
		a.createTranslateWindow(s)
		a.placeTranslate()
	}
	a.showTranslate()
	if strings.TrimSpace(text) != "" {
		a.submitTranslate()
	} else {
		a.refreshTranslate()
	}
}

func (a *application) showTranslate() {
	s := a.translate
	if s == nil {
		return
	}
	if window := s.frame(); window != nil {
		if window.IsMinimized() {
			window.Restore()
		}
		a.reconcileTranslateDisplay()
		a.visibility(true)
		s.visible = true
		window.Show()
		window.Focus()
	}
	s.visible = true
	if s.window != nil {
		s.window.Invalidate()
	}
}

func (a *application) closeTranslate() {
	a.cancelTranslateSelection()
	s := a.translate
	if s == nil || s.closing {
		return
	}
	s.closing = true
	if a.rememberTranslatePlace() {
		a.save()
	}
	a.cancelTranslate()
	a.stopTranslateSpeech()
	s.visible = false
	if window := s.frame(); window != nil {
		window.Destroy()
	}
	if a.translate == s {
		a.translate = nil
	}
	a.visibility(false)
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
	// The first keystroke removes the accessibility hint.
	s.denied = false
	a.cancelTranslate()
	s.directionSwapped = false
	s.clipped = 0
	s.output = ""
	s.dictionary = ""
	s.dictionaryExpanded = true
	s.phase = translateEmpty
	s.errorKey = ""
	s.copiedUntil = time.Time{}
	a.stopTranslateSpeech()
	a.refreshTranslate()
}

// submitTranslate starts one request. A generation number makes late deltas
// and late completions harmless, and every UI update goes through the main
// thread seam.
func (a *application) submitTranslate() {
	s := a.ensureTranslateState()
	a.syncTranslateInput()
	a.stopTranslateSpeech()
	a.cancelTranslate()
	text := s.input
	if strings.TrimSpace(text) == "" {
		s.phase = translateEmpty
		s.output = ""
		s.dictionary = ""
		s.dictionaryExpanded = true
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
	s.dictionary = ""
	s.dictionaryExpanded = true
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
	dictionary := a.translateDictionaryFunction()
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
	// Only single English words get a dictionary lookup: a phrase does not
	// yield a definition for the whole phrase. The system call runs on a
	// worker, so the panel stays responsive while the model streams above.
	if isSingleEnglishWord(clipped) {
		a.runTranslateWorker(func() {
			definition := dictionary(clipped)
			apply(func() {
				if generation != s.request || definition == "" {
					return
				}
				s.dictionary = definition
				a.maybeAutoSpeakTranslate()
				a.refreshTranslate()
			})
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
			a.maybeAutoSpeakTranslate()
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

// toggleDictionary folds or unfolds the definition below the translation.
// The model's failure text and retry button stay above it either way.
func (a *application) toggleDictionary() {
	s := a.translate
	if s == nil || strings.TrimSpace(s.dictionary) == "" {
		return
	}
	s.dictionaryExpanded = !s.dictionaryExpanded
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
	s.window.Invalidate()
}

func (a *application) translateWindowOptions(t tokenValues, locale string) mygo.WindowOptions {
	return mygo.WindowOptions{
		Title:                a.text(locale, "Translate"),
		Width:                int(t.SizeTranslateWidth),
		Height:               int(t.SizeTranslateWindowHeight),
		MinWidth:             int(t.SizeTranslateWidth),
		MinHeight:            int(t.SizeTranslateMinHeight),
		Hidden:               true,
		TitleBarStyle:        mygo.TitleBarHidden,
		TrafficLightPosition: &mygo.Point{X: int(t.Space12), Y: int(t.Space16)},
		DisableFullScreen:    true,
		BackgroundColor:      t.ColorCard,
		Page:                 mygo.PageOptions{DevTools: mygo.DevToolsDisabled},
		Content:              ui.View(a.translateView),
	}
}

func (a *application) buildTranslateWindow(s *translateState) {
	t := tokens(mygo.Theme.IsDark(), mygo.App.Locale())
	window := mygo.NewWindow(a.translateWindowOptions(t, mygo.App.Locale()))
	s.window = window
	window.OnClose(func(*mygo.CloseEvent) {
		if a.rememberTranslatePlace() {
			a.save()
		}
		a.cancelTranslateSelection()
		a.cancelTranslate()
		a.stopTranslateSpeech()
		s.visible = false
	})
	window.OnClosed(func() {
		if a.translate == s {
			a.translate = nil
		}
		a.visibility(false)
	})
	window.OnHide(func() { s.visible = false; a.visibility(false) })
	window.OnMove(a.observeTranslateMove)
	window.OnRestore(a.reconcileTranslateDisplay)
}

func (a *application) setTranslatePinned(pinned bool) {
	if s := a.translate; s != nil {
		s.pinned = pinned
		if window := s.frame(); window != nil {
			window.SetAlwaysOnTop(pinned)
		}
		a.refreshTranslate()
	}
}

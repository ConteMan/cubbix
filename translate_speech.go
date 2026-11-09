package main

import (
	"errors"
	"strings"
)

// speechEngine is the pronunciation seam. The real implementation prefers
// AVSpeechSynthesizer and falls back to the say command; headless tests inject
// a fake so no test ever starts a voice, a process or a native framework.
type speechEngine interface {
	// Speak starts one utterance. done runs at most once when that utterance
	// ends by itself; Stop must make a late done harmless.
	Speak(text string, done func()) error
	Stop()
}

var errSpeechUnavailable = errors.New("speech: no available backend")

// unavailableSpeech is the backend on platforms without system speech. It
// keeps the fallback chain well defined without ever making a sound.
type unavailableSpeech struct{}

func (unavailableSpeech) Speak(string, func()) error { return errSpeechUnavailable }
func (unavailableSpeech) Stop()                      {}

// fallbackSpeech tries the primary backend first. A failure at creation,
// alloc, init or utterance construction goes to the fallback; the failed
// primary is stopped so it cannot leave a half-started utterance behind.
type fallbackSpeech struct {
	primary  speechEngine
	fallback speechEngine
}

func (s *fallbackSpeech) Speak(text string, done func()) error {
	if err := s.primary.Speak(text, done); err == nil {
		return nil
	}
	s.primary.Stop()
	return s.fallback.Speak(text, done)
}

func (s *fallbackSpeech) Stop() {
	s.primary.Stop()
	s.fallback.Stop()
}

// translateSpeechSource is always the source text. The design reads only the
// original (§0, §3.8): a word is read as the English word, and a sentence is
// read in its own language, never as the translated text.
func (a *application) translateSpeechSource() string {
	if a.translate == nil {
		return ""
	}
	return strings.TrimSpace(a.translate.input)
}

// speakTranslateSource is the button: one click reads the source, a second
// click stops it. The engine is only installed by ready(), so a headless test
// without a fake can neither speak nor fail.
func (a *application) speakTranslateSource() {
	s := a.translate
	if s == nil {
		return
	}
	if s.speaking {
		a.stopTranslateSpeech()
		a.refreshTranslate()
		return
	}
	text := a.translateSpeechSource()
	if text == "" {
		return
	}
	a.startTranslateSpeech(text)
}

func (a *application) startTranslateSpeech(text string) {
	s := a.translate
	if s == nil || a.speech == nil || strings.TrimSpace(text) == "" {
		return
	}
	a.stopTranslateSpeech()
	s.speechToken++
	token := s.speechToken
	err := a.speech.Speak(text, func() {
		a.translateOnMain(func() { a.translateSpeechFinished(token) })
	})
	s.speaking = err == nil
	s.speakFailed = err != nil
	a.refreshTranslate()
}

func (a *application) translateSpeechFinished(token uint64) {
	s := a.translate
	if s == nil || token != s.speechToken || !s.speaking {
		return
	}
	s.speaking = false
	a.refreshTranslate()
}

// stopTranslateSpeech interrupts the current utterance on a second click, a
// closed panel, a new request, a changed input or a disabled module. The token
// makes a completion that arrives after the stop harmless.
func (a *application) stopTranslateSpeech() {
	s := a.translate
	if s == nil {
		return
	}
	s.speechToken++
	s.speaking = false
	s.speakFailed = false
	if a.speech != nil {
		a.speech.Stop()
	}
}

// maybeAutoSpeakTranslate runs when a dictionary entry or a translation first
// appears. Only a single English word is read, at most once per opening, and
// only while the panel is showing with the preference on.
func (a *application) maybeAutoSpeakTranslate() {
	s := a.translate
	if s == nil || s.autoSpoke || s.speaking || !s.visible || !a.config.TranslateAutoSpeak || a.speech == nil {
		return
	}
	if !isSingleEnglishWord(strings.TrimSpace(s.input)) {
		return
	}
	if strings.TrimSpace(s.output) == "" && strings.TrimSpace(s.dictionary) == "" {
		return
	}
	s.autoSpoke = true
	a.startTranslateSpeech(a.translateSpeechSource())
}

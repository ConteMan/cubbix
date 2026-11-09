package main

import (
	"context"
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"
)

// fakeSpeechBackend is the only voice a unit test ever sees. It records calls
// and lets a test deliver the natural completion by hand; nothing speaks, no
// process starts and no framework is loaded.
type fakeSpeechBackend struct {
	err    error
	speaks []string
	stops  int
	done   func()
}

func (f *fakeSpeechBackend) Speak(text string, done func()) error {
	f.speaks = append(f.speaks, text)
	if f.err != nil {
		return f.err
	}
	f.done = done
	return nil
}

func (f *fakeSpeechBackend) Stop() { f.stops++ }

func (f *fakeSpeechBackend) finish() {
	if f.done == nil {
		return
	}
	done := f.done
	f.done = nil
	done()
}

func speechTestApp(t *testing.T) (*application, *fakeSpeechBackend, *fakeSpeechBackend) {
	t.Helper()
	a := translateTestApp(t)
	primary, fallback := &fakeSpeechBackend{}, &fakeSpeechBackend{}
	a.speech = &fallbackSpeech{primary: primary, fallback: fallback}
	return a, primary, fallback
}

// The real engine is installed by ready(), never by the constructor, so a
// headless test cannot start a voice or a child process by accident.
func TestTranslateSpeechSeamIsHeadless(t *testing.T) {
	a := translateTestApp(t)
	if a.speech != nil {
		t.Fatal("the constructor installed a real speech engine")
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, key := range []string{"TranslateSpeak", "TranslateSpeakStop", "TranslateSpeakFailed", "TranslateCardAction"} {
			if value := a.text(locale, key); value == "" {
				t.Fatalf("%s: missing %s", locale, key)
			}
		}
	}
}

func speechPanelTester(a *application, locale string, s *translateState) *ui.Tester {
	tk := tokens(false, locale)
	width := int(tk.SizeTranslateWidth) + 2*int(tk.SizePaletteShadowMargin)
	height := int(a.translateHeight(tk, s)) + 2*int(tk.SizePaletteShadowMargin)
	tt := ui.NewTester(a.translateView, width, height)
	tt.SetLocale(locale)
	tt.Frame()
	return tt
}

// The button reads the source in both word and sentence states, never the
// translation. A second click stops the utterance.
func TestTranslateSpeakButtonReadsSource(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, tc := range []struct{ name, input, output string }{
			{"word", "serendipity", "机缘巧合"},
			{"sentence", "Hello, world", "你好，世界"},
		} {
			t.Run(locale+"/"+tc.name, func(t *testing.T) {
				a, primary, fallback := speechTestApp(t)
				s := a.ensureTranslateState()
				s.visible, s.input, s.output, s.phase = true, tc.input, tc.output, translateDone
				tt := speechPanelTester(a, locale, s)
				if _, ok := tt.Find(a.text(locale, "TranslateSpeak")); !ok {
					t.Fatalf("speak button missing: %v", tt.Texts())
				}
				if err := tt.Click(a.text(locale, "TranslateSpeak")); err != nil {
					t.Fatal(err)
				}
				if len(primary.speaks) != 1 || primary.speaks[0] != tc.input {
					t.Fatalf("spoke %q, want the source %q", primary.speaks, tc.input)
				}
				if len(fallback.speaks) != 0 {
					t.Fatalf("the fallback was used although the primary worked: %v", fallback.speaks)
				}
				if !s.speaking {
					t.Fatal("speaking overlay did not start")
				}
				tt.Frame()
				if _, ok := tt.Find(a.text(locale, "TranslateSpeakStop")); !ok {
					t.Fatalf("stop label missing while speaking: %v", tt.Texts())
				}
				if err := tt.Click(a.text(locale, "TranslateSpeakStop")); err != nil {
					t.Fatal(err)
				}
				if s.speaking || primary.stops == 0 {
					t.Fatal("a second click did not stop the utterance")
				}
			})
		}
	}
}

// A natural completion clears the marker; a stopped utterance's late
// completion cannot change the state of a later one.
func TestTranslateSpeechCompletionAndStop(t *testing.T) {
	a, primary, _ := speechTestApp(t)
	s := a.ensureTranslateState()
	s.visible, s.input, s.output, s.phase = true, "hello", "你好", translateDone
	a.speakTranslateSource()
	if !s.speaking {
		t.Fatal("speech did not start")
	}
	primary.finish()
	if s.speaking {
		t.Fatal("natural completion left the speaking overlay on")
	}
	a.speakTranslateSource()
	stale := primary.done
	stops := primary.stops
	a.closeTranslate()
	if s.speaking || s.visible || primary.stops <= stops {
		t.Fatal("closing the panel did not stop speech")
	}
	if stale != nil {
		stale()
	}
	if s.speaking {
		t.Fatal("a late completion revived a stopped utterance")
	}
}

// The auto-speak preference reads one English word once, after a definition
// or a translation appears. Sentences and a disabled switch stay silent.
func TestTranslateAutoSpeakReadsOnce(t *testing.T) {
	configure := func(a *application, streamErr error) {
		a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
		a.translateSecret.(*fakeTranslateSecrets).value = []byte("k")
		a.translateDictionaryFn = func(string) string { return "hello hel·lo | noun a greeting" }
		a.translateStreamFn = func(_ context.Context, _ translateConfig, _ string, _, _ string, onDelta func(string)) (string, error) {
			if streamErr != nil {
				return "", streamErr
			}
			onDelta("你好")
			return "你好", nil
		}
	}
	a, primary, _ := speechTestApp(t)
	configure(a, nil)
	a.openTranslateText("hello")
	s := a.translate
	if !s.visible || s.phase != translateDone || !s.autoSpoke {
		t.Fatalf("word result = %+v", s)
	}
	if len(primary.speaks) != 1 || primary.speaks[0] != "hello" {
		t.Fatalf("auto speech = %v", primary.speaks)
	}
	a.refreshTranslate()
	a.maybeAutoSpeakTranslate()
	if len(primary.speaks) != 1 {
		t.Fatalf("the same result was read twice: %v", primary.speaks)
	}
	// A failed model still leaves a dictionary result to speak from.
	b, failedPrimary, _ := speechTestApp(t)
	configure(b, errors.New("endpoint down"))
	b.openTranslateText("serendipity")
	if b.translate.phase != translateFailed || len(failedPrimary.speaks) != 1 || failedPrimary.speaks[0] != "serendipity" {
		t.Fatalf("dictionary-only auto speech = %v, phase %v", failedPrimary.speaks, b.translate.phase)
	}
	// The switch off, then a sentence with the switch on: neither speaks.
	a.changeTranslateAutoSpeak(false)
	a.openTranslateText("hello")
	a.changeTranslateAutoSpeak(true)
	a.openTranslateText("Hello, world")
	if len(primary.speaks) != 1 {
		t.Fatalf("a disabled switch or a sentence spoke: %v", primary.speaks)
	}
}

// A failing AVSpeech backend falls back to say; both failing shows the
// failure tooltip. Stop reaches both backends.
func TestTranslateSpeechFallback(t *testing.T) {
	a, primary, fallback := speechTestApp(t)
	primary.err = errors.New("AVSpeech unavailable")
	s := a.ensureTranslateState()
	s.visible, s.input, s.output, s.phase = true, "hello", "你好", translateDone
	a.speakTranslateSource()
	if len(fallback.speaks) != 1 || fallback.speaks[0] != "hello" || !s.speaking || s.speakFailed {
		t.Fatalf("fallback = %v, speaking=%v failed=%v", fallback.speaks, s.speaking, s.speakFailed)
	}
	if primary.stops == 0 {
		t.Fatal("a failed primary was not stopped")
	}
	a.stopTranslateSpeech()
	if primary.stops < 2 || fallback.stops == 0 {
		t.Fatal("Stop did not reach both backends")
	}

	b, badPrimary, badFallback := speechTestApp(t)
	badPrimary.err = errors.New("no AVSpeech")
	badFallback.err = errors.New("no say")
	bs := b.ensureTranslateState()
	bs.visible, bs.input, bs.output, bs.phase = true, "hello", "你好", translateDone
	b.speakTranslateSource()
	if bs.speaking || !bs.speakFailed {
		t.Fatalf("both backends failing: speaking=%v failed=%v", bs.speaking, bs.speakFailed)
	}
	tt := speechPanelTester(b, "en", bs)
	if _, ok := tt.Find(b.text("en", "TranslateSpeakFailed")); !ok {
		t.Fatalf("failure tooltip missing: %v", tt.Texts())
	}
}

// A new request, a changed input, a hidden panel and the module switch all
// stop an utterance.
func TestTranslateSpeechStopsOnNewWork(t *testing.T) {
	a, primary, _ := speechTestApp(t)
	a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
	a.translateSecret.(*fakeTranslateSecrets).value = []byte("k")
	a.translateStreamFn = func(context.Context, translateConfig, string, string, string, func(string)) (string, error) {
		return "你好", nil
	}
	// Automatic reading stays off here: the test is about stopping a manual
	// utterance, not about starting a new one when the next result arrives.
	a.config.TranslateAutoSpeak = false
	s := a.ensureTranslateState()
	s.visible, s.input, s.output, s.phase = true, "hello", "你好", translateDone
	a.speakTranslateSource()
	stops := primary.stops
	a.submitTranslate()
	if s.speaking || primary.stops <= stops {
		t.Fatal("a new request did not stop speech")
	}
	a.speakTranslateSource()
	stops = primary.stops
	a.hideTranslate()
	if s.speaking || primary.stops <= stops {
		t.Fatal("hiding the panel did not stop speech")
	}
	a.config.TranslateAutoSpeak = true
	a.speakTranslateSource()
	stops = primary.stops
	a.changeTranslateAutoSpeak(false)
	if s.speaking || primary.stops <= stops {
		t.Fatal("turning the preference off did not stop speech")
	}
	a.speakTranslateSource()
	stops = primary.stops
	a.changeTranslateEnabled(false)
	if s.speaking || primary.stops <= stops {
		t.Fatal("turning the module off did not stop speech")
	}
}

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

const dictionaryTestDefinition = "hello hel·lo | həˈlō | noun 1 a greeting. 2 a question • a surprised greeting ORIGIN Old English"

func renderTranslatePanel(a *application, locale string, dark bool, s *translateState) *ui.Tester {
	tk := tokens(dark, locale)
	width := int(tk.SizeTranslateWidth)
	height := int(a.translateHeight(tk, s))
	tt := ui.NewTester(a.translateView, width, height)
	tt.SetLocale(locale)
	tt.SetDark(dark)
	tt.Frame()
	// The theme colour transition is 140 ms; advance past it before anything
	// inspects the frame.
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	return tt
}

func TestTranslateDictionaryStatesRender(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			for _, state := range []struct {
				name     string
				phase    translatePhase
				errorKey string
				expanded bool
			}{
				{"expanded", translateDone, "", true},
				{"collapsed", translateDone, "", false},
				{"failed", translateFailed, "TranslateNetworkError", true},
			} {
				t.Run(locale+"/"+state.name+"/dark="+map[bool]string{true: "1", false: "0"}[dark], func(t *testing.T) {
					a := translateTestApp(t)
					s := a.ensureTranslateState()
					s.input, s.phase, s.errorKey = "hello", state.phase, state.errorKey
					s.dictionary, s.dictionaryExpanded = dictionaryTestDefinition, state.expanded
					s.output = ""
					if state.phase == translateDone {
						s.output = "你好，世界"
					}
					tt := renderTranslatePanel(a, locale, dark, s)
					tk := tokens(dark, locale)
					header := a.text(locale, "TranslateDictionary")
					bounds, ok := tt.Find(header)
					if !ok || bounds.X < 0 || bounds.X+bounds.W > tk.SizeTranslateWidth {
						t.Fatalf("dictionary header clipped: %+v; %v", bounds, tt.Texts())
					}
					if state.expanded {
						if !tt.HasText("noun 1 a greeting.") {
							t.Fatalf("definition text missing: %v", tt.Texts())
						}
					} else if tt.HasText("noun 1 a greeting.") {
						t.Fatal("collapsed block still drew the definition")
					}
					if state.phase == translateFailed {
						failure, ok := tt.Find(a.text(locale, "TranslateNetworkError"))
						if !ok || !tt.HasText(a.text(locale, "TranslateRetry")) {
							t.Fatalf("failure copy missing: %+v; %v", failure, tt.Texts())
						}
						if failure.Y >= bounds.Y {
							t.Fatalf("dictionary was not below the failure text: %+v / %+v", bounds, failure)
						}
					}
				})
			}
		}
	}
}

func TestTranslateDictionaryToggle(t *testing.T) {
	a := translateTestApp(t)
	tk := tokens(false, "en")
	s := a.ensureTranslateState()
	s.input, s.output, s.phase = "hello", "你好", translateDone
	s.dictionary, s.dictionaryExpanded = dictionaryTestDefinition, true
	expanded := a.translateHeight(tk, s)
	tt := renderTranslatePanel(a, "en", false, s)
	if err := tt.Click(a.text("en", "TranslateDictionary")); err != nil {
		t.Fatalf("fold button: %v", err)
	}
	if s.dictionaryExpanded || a.translateHeight(tk, s) > expanded {
		t.Fatalf("fold did not shrink the panel: %v -> %v", expanded, a.translateHeight(tk, s))
	}
	tt.Frame()
	if err := tt.Click(a.text("en", "TranslateDictionary")); err != nil {
		t.Fatalf("unfold button: %v", err)
	}
	if !s.dictionaryExpanded || a.translateHeight(tk, s) != expanded {
		t.Fatal("unfold did not restore the expanded height")
	}
	// A block without a definition has no fold button at all.
	s.dictionary = ""
	tt = renderTranslatePanel(a, "en", false, s)
	if err := tt.Click(a.text("en", "TranslateDictionary")); err == nil {
		t.Fatal("empty dictionary still had a fold button")
	}
}

func TestTranslateDictionaryOnlyForSingleWords(t *testing.T) {
	a := translateTestApp(t)
	secrets := a.translateSecret.(*fakeTranslateSecrets)
	secrets.value = []byte("k")
	a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
	a.translateStreamFn = func(context.Context, translateConfig, string, string, string, func(string)) (string, error) {
		return "translated", nil
	}
	calls := 0
	a.translateDictionaryFn = func(term string) string {
		calls++
		return "noun 1 " + term
	}
	for _, term := range []string{"hello", "Hello", "well-known", "don't", "it’s"} {
		s := a.ensureTranslateState()
		s.input = term
		a.submitTranslate()
		if s.dictionary == "" || calls == 0 {
			t.Fatalf("%q did not get a definition", term)
		}
	}
	before := calls
	for _, term := range []string{"take into account", "out of the box", "hello world", "take2", "你好", "123", "a.b", "hello, world"} {
		s := a.ensureTranslateState()
		s.input = term
		a.submitTranslate()
		if s.dictionary != "" || calls != before {
			t.Fatalf("%q was looked up in the dictionary", term)
		}
	}
	// A nil definition omits the whole block, including the fold header.
	a.translateDictionaryFn = func(string) string { return "" }
	a.translateStreamFn = func(context.Context, translateConfig, string, string, string, func(string)) (string, error) {
		return "", &translateHTTPError{status: 401}
	}
	s := a.ensureTranslateState()
	s.input = "hello"
	a.submitTranslate()
	if s.dictionary != "" || s.phase != translateFailed || s.errorKey != "TranslateAuthError" {
		t.Fatalf("nil definition or model failure broke the state: %+v", s)
	}
}

func TestTranslateModelFailureKeepsDictionary(t *testing.T) {
	a := translateTestApp(t)
	secrets := a.translateSecret.(*fakeTranslateSecrets)
	secrets.value = []byte("k")
	a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
	a.translateStreamFn = func(context.Context, translateConfig, string, string, string, func(string)) (string, error) {
		return "", &translateHTTPError{status: 503}
	}
	a.translateDictionaryFn = func(string) string { return dictionaryTestDefinition }
	s := a.ensureTranslateState()
	s.input = "hello"
	a.submitTranslate()
	if s.phase != translateFailed || s.errorKey != "TranslateServerError" || s.dictionary == "" || !s.dictionaryExpanded {
		t.Fatalf("failure dropped the dictionary: %+v", s)
	}
	tt := renderTranslatePanel(a, "en", false, s)
	if !tt.HasText(a.text("en", "TranslateServerError")) || !tt.HasText(a.text("en", "TranslateRetry")) ||
		!tt.HasText(a.text("en", "TranslateDictionary")) || !tt.HasText("noun 1 a greeting.") {
		t.Fatalf("failure layout is incomplete: %v", tt.Texts())
	}

}

func TestTranslateDictionaryLookupUsesWorker(t *testing.T) {
	a := translateTestApp(t)
	a.translateAsync = true
	secrets := a.translateSecret.(*fakeTranslateSecrets)
	secrets.value = []byte("k")
	a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
	a.translateStreamFn = func(context.Context, translateConfig, string, string, string, func(string)) (string, error) {
		return "translated", nil
	}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	a.translateDictionaryFn = func(string) string {
		started <- struct{}{}
		<-release
		return dictionaryTestDefinition
	}
	queued := make(chan func(), 4)
	a.translateRunOnMain = func(fn func()) { queued <- fn }
	drain := func() {
		for range 2 {
			select {
			case fn := <-queued:
				fn()
			case <-time.After(time.Second):
				t.Fatal("worker result was not dispatched")
			}
		}
	}
	s := a.ensureTranslateState()
	s.input = "hello"
	a.submitTranslate()
	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("dictionary lookup did not start on a worker")
	}
	if strings.TrimSpace(s.dictionary) != "" {
		close(release)
		t.Fatal("worker wrote the UI before main-thread dispatch")
	}
	close(release)
	drain()
	if s.dictionary != dictionaryTestDefinition {
		t.Fatalf("worker result was not dispatched: %q", s.dictionary)
	}
}

func TestTranslateInputEditDiscardsQueuedResults(t *testing.T) {
	for _, action := range []string{"edit", "close", "submit"} {
		t.Run(action, func(t *testing.T) {
			testTranslateDiscardQueuedResults(t, action)
		})
	}
}

func testTranslateDiscardQueuedResults(t *testing.T, action string) {
	t.Helper()
	a := translateTestApp(t)
	a.translateSecret.(*fakeTranslateSecrets).value = []byte("k")
	a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
	a.translateDictionaryFn = func(string) string { return dictionaryTestDefinition }
	a.translateStreamFn = func(_ context.Context, _ translateConfig, _, _, _ string, delta func(string)) (string, error) {
		delta("old delta")
		return "old result", nil
	}
	var pending []func()
	a.translateRunOnMain = func(fn func()) { pending = append(pending, fn) }
	s := a.ensureTranslateState()
	s.input = "hello"
	a.submitTranslate()
	old := pending
	pending = nil
	wantPhase := translateEmpty
	switch action {
	case "edit":
		s.input = "new phrase"
		a.syncTranslateInput()
	case "close":
		a.closeTranslate()
		wantPhase = translateLoading
	case "submit":
		s.input = "world"
		a.submitTranslate()
		wantPhase = translateLoading
	}
	for _, fn := range old {
		fn()
	}
	if s.dictionary != "" || s.output != "" || s.phase != wantPhase {
		t.Fatalf("%s accepted old results: %+v", action, s)
	}
	if action == "submit" {
		for _, fn := range pending {
			fn()
		}
		if s.dictionary == "" || s.output != "old result" || s.phase != translateDone {
			t.Fatalf("current generation was discarded: %+v", s)
		}
	}
}

func TestTranslateDictionaryHeightBudgetAndScroll(t *testing.T) {
	for _, expanded := range []bool{false, true} {
		t.Run(map[bool]string{false: "collapsed", true: "expanded"}[expanded], func(t *testing.T) {
			a := translateTestApp(t)
			s := a.ensureTranslateState()
			s.input, s.output, s.phase = "a\nb\nc\nd", strings.Repeat("long output ", 500), translateStreaming
			s.dictionary = strings.Repeat("noun a definition • another sense ", 10) + "ORIGIN last visible marker"
			s.dictionaryExpanded, s.clipped = expanded, 10
			tk := tokens(false, "en")
			tester := renderTranslatePanel(a, "en", false, s)
			header, ok := tester.Find(a.text("en", "TranslateDictionary"))
			if !ok {
				t.Fatal("dictionary header missing")
			}
			bottom := a.translateHeight(tk, s) - tk.Space12
			wantHeaderBottom := bottom
			if expanded {
				wantHeaderBottom -= dictionaryBodyHeight(tk)
			}
			if header.Y+header.H > wantHeaderBottom+1 {
				t.Fatalf("dictionary lost its bottom padding: header=%+v, bottom=%v, want header end <= %v", header, bottom, wantHeaderBottom)
			}
			if expanded {
				tester.Scroll(header.X+header.W/2, header.Y+header.H+tk.Space8, 0, 10000)
				tester.Frame()
				last, found := tester.Find("ORIGIN last visible marker")
				if !found || last.Y < header.Y+header.H || last.Y+last.H > bottom+1 {
					t.Fatalf("last definition line is not reachable inside the block: %+v, header=%+v, bottom=%v", last, header, bottom)
				}
			}
		})
	}
}

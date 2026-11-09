package main

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// fakeSelectionReader is the only AX implementation the unit tests see. It
// never reads the frontmost application, the clipboard or the TCC state.
type fakeSelectionReader struct {
	trusted bool
	pid     int32
	text    string
	err     error
	wait    <-chan struct{}

	trustedCalls atomic.Int32
	pidCalls     atomic.Int32
	readCalls    atomic.Int32
}

func (f *fakeSelectionReader) Trusted() bool {
	f.trustedCalls.Add(1)
	return f.trusted
}

func (f *fakeSelectionReader) FrontmostPID() int32 {
	f.pidCalls.Add(1)
	return f.pid
}

func (f *fakeSelectionReader) SelectedText(ctx context.Context, _ int32) (string, error) {
	f.readCalls.Add(1)
	if f.wait != nil {
		select {
		case <-f.wait:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.err != nil {
		return "", f.err
	}
	return f.text, nil
}

func otherSelectionPID() int32 { return int32(os.Getpid() + 1) }

func selectionTestApp(t *testing.T, reader selectionReader) *application {
	t.Helper()
	a := translateTestApp(t)
	a.selection = reader
	a.selectionBudget = selectionTimeout
	a.openAccessibilitySettings = func() {}
	return a
}

// TestTranslateAXClassification is the six-state matrix from the work order:
// selected text, empty selection, read failure, timeout, no permission and
// Cubbix itself in front. A failed or slow read is never retried.
func TestTranslateAXClassification(t *testing.T) {
	blocked := make(chan struct{})
	defer close(blocked)
	cases := []struct {
		name   string
		reader *fakeSelectionReader
		status selectionStatus
		text   string
		reads  int32
		budget time.Duration
	}{
		{"selected", &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "Hello iOS"}, selectionFound, "Hello iOS", 1, 0},
		{"empty", &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "  \n "}, selectionNone, "", 1, 0},
		{"no-value", &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), err: errSelectionNoValue}, selectionNone, "", 1, 0},
		{"cannot-complete", &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), err: errSelectionUnavailable}, selectionNone, "", 1, 0},
		{"illegal-argument", &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), err: errSelectionIllegalArgument}, selectionNone, "", 1, 0},
		{"api-disabled", &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), err: errSelectionDenied}, selectionDenied, "", 1, 0},
		{"no-permission", &fakeSelectionReader{pid: otherSelectionPID(), text: "secret"}, selectionDenied, "", 0, 0},
		{"own-application", &fakeSelectionReader{trusted: true, pid: int32(os.Getpid()), text: "secret"}, selectionNone, "", 0, 0},
		{"no-frontmost", &fakeSelectionReader{trusted: true, pid: 0, text: "secret"}, selectionNone, "", 0, 0},
		{"timeout", &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "late", wait: blocked}, selectionNone, "", 1, 20 * time.Millisecond},
	}
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			a := selectionTestApp(t, scenario.reader)
			if scenario.budget > 0 {
				a.selectionBudget = scenario.budget
			}
			start := time.Now()
			answer := a.captureSelection()
			if scenario.budget > 0 && time.Since(start) > selectionTimeout {
				t.Fatalf("AX read exceeded the 200 ms budget: %v", time.Since(start))
			}
			if scenario.budget > 0 && answer.trustedKnown {
				t.Fatal("a timeout invented a permission state")
			}
			if answer.status != scenario.status || answer.text != scenario.text {
				t.Fatalf("selection = %+v; want status %v text %q", answer, scenario.status, scenario.text)
			}
			if got := scenario.reader.trustedCalls.Load(); got != 1 {
				t.Fatalf("permission checked %d times; want 1", got)
			}
			if got := scenario.reader.readCalls.Load(); got != scenario.reads {
				t.Fatalf("AX read %d times; want %d", got, scenario.reads)
			}
		})
	}
}

// configuredSelectionApp lets a shortcut press really start a translation
// with the injected stream seam: no window, keychain or network is involved.
func configuredSelectionApp(t *testing.T, reader selectionReader) (*application, *[]string) {
	t.Helper()
	a := selectionTestApp(t, reader)
	a.translateSecret.(*fakeTranslateSecrets).value = []byte("k")
	a.config.TranslateBaseURL = "https://h/v1"
	a.config.TranslateModel = "m"
	texts := new([]string)
	a.translateStreamFn = func(_ context.Context, _ translateConfig, text, _, _ string, _ func(string)) (string, error) {
		*texts = append(*texts, text)
		return "译文", nil
	}
	return a, texts
}

func TestTranslateShortcutSelectionFlow(t *testing.T) {
	a, texts := configuredSelectionApp(t, &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "Hello iOS"})
	a.toggleTranslate()
	if !a.translateVisible() || a.translate.input != "Hello iOS" || a.translate.phase != translateDone {
		t.Fatalf("selected text did not translate: %+v", a.translate)
	}
	if len(*texts) != 1 || (*texts)[0] != "Hello iOS" {
		t.Fatalf("streamed texts = %v", *texts)
	}
	if !a.translateSelectionTrusted {
		t.Fatal("a successful read did not record the permission state")
	}

	b, _ := configuredSelectionApp(t, &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "  "})
	b.toggleTranslate()
	if !b.translateVisible() || b.translate.input != "" || b.translate.phase != translateEmpty || b.translate.denied {
		t.Fatalf("empty selection did not open an empty input: %+v", b.translate)
	}

	c, _ := configuredSelectionApp(t, &fakeSelectionReader{pid: otherSelectionPID()})
	c.toggleTranslate()
	if !c.translateVisible() || c.translate.input != "" || !c.translate.denied || !c.translateSelectionHintShown {
		t.Fatalf("denied read did not show the hint: %+v", c.translate)
	}
	// A second denied press in the same launch keeps the input empty and
	// never repeats the hint or opens a system window.
	c.closeTranslate()
	c.toggleTranslate()
	if c.translate.denied {
		t.Fatal("accessibility hint repeated in the same launch")
	}
}

func TestTranslateSelectionHintDismissAndSettingsButton(t *testing.T) {
	opens := 0
	a := selectionTestApp(t, &fakeSelectionReader{pid: otherSelectionPID()})
	a.openAccessibilitySettings = func() { opens++ }
	a.toggleTranslate()
	if !a.translate.denied {
		t.Fatal("hint missing")
	}
	// Typing removes the hint but keeps the request alive.
	a.translate.input = "hello"
	a.syncTranslateInput()
	if a.translate.denied {
		t.Fatal("hint stayed after typing")
	}
	a.translate.denied = true
	a.grantAccessibility()
	if a.translate.denied || opens != 1 {
		t.Fatalf("grant = denied %v opens %d", a.translate.denied, opens)
	}
	a.translate.denied = true
	a.grantAccessibility()
	if opens != 1 {
		t.Fatalf("System Settings opened %d times in one launch", opens)
	}
	if a.selection.(*fakeSelectionReader).readCalls.Load() != 0 {
		t.Fatal("no permission still made an AX read")
	}
}

func TestTranslateSelectionSettingsRow(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		for _, locale := range []string{"zh-CN", "en"} {
			for _, dark := range []bool{false, true} {
				a := selectionTestApp(t, nil)
				a.page = settingsTranslate
				a.translateSelectionTrusted = trusted
				tt := ui.NewTester(a.view, 720, settingsWindowHeight)
				tt.SetLocale(locale)
				tt.SetDark(dark)
				tt.Frame()
				status := "TranslateSelectionUnauthorized"
				if trusted {
					status = "TranslateSelectionAuthorized"
				}
				for _, key := range []string{"TranslateSelectionLabel", status, "TranslateGrantAccess"} {
					label := a.text(locale, key)
					bounds, ok := tt.Find(label)
					if !ok || bounds.W <= 0 || bounds.X < 0 || bounds.X+bounds.W > 720 {
						t.Fatalf("%s %v trusted=%v: %q = %+v %v", locale, dark, trusted, key, bounds, ok)
					}
				}
				// Level 3 keeps the long rule in an info tooltip: the anchor
				// is focusable and carries the whole text as its label.
				info := a.text(locale, "TranslateSelectionInfo")
				if _, ok := tt.Find(info); !ok {
					t.Fatalf("level-3 explanation missing for %s", locale)
				}
			}
		}
	}
}

func TestTranslateSelectionHintRendered(t *testing.T) {
	a := selectionTestApp(t, &fakeSelectionReader{pid: otherSelectionPID()})
	a.toggleTranslate()
	s := a.translate
	if s == nil || !s.denied {
		t.Fatal("hint state missing")
	}
	tk := tokens(false, "zh-CN")
	margin := int(tk.SizePaletteShadowMargin)
	width := int(tk.SizeTranslateWidth) + 2*margin
	height := int(a.translateHeight(tk, s)) + 2*margin
	tt := ui.NewTester(a.translateView, width, height)
	tt.SetLocale("zh-CN")
	tt.Frame()
	for _, key := range []string{"TranslateSelectionDenied", "TranslateGrantAccess"} {
		bounds, ok := tt.Find(a.text("zh-CN", key))
		if !ok || bounds.Y < 0 || bounds.Y+bounds.H > float32(height) {
			t.Fatalf("hint %s clipped: %+v %v (height %d)", key, bounds, ok, height)
		}
	}
	// The hint sits below the input row, not inside the result block.
	input, hasInput := tt.Find(a.text("zh-CN", "TranslateInputPlaceholder"))
	hint, _ := tt.Find(a.text("zh-CN", "TranslateSelectionDenied"))
	if hasInput && hint.Y < input.Y {
		t.Fatal("hint drawn above the input")
	}
}

func TestCLITranslateSelection(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		reader *fakeSelectionReader
		ok     bool
	}{
		{"selected", &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "Hello iOS"}, true},
		{"empty", &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "  "}, false},
		{"read-failure", &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), err: errSelectionUnavailable}, false},
		{"no-permission", &fakeSelectionReader{pid: otherSelectionPID(), text: "secret"}, false},
		{"own-application", &fakeSelectionReader{trusted: true, pid: int32(os.Getpid()), text: "secret"}, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			a := translateCLIApp(t)
			a.selection = scenario.reader
			a.selectionBudget = selectionTimeout
			var streamed string
			a.translateStreamFn = func(_ context.Context, _ translateConfig, text, _, _ string, _ func(string)) (string, error) {
				streamed = text
				return "译文", nil
			}
			result, body := runTranslateCLI(t, a, translateCLIRequest(""), nil)
			if scenario.ok {
				if !result.OK || string(body) != "译文" || streamed != "Hello iOS" {
					t.Fatalf("selection translation = %+v %q streamed %q", result, body, streamed)
				}
				if a.translateVisible() {
					t.Fatal("CLI opened the floating panel")
				}
			} else {
				if result.Code != "TranslateNoSelection" || result.Message == "" || len(body) != 0 || streamed != "" {
					t.Fatalf("no selection = %+v %q streamed %q", result, body, streamed)
				}
			}
		})
	}
}

func TestTranslateSelectionTimeoutKeepsTrust(t *testing.T) {
	blocked := make(chan struct{})
	defer close(blocked)
	a := selectionTestApp(t, &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "late", wait: blocked})
	a.selectionBudget = 20 * time.Millisecond
	a.translateSelectionTrusted = true
	a.toggleTranslate()
	if !a.translateSelectionTrusted {
		t.Fatal("a timeout cleared the last known permission state")
	}
	if a.translate == nil || a.translate.denied || a.translate.input != "" || !a.translateVisible() {
		t.Fatalf("timeout did not fall back to an empty input: %+v", a.translate)
	}
}

func TestCLITranslateSelectionTimeout(t *testing.T) {
	blocked := make(chan struct{})
	defer close(blocked)
	a := translateCLIApp(t)
	a.selection = &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "late", wait: blocked}
	a.selectionBudget = 20 * time.Millisecond
	result, body := runTranslateCLI(t, a, translateCLIRequest(""), nil)
	if result.Code != "TranslateNoSelection" || len(body) != 0 {
		t.Fatalf("timed-out selection = %+v %q", result, body)
	}
}

func TestTranslateSelectionNoRealSystemAccessInTests(t *testing.T) {
	a := translateTestApp(t)
	if a.selection != nil || a.openAccessibilitySettings != nil {
		t.Fatal("headless app installed a real AX reader or System Settings opener")
	}
	if selectionTimeout > 300*time.Millisecond {
		t.Fatalf("AX budget %v exceeds the 300 ms cap", selectionTimeout)
	}
	if got := a.selectionTimeoutDuration(); got != selectionTimeout {
		t.Fatalf("default budget = %v", got)
	}
	// The platform entry points keep their documented signatures without
	// ever being called by a unit test.
	var _ func() bool = accessibilityTrusted
	var _ func() (string, bool) = copySelectedText
}

package main

import (
	"context"
	"testing"
	"time"

	"github.com/ConteMan/cubbix/internal/cli"
	"github.com/egoist/mygo/ui"
)

func TestTranslateSelectionQueuedResultInvalidated(t *testing.T) {
	for _, scenario := range []string{"close", "disable", "palette", "manual", "quit", "second-press"} {
		t.Run(scenario, func(t *testing.T) {
			a, texts := configuredSelectionApp(t, &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "old text"})
			a.createPaletteWindow = func(*paletteState) {}
			var queued []func()
			a.translateRunOnMain = func(fn func()) { queued = append(queued, fn) }
			a.toggleTranslate()
			if !a.translateSelectionPending || a.translateVisible() {
				t.Fatal("read did not wait for main dispatch")
			}
			switch scenario {
			case "close":
				a.closeTranslate()
			case "disable":
				a.changeTranslateEnabled(false)
			case "palette":
				a.showPalette()
			case "manual":
				a.openTranslateText("")
				a.closeTranslate()
			case "quit":
				a.quitting = true
			case "second-press":
				a.toggleTranslate()
				a.closeTranslate()
			}
			for _, finish := range queued {
				finish()
			}
			if a.translateVisible() || len(*texts) != 0 {
				t.Fatal("stale selection reopened the panel or submitted text")
			}
			if scenario == "palette" && !a.paletteVisible() {
				t.Fatal("stale selection hid the palette")
			}
		})
	}
}

type delayedSelectionReader struct {
	fakeSelectionReader
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func (reader *delayedSelectionReader) SelectedText(context.Context, int32) (string, error) {
	reader.readCalls.Add(1)
	close(reader.started)
	<-reader.release
	close(reader.finished)
	return "late text", nil
}

func TestTranslateSelectionBoundsOutstandingRead(t *testing.T) {
	reader := &delayedSelectionReader{
		fakeSelectionReader: fakeSelectionReader{trusted: true, pid: otherSelectionPID()},
		started:             make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{}),
	}
	a := selectionTestApp(t, reader)
	a.selectionBudget = 20 * time.Millisecond
	defer func() {
		close(reader.release)
		select {
		case <-reader.finished:
		case <-time.After(time.Second):
			t.Error("late native read did not release its worker")
		}
	}()
	if answer := a.captureSelection(); answer.status != selectionNone || answer.text != "" {
		t.Fatal("slow read was accepted")
	}
	for attempt := 0; attempt < 20; attempt++ {
		if answer := a.captureSelection(); answer.status != selectionNone || answer.text != "" {
			t.Fatal("outstanding read was accepted")
		}
	}
	if reader.readCalls.Load() != 1 {
		t.Fatal("timeouts spawned additional native reads")
	}
}

func TestTranslateSelectionWorkerDoesNotBlockShortcut(t *testing.T) {
	blocked := make(chan struct{})
	defer close(blocked)
	a := selectionTestApp(t, &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), wait: blocked})
	a.translateAsync = true
	a.selectionBudget = 30 * time.Millisecond
	queued := make(chan func(), 1)
	a.translateRunOnMain = func(fn func()) { queued <- fn }
	start := time.Now()
	a.toggleTranslate()
	if time.Since(start) >= a.selectionBudget || !a.translateSelectionPending {
		t.Fatal("shortcut waited for selection")
	}
	select {
	case finish := <-queued:
		finish()
	case <-time.After(time.Second):
		t.Fatal("read did not reach its deadline")
	}
	if !a.translateVisible() || a.translate.input != "" {
		t.Fatal("timeout did not open empty panel")
	}
}

func TestTranslatePermissionRefreshOnSettingsFocus(t *testing.T) {
	reader := &fakeSelectionReader{}
	a := selectionTestApp(t, reader)
	a.selectSettingsPage(settingsTranslate)
	for _, trusted := range []bool{true, false, true} {
		reader.trusted = trusted
		a.onSettingsFocus()
		if a.translateSelectionTrusted != trusted {
			t.Fatal("returning to settings kept stale permission")
		}
	}
	if reader.readCalls.Load() != 0 || reader.pidCalls.Load() != 0 {
		t.Fatal("permission refresh read the foreground selection")
	}
}

func TestCLITranslateSelectionQueuedResultInvalidated(t *testing.T) {
	for _, scenario := range []string{"cancel", "disable", "quit"} {
		t.Run(scenario, func(t *testing.T) {
			a := translateCLIApp(t)
			a.selection = &fakeSelectionReader{trusted: true, pid: otherSelectionPID(), text: "late text"}
			var queued []func()
			a.translateCLIDispatch = func(fn func()) { queued = append(queued, fn) }
			streamed := false
			a.translateStreamFn = func(context.Context, translateConfig, string, string, string, func(string)) (string, error) {
				streamed = true
				return "", nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var result cli.Result
			replies := 0
			a.applyTranslateCLI(ctx, translateCLIRequest(""), nil, func(reply cli.Result, _ []byte) {
				result = reply
				replies++
			})
			if len(queued) != 1 || replies != 0 {
				t.Fatal("selection did not wait for main dispatch")
			}
			switch scenario {
			case "cancel":
				cancel()
			case "disable":
				a.changeTranslateEnabled(false)
			case "quit":
				a.quitting = true
			}
			for _, finish := range queued {
				finish()
			}
			if scenario == "disable" {
				if replies != 1 || result.Code != cli.CodeDisabled {
					t.Fatalf("disabled selection replied %d times: %+v", replies, result)
				}
			} else if replies != 0 {
				t.Fatal("cancelled selection replied")
			}
			if streamed || a.translateVisible() {
				t.Fatal("stale CLI selection submitted text or opened a panel")
			}
		})
	}
}

func TestTranslateSettingsScrollAtSharedMinimum(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		a := translateTestApp(t)
		a.page = settingsTranslate
		tester := ui.NewTester(a.view, 720, settingsWindowHeight)
		tester.SetLocale(locale)
		tester.Frame()
		before, ok := tester.Find(a.text(locale, "TranslateRecordShortcut"))
		if !ok {
			t.Fatal("shortcut section missing")
		}
		tester.Scroll(650, 350, 0, 1000)
		tester.Frame()
		after, ok := tester.Find(a.text(locale, "TranslateRecordShortcut"))
		tokens := tokens(false, locale)
		bottomInset := (rowHeight(tokens, 3)-tokens.SizeSegmentHeight)/2 + tokens.StrokeHairline
		if !ok || after.Y >= before.Y || after.Y+after.H+bottomInset > settingsWindowHeight-tokens.Space24 {
			t.Fatalf("shortcut section did not scroll into view: before=%+v after=%+v", before, after)
		}
		tester.SetSize(720, 600)
		tester.Frame()
		full, ok := tester.Find(a.text(locale, "TranslateRecordShortcut"))
		if !ok || full.Y != before.Y {
			t.Fatal("enlarging settings did not clear excess scroll")
		}
	}
}

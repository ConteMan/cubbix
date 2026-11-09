package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ConteMan/cubbix/internal/cli"
)

// selectionTimeout bounds the whole AX read. The spec sets a 200 ms budget;
// a stuck application is treated exactly like "no selection".
const selectionTimeout = 200 * time.Millisecond

// accessibilitySettingsURL is opened only when the user asks for it. Opening
// System Settings never triggers a TCC prompt; AXIsProcessTrustedWithOptions
// would, so it is deliberately not used anywhere in this package.
const accessibilitySettingsURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"

type selectionStatus int

const (
	selectionNone selectionStatus = iota
	selectionFound
	selectionDenied
)

// selectionReader is the seam around every Accessibility call. Production
// uses the purego/objc reader; unit tests inject a fake so no test touches the
// real frontmost application, the clipboard or an authorization prompt.
type selectionReader interface {
	Trusted() bool
	FrontmostPID() int32
	SelectedText(pid int32) (string, error)
}

// The AX errors that reach this layer are classified here; the darwin reader
// maps AXError codes onto these sentinels. None of them carries user text.
var (
	errSelectionNoValue         = errors.New("selection: no selected text attribute")
	errSelectionDenied          = errors.New("selection: accessibility disabled")
	errSelectionUnavailable     = errors.New("selection: application did not answer")
	errSelectionIllegalArgument = errors.New("selection: invalid attribute")
)

type selectionAnswer struct {
	text         string
	status       selectionStatus
	trusted      bool
	trustedKnown bool // false for a timeout: keep the last known permission
	reason       string
}

// systemSelectionReader is defined per platform: the darwin one uses AX, the
// stub on other platforms reports no permission and no frontmost application.
func (a *application) selectionTimeoutDuration() time.Duration {
	if a.selectionBudget > 0 {
		return a.selectionBudget
	}
	return selectionTimeout
}

// captureSelection reads the frontmost application's selection with a hard
// deadline. A slow AX call cannot hold the shortcut callback or the CLI
// socket: the caller runs this on a worker and the timer always wins.
func (a *application) captureSelection() selectionAnswer {
	reader := a.selection
	if reader == nil {
		return selectionAnswer{status: selectionNone, reason: "no selection reader"}
	}
	type outcome struct{ answer selectionAnswer }
	done := make(chan outcome, 1) // buffered: a late reader never blocks
	go func() {
		done <- outcome{readSelection(reader, int32(os.Getpid()))}
	}()
	timer := time.NewTimer(a.selectionTimeoutDuration())
	defer timer.Stop()
	select {
	case result := <-done:
		return result.answer
	case <-timer.C:
		return selectionAnswer{status: selectionNone, reason: "selection timeout"}
	}
}

// readSelection is the AX decision tree, free of window and platform state:
// no permission, the frontmost application being Cubbix itself, an empty
// string and every read failure all fall back to an empty input.
func readSelection(reader selectionReader, ownPID int32) selectionAnswer {
	if !reader.Trusted() {
		return selectionAnswer{status: selectionDenied, trustedKnown: true, reason: "accessibility not trusted"}
	}
	pid := reader.FrontmostPID()
	switch {
	case pid <= 0:
		return selectionAnswer{status: selectionNone, trusted: true, trustedKnown: true, reason: "no frontmost application"}
	case pid == ownPID:
		return selectionAnswer{status: selectionNone, trusted: true, trustedKnown: true, reason: "frontmost is Cubbix"}
	}
	text, err := reader.SelectedText(pid)
	if err != nil {
		if errors.Is(err, errSelectionDenied) {
			return selectionAnswer{status: selectionDenied, trustedKnown: true, reason: "accessibility disabled"}
		}
		return selectionAnswer{status: selectionNone, trusted: true, trustedKnown: true, reason: err.Error()}
	}
	if strings.TrimSpace(text) == "" {
		return selectionAnswer{status: selectionNone, trusted: true, trustedKnown: true, reason: "empty selection"}
	}
	return selectionAnswer{text: text, status: selectionFound, trusted: true, trustedKnown: true}
}

// beginTranslateSelection is the shortcut path. It never blocks the hotkey:
// the AX read runs on a worker, and the panel is opened on the main thread
// once the bounded read has finished. A second press cancels a pending read.
func (a *application) beginTranslateSelection() {
	a.hideMenubarPanel()
	a.hidePalette()
	if a.translateSelectionPending {
		a.translateSelectionGeneration++
		a.translateSelectionPending = false
		a.openTranslateText("")
		return
	}
	a.translateSelectionGeneration++
	generation := a.translateSelectionGeneration
	a.translateSelectionPending = true
	a.runTranslateWorker(func() {
		answer := a.captureSelection()
		a.translateOnMain(func() {
			if generation != a.translateSelectionGeneration {
				return
			}
			a.translateSelectionPending = false
			if answer.trustedKnown {
				a.translateSelectionTrusted = answer.trusted
			}
			if a.translateVisible() {
				return
			}
			switch answer.status {
			case selectionFound:
				a.openTranslateText(answer.text)
			case selectionDenied:
				a.openTranslateText("")
				a.showSelectionHintOnce()
			default:
				a.openTranslateText("")
			}
		})
	})
}

// showSelectionHintOnce puts the accessibility hint under the input. It is a
// per-launch, in-memory marker: a denied read never opens a system window on
// its own and never repeats the hint during the same run.
func (a *application) showSelectionHintOnce() {
	if a.translateSelectionHintShown {
		return
	}
	a.translateSelectionHintShown = true
	s := a.ensureTranslateState()
	s.denied = true
	a.refreshTranslate()
}

// refreshSelectionTrust is called when the Translate settings page opens. The
// next shortcut press checks again, so returning from System Settings needs no
// polling, no observer and no prompt.
func (a *application) refreshSelectionTrust() {
	if a.selection == nil {
		return
	}
	a.translateSelectionTrusted = a.selection.Trusted()
}

// grantAccessibility is the "Open Settings" text button in the panel and on
// the settings row. One launch opens the pane at most once, so a denied read
// cannot make system windows pile up. Tests replace the opener seam.
func (a *application) grantAccessibility() {
	if s := a.translate; s != nil {
		s.denied = false
	}
	if !a.accessibilitySettingsOpened {
		a.accessibilitySettingsOpened = true
		if a.openAccessibilitySettings != nil {
			a.openAccessibilitySettings()
		}
	}
	a.refreshTranslate()
	a.invalidateWindow()
}

// openAccessibilitySettingsPane is the production opener. It is installed in
// ready(), never in a test, so headless tests cannot open a system window.
func openAccessibilitySettingsPane() {
	cmd := exec.Command("/usr/bin/open", accessibilitySettingsURL)
	if err := cmd.Start(); err == nil {
		go cmd.Wait()
	}
}

// startTranslateCLISelection handles bytes == 0: translate the current AX
// selection without opening a panel. Missing permission and an empty or slow
// read both answer TranslateNoSelection (exit 4), as the spec requires.
func (a *application) startTranslateCLISelection(ctx context.Context, req cli.Request, send func(cli.Result, []byte)) {
	a.runTranslateWorker(func() {
		answer := a.captureSelection()
		a.translateCLIMain(func() {
			if ctx.Err() != nil || a.quitting {
				return
			}
			if answer.trustedKnown {
				a.translateSelectionTrusted = answer.trusted
			}
			if answer.status != selectionFound || !cli.ValidTranslateInput([]byte(answer.text)) ||
				strings.TrimSpace(answer.text) == "" {
				result := cli.Failure("TranslateNoSelection")
				result.Message = a.text(uiLocale(), "TranslateNoSelection")
				send(result, nil)
				return
			}
			a.startTranslateCLI(ctx, req, []byte(answer.text), send)
		})
	})
}

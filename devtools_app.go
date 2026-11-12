package main

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"

	"github.com/ConteMan/cubbix/internal/devtools"
)

// defaultDevtoolsCopy is the clipboard write of a result. It returns the
// failure so the panel can keep itself open and say the copy did not happen.
func defaultDevtoolsCopy(text string) error {
	return mygo.Clipboard.Write(transfer.TextData(text))
}

// devtoolsSuggestion is the empty-query clipboard hint: only a whole hit
// becomes a group. A long text, an ordinary sentence or a prefixed input is
// never read again for this launch of the panel.
func devtoolsSuggestion(text string) []devtools.Row {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 128 {
		return nil
	}
	if output, ok := devtools.ConvertBareTimestamp(text, nil); ok {
		return output.Rows
	}
	if output, ok := devtools.ConvertDate(text, nil); ok && output.ErrorKey == "" {
		return output.Rows
	}
	if output, ok := devtools.ParseUUID(text); ok {
		return output.Rows
	}
	return nil
}

// changeDevtoolsEnabled is the module switch. This step has no shortcut and
// no window, so the switch only writes the flag, saves, and drops cached
// palette results and the clipboard suggestion.
func (a *application) changeDevtoolsEnabled(target bool) {
	a.cancelShortcutRecording()
	if target == a.config.DevtoolsEnabled {
		a.invalidateWindow()
		return
	}
	a.config.DevtoolsEnabled = target
	a.save()
	a.invalidateWindow()
	a.invalidatePaletteAfterModuleChange()
}

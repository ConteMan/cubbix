package main

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// menubarPanelState is the card panel's own state: the flyout while it shows,
// and the one thing the user can fold open, the JSON window list. A closed
// panel drops the state with its window, so every open starts from config.
type menubarPanelState struct {
	window   *mygo.Window
	expanded bool
	errorKey string
}

// menubarPanelRows is what the panel draws. The height calculation and the
// view read the same answer, so the flyout is exactly as tall as its content.
type menubarPanelRows struct {
	awake, json, empty bool
	status             bool // the awake card's status line
	documents          []*jsonDocument
	folded             bool // the JSON windows hide behind one count row
	listRows           int
}

// toggleMenubarPanel is the tray icon's left click: it opens the card panel,
// or closes it when it shows.
func (a *application) toggleMenubarPanel() {
	if a.shortcutRecording { // recording needs every key for itself
		return
	}
	if a.menubarPanelVisible() {
		a.hideMenubarPanel()
		return
	}
	a.showMenubarPanel()
}

// showMenubarPanel opens the panel under the icon. The palette closes first:
// the two panels never stack.
func (a *application) showMenubarPanel() {
	if a.shortcutRecording {
		return
	}
	a.hidePalette()
	if a.panel == nil {
		a.panel = &menubarPanelState{}
	}
	if a.panel.window == nil && a.createMenubarPanel != nil {
		a.createMenubarPanel(a.panel)
	}
	a.refreshMenubarPanel()
	if a.panel.window != nil {
		a.panel.window.Show()
	}
}

func (a *application) menubarPanelVisible() bool {
	return a.panel != nil
}

// buildMenubarPanel creates the flyout that hangs from the tray icon. It uses
// no system popover (Popover: false), so showing it never activates the app
// or pulls the Dock forward, and it joins every Space.
func (a *application) buildMenubarPanel(p *menubarPanelState) {
	if a.tray == nil {
		return
	}
	t := tokens(false, "zh-CN")
	window := mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:         a.tray,
		Placement:    mygo.PlacementBottom,
		Width:        int(t.SizeMenubarPanelWidth),
		Height:       a.menubarPanelHeight(t),
		Focusable:    true,
		Popover:      false,
		Shadow:       true,
		Vibrancy:     mygo.VibrancyPopover,
		CornerRadius: int(t.RadiusWindow),
		Hidden:       true,
		Content:      ui.View(a.menubarPanelView),
	})
	window.SetVisibleOnAllWorkspaces(true)
	window.OnClosed(func() {
		if a.panel == p {
			a.panel = nil
		}
	})
	p.window = window
}

// refreshMenubarPanel resizes the panel to its content and draws it again.
// Tile toggles, JSON window changes, keep awake changes and locale changes
// all land here.
func (a *application) refreshMenubarPanel() {
	p := a.panel
	if p == nil || p.window == nil {
		return
	}
	t := tokens(false, "zh-CN")
	p.window.SetContentSize(int(t.SizeMenubarPanelWidth), a.menubarPanelHeight(t))
	p.window.Invalidate()
}

// rows collects what the panel draws now.
func (a *application) menubarPanelRows() menubarPanelRows {
	rows := menubarPanelRows{}
	if a.config.AwakeEnabled {
		rows.awake = true
		state := a.awake.snapshot()
		rows.status = state.Active || state.Error != "" || a.menubarPanelErrorKey() != ""
	}
	if a.config.JSONEnabled {
		rows.json = true
		rows.documents = a.openJSONDocuments()
		switch {
		case len(rows.documents) == 0:
		case len(rows.documents) >= jsonWindowSubmenuAt && !a.panelExpanded():
			// One row names the count; the list stays folded by default.
			rows.folded, rows.listRows = true, 1
		case len(rows.documents) >= jsonWindowSubmenuAt:
			rows.listRows = len(rows.documents) + 1
		default:
			rows.listRows = len(rows.documents)
		}
	}
	rows.empty = !rows.awake && !rows.json
	return rows
}

func (a *application) panelExpanded() bool {
	return a.panel != nil && a.panel.expanded
}

func (a *application) openJSONDocuments() []*jsonDocument {
	documents := make([]*jsonDocument, 0, len(a.jsonDocuments))
	for _, d := range a.jsonDocuments {
		if !d.closed {
			documents = append(documents, d)
		}
	}
	return documents
}

// menubarPanelErrorKey is the failure the awake card reports: the last action's
// error, or the one the controller recorded, such as a target process ending.
func (a *application) menubarPanelErrorKey() string {
	if a.panel != nil && a.panel.errorKey != "" {
		return a.panel.errorKey
	}
	return a.awake.snapshot().Error
}

func awakeCardHeight(t tokenValues, status bool) float32 {
	height := 2*t.Space10 + t.SizeNavHeight + t.Space8 + t.SizeSegmentHeight + t.Space8 + t.SizeButtonHeight
	if status {
		height += t.Space8 + t.Space20
	}
	return height
}

func jsonCardHeight(t tokenValues, listRows int) float32 {
	height := 2*t.Space10 + t.SizeNavHeight + t.Space8 + t.SizeButtonHeight
	if listRows > 0 {
		height += float32(listRows)*t.SizeNavHeight + float32(listRows-1)*t.Space2 + t.Space8
	}
	return height
}

// menubarPanelHeight is the content height of the panel for the current rows.
// The flyout defaults to 200×200, so every change goes through SetContentSize.
func (a *application) menubarPanelHeight(t tokenValues) int {
	rows := a.menubarPanelRows()
	height := 2 * t.Space12
	cards := 0
	if rows.awake {
		height += awakeCardHeight(t, rows.status)
		cards++
	}
	if rows.json {
		height += jsonCardHeight(t, rows.listRows)
		cards++
	}
	if rows.empty {
		height += t.SizeNavHeight
		cards++
	}
	if cards > 1 {
		height += float32(cards-1) * t.Space8
	}
	// The footer: one hairline above the system actions.
	height += t.Space8 + t.StrokeHairline + t.SizeNavHeight
	return int(math.Ceil(float64(height)))
}

// --- Actions ---

// panelResult keeps a failed action's reason on the panel instead of the
// settings page, and draws the new height either way.
func (a *application) panelResult(err error) {
	if a.panel != nil {
		if err != nil {
			a.panel.errorKey = paletteErrorKey(err)
		} else {
			a.panel.errorKey = ""
		}
	}
	a.refreshMenubarPanel()
}

// panelToggleAwake is the card switch: stop when it runs, otherwise start the
// remembered mode and duration.
func (a *application) panelToggleAwake() {
	if a.awake.snapshot().Active {
		a.awake.stop()
		a.panelResult(nil)
		return
	}
	a.panelResult(a.startAwake(a.selectedSession()))
}

func (a *application) panelSetMode(mode int) {
	modes := []string{"screen", "tasks"}
	if mode < 0 || mode >= len(modes) {
		return
	}
	a.panelResult(a.setMode(modes[mode]))
}

// panelSetDuration starts keep awake at a preset, or resets a running one to
// it. Choosing the preset that already runs changes nothing.
func (a *application) panelSetDuration(minutes int) {
	if preset, ok := a.panelPreset(time.Now()); ok && preset == minutes && a.awake.snapshot().Active {
		return
	}
	a.panelResult(a.setDuration(minutes))
}

// panelPreset is the preset the pills highlight: the one in effect while
// keep awake runs, otherwise the remembered one. A deadline, a followed
// process, four hours and custom minutes highlight no pill.
func (a *application) panelPreset(now time.Time) (int, bool) {
	state := a.awake.snapshot()
	switch {
	case a.config.AwakeUntil != "":
		return 0, false
	case state.Active && state.Session.PID > 0:
		return 0, false
	case state.Active && state.Session.End.IsZero():
		return 0, true
	}
	switch a.config.AwakeMinutes {
	case 30, 60, 120:
		if state.Active {
			// A preset lasts its full length; anything longer is not it.
			if state.Session.End.Sub(now) > time.Duration(a.config.AwakeMinutes+1)*time.Minute {
				return 0, false
			}
		}
		return a.config.AwakeMinutes, true
	case 0:
		if state.Active && !state.Session.End.IsZero() {
			return 0, false
		}
		return 0, true
	}
	return 0, false
}

// panelToggleJSONWindows folds the JSON window list open or shut.
func (a *application) panelToggleJSONWindows() {
	if a.panel == nil {
		return
	}
	a.panel.expanded = !a.panel.expanded
	a.refreshMenubarPanel()
}

// panelNewJSON creates a window from the clipboard. The new window takes the
// focus, so the panel goes first.
func (a *application) panelNewJSON() {
	a.hideMenubarPanel()
	a.newJSONWindow()
}

// panelShowJSON brings one document forward and closes the panel.
func (a *application) panelShowJSON(d *jsonDocument) {
	a.hideMenubarPanel()
	a.showJSON(d)
}

// menubarPanelStatusLine is the awake card's running status, or "" when the
// card shows none.
func (a *application) menubarPanelStatusLine(locale string, now time.Time) string {
	state := a.awake.snapshot()
	if !state.Active || a.menubarPanelErrorKey() != "" {
		return ""
	}
	return a.awakeMode(locale, state.Session) + " · " + a.awakeSuffix(locale, state.Session, now)
}

// menubarPanelTooltip is the tooltip of a button that also has a shortcut.
func (a *application) menubarPanelTooltip(locale, key, accelerator string) string {
	label := a.text(locale, key)
	if accelerator == "" {
		return label
	}
	return fmt.Sprintf("%s (%s)", label, shortcutLabel(accelerator))
}

func shortcutLabel(accelerator string) string {
	label := ""
	for _, symbol := range shortcutSymbols(accelerator) {
		label += symbol
	}
	return label
}

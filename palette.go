package main

import (
	"errors"
	"fmt"
	"github.com/ConteMan/cubbix/internal/awake"
	"github.com/ConteMan/cubbix/internal/devtools"
	"log"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// paletteMode is what the panel lists: commands, or the actions of one.
type paletteMode int

const (
	paletteCommandMode paletteMode = iota
	paletteActionMode
)

type paletteRowKind int

const (
	paletteRowHeader paletteRowKind = iota
	paletteRowCommand
	paletteRowAction
	paletteRowActionTitle
	paletteRowSeparator
	paletteRowResult
)

// paletteRow is one line of the panel. Headers and separators are not chosen.
type paletteRow struct {
	Kind     paletteRowKind
	Key      string
	Title    string
	Group    string // resource key of a group header
	Category string // the tile a command belongs to, after its title
	Icon     string
	Hint     string
	ErrorKey string
	Checked  bool
	Command  *command
	Action   *paletteAction
	// Result is a devtools row. Its Payload is what Enter copies; the title
	// may only be a readable clock face.
	Result devtools.Row
}

// paletteState is the panel's own state. It lives on the main thread, like
// awakeDialog and jsonDocument.
type paletteState struct {
	window          *mygo.Window
	visible         bool
	query           string
	lastQuery       string
	mode            paletteMode
	errorKey        string
	runError        string
	rows            []paletteRow
	selected        int
	selectionKey    string
	request         request
	actionsCommand  *command
	actionsRequest  request
	actionSelection string
	scrollSelection bool
	list            ui.ListState
	// drag is the pointer drag in progress, if any.
	drag paletteDrag
	// rootBounds, panelBounds and grabBounds are the last frame's boxes of the
	// three drag areas, in window DIPs. A handler cannot read its element once
	// the frame that built it is over, so the boxes are kept.
	rootBounds  ui.Rect
	panelBounds ui.Rect
	grabBounds  ui.Rect
	// searchBounds and listBounds are the last frame's boxes of the search row
	// and the list, in window DIPs. A press inside either of them belongs to
	// them, not to the drag.
	searchBounds ui.Rect
	listBounds   ui.Rect
	// resultCache keeps one devtools result per query. The palette redraws
	// every frame, and regenerating a UUID there would copy a value the user
	// never saw. The cache is dropped when the panel reopens or a module
	// changes.
	resultCache map[string]devtoolsCacheEntry
	// suggestion is the empty-query clipboard group. It is read once when the
	// panel opens, never while it stays open.
	suggestion []devtools.Row
	// actionsResult is the ⌘K page of a result row, which has no command.
	actionsResult      bool
	actionsResultQuery string
	actionsResultUUID  bool
	actionsPayload     string
	actionsTitle       string
	actionsIcon        string
	// done stops a second Enter from executing the same result twice, and
	// resets on a query change or the next open.
	done bool
	// devSession changes when the panel opens or a module changes. Result rows
	// remember the session they were built in, so a row that outlives its
	// session cannot copy or open anything.
	devSession uint64
	// resultRequest is the complete generation request behind the current
	// result rows, so "open in window" never re-generates with the window's
	// defaults. actionsDevRequest is that request while the result action page
	// is open. resultUpper and resultPlain follow the presentation actions.
	resultRequest     devtoolsRequest
	actionsDevRequest devtoolsRequest
	resultUpper       bool
	resultPlain       bool
	// resultSession is the session the current result rows belong to.
	resultSession uint64
	// actionsResultSession is the session the open result action page
	// belongs to.
	actionsResultSession uint64
}

// devtoolsCacheEntry remembers both the result and whether this query was a
// devtools request at all, so an ordinary search is cached too.
type devtoolsCacheEntry struct {
	output     devtools.Output
	recognized bool
}

// togglePalette is the ⌥. entry and the card panel's palette button target.
func (a *application) togglePalette() {
	if a.shortcutRecording { // recording needs every key for itself
		return
	}
	if a.paletteVisible() {
		a.hidePalette()
		return
	}
	a.showPalette()
}

// hideMenubarPanel closes the menu bar card panel (001). Showing either panel
// closes the other, so the two never stack.
func (a *application) hideMenubarPanel() {
	if a.panel == nil {
		return
	}
	p := a.panel
	a.panel = nil
	if p.window != nil {
		p.window.Close()
	}
}

func (a *application) paletteVisible() bool {
	return a.palette != nil && a.palette.visible
}

// showPalette opens the command palette. It closes the card panel, which is
// another overlay, and leaves the tool windows alone: Translate and QR stay
// open, so a command can be chosen while they are visible.
func (a *application) showPalette() {
	a.hideMenubarPanel()
	if a.palette == nil {
		a.palette = &paletteState{}
	}
	if a.palette.window == nil {
		a.createPaletteWindow(a.palette)
	}
	a.palette.query = ""
	a.palette.lastQuery = ""
	a.palette.mode = paletteCommandMode
	a.palette.errorKey = ""
	a.palette.runError = ""
	a.palette.selectionKey = ""
	a.palette.actionsCommand = nil
	a.palette.actionsResult = false
	a.palette.actionSelection = ""
	a.palette.resultCache = nil
	a.palette.suggestion = nil
	a.palette.resultRequest = devtoolsRequest{}
	a.palette.resultUpper = false
	a.palette.resultPlain = false
	a.palette.done = false
	a.palette.devSession++
	a.palette.resultSession = 0
	a.palette.actionsResultSession = 0
	if a.config.DevtoolsEnabled && a.devtoolsReadClipboard != nil {
		a.palette.suggestion = devtoolsSuggestion(a.devtoolsReadClipboard())
	}
	a.palette.list.ScrollTo(0, ui.Start)
	a.presentPalette()
}

func (a *application) presentPalette() {
	if a.palette == nil {
		return
	}
	a.placePalette()
	a.palette.visible = true
	if a.palette.window != nil {
		a.palette.window.Show()
		a.palette.window.Invalidate()
	}
}

func (a *application) hidePalette() {
	a.lidPending = nil
	if a.palette == nil {
		return
	}
	a.palette.visible = false
	a.palette.drag = paletteDrag{}
	a.hidePaletteGuides()
	if a.palette.window != nil {
		a.palette.window.Hide()
	}
}

// invalidatePaletteAfterModuleChange refreshes a cached palette after a module
// switch: a cached action page of a command that no longer exists falls back to
// the command list, and the rows are rebuilt on the next frame.
func (a *application) invalidatePaletteAfterModuleChange() {
	p := a.palette
	if p == nil {
		return
	}
	if p.mode == paletteActionMode && p.actionsCommand != nil {
		if findCommand(a.commands(), p.actionsCommand.ID) == nil {
			p.mode = paletteCommandMode
			p.actionsCommand = nil
			p.actionsRequest = request{}
			p.runError = ""
			p.selectionKey = ""
			p.scrollSelection = true
		}
	}
	p.selectionKey = ""
	// A module change invalidates every cached devtools result and the
	// clipboard suggestion; a result page loses its command, and the next
	// frame rebuilds the rows.
	p.resultCache = nil
	p.suggestion = nil
	p.resultRequest = devtoolsRequest{}
	p.resultUpper = false
	p.resultPlain = false
	p.done = false
	p.devSession++
	p.resultSession = 0
	p.actionsResultSession = 0
	if p.mode == paletteActionMode && p.actionsResult {
		p.mode = paletteCommandMode
		p.actionsResult = false
		p.actionsResultQuery = ""
		p.actionsCommand = nil
		p.actionSelection = ""
		p.scrollSelection = true
	}
	a.invalidatePalette()
}

func (a *application) invalidatePalette() {
	if a.palette != nil && a.palette.window != nil {
		a.palette.window.Invalidate()
	}
}

// buildPaletteWindow builds the nonactivating panel. It is created on the
// first use so the app never opens a hidden window at launch. The window is
// the panel plus the shadow margin on every side: the shadow is drawn by the
// panel, and what falls outside the window would be cut off, so the system's
// own shadow stays off.
func (a *application) buildPaletteWindow(p *paletteState) {
	t := tokens(false, "zh-CN")
	margin := int(t.SizePaletteShadowMargin)
	window := mygo.NewWindow(mygo.WindowOptions{
		Title:             "Cubbix",
		Width:             int(t.SizePaletteWidth) + 2*margin,
		Height:            int(t.SizePaletteHeight) + 2*margin,
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
		Content:           ui.View(a.paletteView),
	})
	window.OnClose(func(event *mygo.CloseEvent) {
		if !a.quitting {
			event.PreventDefault()
			a.hidePalette()
		}
	})
	window.OnBlur(a.hidePalette)
	p.window = window
}

// placePalette opens the panel where the display under the pointer remembers
// it, in the default position when it remembers nothing, and always fully
// inside that display's work area.
func (a *application) placePalette() {
	p := a.palette
	if p == nil {
		return
	}
	size := a.palettePanelSize()
	display := displayNearest(a.paletteDisplaysNow(), a.paletteCursorPoint())
	position := a.paletteDefaultPoint(display)
	if stored, ok := (seatTable{seats: &a.config.PalettePositions}).lookup(display); ok {
		remembered := seatPoint(stored, display.WorkArea)
		// Clamp cannot fit an oversized panel. Keep the default fallback on
		// those axes so a formerly valid offset cannot leave it off-screen.
		if display.WorkArea.Width >= size.Width {
			position.X = remembered.X
		}
		if display.WorkArea.Height >= size.Height {
			position.Y = remembered.Y
		}
	}
	a.movePalettePanelTo(clampToWorkArea(position, size, display.WorkArea))
}

// palettePosition centers the panel on a display's work area, near its top,
// and keeps it fully on screen when the display is short.
func palettePosition(display mygo.Display, width, height int, topOffset float32) (int, int) {
	x := display.WorkArea.X + (display.WorkArea.Width-width)/2
	y := display.WorkArea.Y + int(topOffset)
	if y+height > display.WorkArea.Y+display.WorkArea.Height {
		y = max(display.WorkArea.Y, display.WorkArea.Y+display.WorkArea.Height-height)
	}
	return x, y
}

// registerPaletteShortcut registers ⌥. and reports conflicts with the other
// three slots in the settings row.
func (a *application) registerPaletteShortcut() {
	a.registerSlot("palette", a.config.PaletteShortcut, true, a.togglePalette)
}

// registerPanelShortcut registers ⌃⌥⌘P, the menu bar card panel's toggle.
func (a *application) registerPanelShortcut() {
	a.registerSlot("panel", a.config.PanelShortcut, true, a.toggleMenubarPanel)
}

// --- Selection and execution ---

func (a *application) paletteEscape() {
	p := a.palette
	if p == nil {
		return
	}
	if p.mode == paletteActionMode {
		p.mode = paletteCommandMode
		p.actionsCommand = nil
		p.actionsResult = false
		p.actionsResultQuery = ""
		p.actionSelection = ""
		p.scrollSelection = true
		a.invalidatePalette()
		return
	}
	a.hidePalette()
}

func (a *application) togglePaletteActions() {
	p := a.palette
	if p == nil {
		return
	}
	if p.mode == paletteActionMode {
		p.mode = paletteCommandMode
		p.actionsCommand = nil
		p.actionsResult = false
		p.actionsResultQuery = ""
		p.actionSelection = ""
		p.scrollSelection = true
		a.invalidatePalette()
		return
	}
	if row, ok := p.selectedRow(); ok && row.Kind == paletteRowResult {
		if !a.config.DevtoolsEnabled || p.resultSession != p.devSession || row.ErrorKey != "" {
			// A stale or error row has nothing to copy; ⌘K must not open an empty page.
			return
		}
		p.mode = paletteActionMode
		p.actionsCommand = nil
		p.actionsResult = true
		p.actionsResultQuery = p.lastQuery
		p.actionsResultSession = p.devSession
		p.actionsResultUUID = row.Result.Kind == devtools.KindUUID
		p.actionsPayload = row.Result.Payload
		p.actionsDevRequest = p.resultRequest
		if p.actionsResultUUID {
			p.actionsDevRequest.Upper = p.resultUpper
			p.actionsDevRequest.Plain = p.resultPlain
		}
		p.actionsTitle = row.Title
		p.actionsIcon = row.Icon
		p.actionSelection = ""
		p.list.ScrollTo(0, ui.Start)
		a.invalidatePalette()
		return
	}
	cmd := p.selectedCommand()
	if cmd == nil || cmd.Actions == nil {
		return
	}
	p.actionsCommand = cmd
	p.actionsRequest = p.request
	p.mode = paletteActionMode
	p.actionSelection = ""
	p.list.ScrollTo(0, ui.Start)
	a.invalidatePalette()
}

func (p *paletteState) selectedCommand() *command {
	if p.selected < 0 || p.selected >= len(p.rows) {
		return nil
	}
	return p.rows[p.selected].Command
}

func (p *paletteState) selectedAction() *paletteAction {
	if p.selected < 0 || p.selected >= len(p.rows) {
		return nil
	}
	return p.rows[p.selected].Action
}

func (p *paletteState) selectedRow() (paletteRow, bool) {
	if p.selected < 0 || p.selected >= len(p.rows) {
		return paletteRow{}, false
	}
	return p.rows[p.selected], true
}

func (a *application) movePaletteSelection(delta int) {
	p := a.palette
	if p == nil {
		return
	}
	for index := p.selected + delta; index >= 0 && index < len(p.rows); index += delta {
		row := p.rows[index]
		if row.Kind != paletteRowCommand && row.Kind != paletteRowAction && row.Kind != paletteRowResult {
			continue
		}
		if row.Action != nil && row.Action.Available != nil && !row.Action.Available(a) {
			continue
		}
		p.selected = index
		p.runError = ""
		if p.mode == paletteActionMode {
			p.actionSelection = row.Key
		} else {
			p.selectionKey = row.Key
		}
		p.list.ScrollIntoView(index)
		a.invalidatePalette()
		return
	}
}

// runPalette runs the selected command or action. A failure keeps the panel
// open with the query and shows an alert in place; a success closes it.
func (a *application) runPalette() {
	p := a.palette
	if p == nil {
		return
	}
	if p.done {
		// A second handler for the same Return must not run the result again.
		return
	}
	p.runError = ""
	if p.mode == paletteActionMode {
		if p.actionsResult {
			if !a.config.DevtoolsEnabled || p.actionsResultSession != p.devSession {
				a.invalidatePaletteAfterModuleChange()
				return
			}
			a.runPaletteResultAction(p)
			return
		}
		if p.actionsCommand != nil && findCommand(a.commands(), p.actionsCommand.ID) == nil {
			a.invalidatePaletteAfterModuleChange()
			return
		}
		if a.moduleClosing && p.actionsCommand != nil && p.actionsCommand.Tile == "awake" {
			p.runError, p.errorKey = "ModuleBusy", "ModuleBusy"
			a.invalidatePalette()
			return
		}
		action := p.selectedAction()
		if action == nil || action.Run == nil {
			return
		}
		if action.Available != nil && !action.Available(a) {
			return
		}
		// Favorites stay open: the user usually files several commands.
		if action.ID == "palette.favorite" || action.ID == "awake.start.lid" {
			if err := action.Run(a); err != nil {
				p.runError = paletteErrorKey(err)
				p.errorKey = p.runError
			}
			a.invalidatePalette()
			return
		}
		if err := action.Run(a); err != nil {
			p.runError = paletteErrorKey(err)
			p.errorKey = p.runError
			a.invalidatePalette()
			return
		}
		if p.actionsCommand != nil {
			a.recordRecent(p.actionsCommand.ID)
		}
		p.done = true
		a.hidePalette()
		return
	}
	if row, ok := p.selectedRow(); ok && row.Kind == paletteRowResult {
		if !a.config.DevtoolsEnabled || p.resultSession != p.devSession {
			// The module closed, or the panel session changed, after the row
			// was built.
			p.resultCache = nil
			p.selectionKey = ""
			a.invalidatePaletteAfterModuleChange()
			return
		}
		if row.ErrorKey != "" {
			p.errorKey = row.ErrorKey
			a.invalidatePalette()
			return
		}
		if err := a.copyDevtoolsPayload(row.Result.Payload); err != nil {
			p.runError, p.errorKey = "DevtoolsCopyFailed", "DevtoolsCopyFailed"
			a.invalidatePalette()
			return
		}
		p.done = true
		a.hidePalette()
		return
	}
	cmd := p.selectedCommand()
	if cmd == nil || cmd.Run == nil {
		return
	}
	// Rows are cached from the frame that built them. Re-read the registry so a
	// module closed in the meantime cannot run a stale closure.
	if live := findCommand(a.commands(), cmd.ID); live == nil {
		p.runError = ""
		p.selectionKey = ""
		a.invalidatePalette()
		return
	}
	if a.moduleClosing && cmd.Tile == "awake" {
		p.runError, p.errorKey = "ModuleBusy", "ModuleBusy"
		a.invalidatePalette()
		return
	}
	req := p.request
	if req.Command != cmd {
		req = request{Command: cmd, Awake: awake.Arg{Kind: awake.KindDefault}}
	}
	if req.Awake.Error != "" {
		// Invalid parameters never execute and never close the panel.
		p.errorKey = req.Awake.Error
		a.invalidatePalette()
		return
	}
	if err := cmd.Run(a, req); err != nil {
		p.runError = paletteErrorKey(err)
		p.errorKey = p.runError
		a.invalidatePalette()
		return
	}
	a.recordRecent(cmd.ID)
	p.done = true
	a.hidePalette()
}

// paletteErrorKey maps a command error to the key shown in place. Parameter
// errors keep their own text; everything else is a start failure.
func paletteErrorKey(err error) string {
	if err == nil {
		return ""
	}
	if strings.HasPrefix(err.Error(), "Lid") {
		return err.Error()
	}
	switch err.Error() {
	case "DurationError", "TimeError":
		return err.Error()
	}
	return "AwakeStartError"
}

func (a *application) isFavorite(id string) bool {
	return slices.Contains(a.config.PaletteFavorites, id)
}

func (a *application) toggleFavorite(id string) {
	favorites := append([]string(nil), a.config.PaletteFavorites...)
	for index, candidate := range favorites {
		if candidate == id {
			favorites = append(favorites[:index:index], favorites[index+1:]...)
			a.config.PaletteFavorites = normalizePaletteFavorites(favorites)
			a.save()
			return
		}
	}
	a.config.PaletteFavorites = normalizePaletteFavorites(append(favorites, id))
	a.save()
}

// recordRecent moves an executed command to the front, keeping five. JSON
// window commands are left out: they die with their document.
func (a *application) recordRecent(id string) {
	if strings.HasPrefix(id, "json.window.") {
		return
	}
	recents := []string{id}
	for _, candidate := range a.config.PaletteRecents {
		if candidate != id {
			recents = append(recents, candidate)
		}
	}
	recents = normalizePaletteRecents(recents)
	if slices.Equal(recents, a.config.PaletteRecents) {
		return
	}
	a.config.PaletteRecents = recents
	a.save()
}

// --- Row building ---

// refreshPalette rebuilds the visible rows and keeps the chosen command while
// it is still listed. Called once per frame; the registry is cheap.
func (a *application) refreshPalette(p *paletteState, locale string) {
	if p.mode == paletteActionMode {
		a.refreshPaletteActions(p, locale)
		return
	}
	commands := a.commands()
	previousSelection := p.selectionKey
	query := normalizeQuery(p.query)
	if query != p.lastQuery {
		// A new query chooses its own default; arrows keep a choice until the
		// text changes again.
		p.lastQuery = query
		p.selectionKey = ""
		p.runError = ""
		p.done = false
		p.resultUpper, p.resultPlain = false, false
		if query != "" {
			// Typing ends the clipboard group for this panel session; only a
			// fresh open reads the clipboard again.
			p.suggestion = nil
		}
	}
	prefix, matchedPrefix := splitCommandQuery(a, locale, query, commands)
	forced := matchedPrefix && prefix.Command.Param != paramNone && prefix.Arg != ""
	now := time.Now()

	argFor := func(cmd *command) awake.Arg {
		if cmd.Param != paramAwake {
			return awake.Arg{}
		}
		if matchedPrefix && prefix.Command == cmd {
			return awake.ParseArg(prefix.Arg, now)
		}
		return awake.Arg{Kind: awake.KindDefault}
	}
	commandRow := func(cmd *command) paletteRow {
		title, category := cmd.Title(a, locale), a.text(locale, commandCategory(cmd))
		if strings.EqualFold(strings.TrimSpace(title), strings.TrimSpace(category)) {
			// An unnamed JSON window is titled with its tile's own name: printing
			// that name twice says nothing the title has not said already.
			category = ""
		}
		row := paletteRow{Kind: paletteRowCommand, Key: cmd.ID, Title: title, Category: category, Icon: cmd.Icon, Command: cmd}
		if cmd.Param == paramAwake {
			arg := argFor(cmd)
			row.Hint = awakeHint(a, locale, arg)
			row.ErrorKey = arg.Error
			if arg.Error != "" {
				// An invalid argument shows the raw text, not a duration.
				row.Hint = prefix.Arg
			}
		}
		return row
	}
	usable := func(cmd *command) bool {
		return cmd.Available == nil || cmd.Available(a)
	}
	matches := func(cmd *command) bool {
		return query == "" || matchScore(query, cmd.Title(a, locale), cmd.Keywords) > 0
	}
	forced = forced && usable(prefix.Command)

	var resultRows []paletteRow
	p.resultSession = 0
	if a.config.DevtoolsEnabled && query != "" {
		if entry := a.devtoolsCache(p, query, now); entry.recognized {
			resultRows = a.devtoolsResultRows(locale, entry.output)
			p.resultSession = p.devSession
			p.resultRequest = devtoolsRequestForQuery(query)
		}
	}

	var rows []paletteRow
	if query == "" && len(p.suggestion) > 0 {
		rows = append(rows, paletteRow{Kind: paletteRowHeader, Group: "DevtoolsClipboardGroup"})
		rows = append(rows, a.devtoolsRows(locale, p.suggestion, "devtools.clipboard")...)
	}
	if len(resultRows) > 0 {
		// A certain result sits on top, but the matching commands stay below:
		// a JSON window named "now" must remain reachable.
		rows = append(rows, paletteRow{Kind: paletteRowHeader, Group: "PaletteResults"})
		rows = append(rows, resultRows...)
	}
	if forced {
		rows = append(rows, paletteRow{Kind: paletteRowHeader, Group: "PaletteAll"})
		rows = append(rows, commandRow(prefix.Command))
	} else {
		seen := map[string]bool{}
		appendGroup := func(key string, group []*command) {
			if len(group) == 0 {
				return
			}
			rows = append(rows, paletteRow{Kind: paletteRowHeader, Group: key})
			for _, cmd := range group {
				rows = append(rows, commandRow(cmd))
			}
		}
		var favorites, recents []*command
		for _, id := range a.config.PaletteFavorites {
			cmd := findCommand(commands, id)
			if cmd == nil || seen[id] || strings.HasPrefix(id, "json.window.") || !usable(cmd) || !matches(cmd) {
				continue
			}
			seen[id] = true
			favorites = append(favorites, cmd)
		}
		for _, id := range a.config.PaletteRecents {
			cmd := findCommand(commands, id)
			if cmd == nil || seen[id] || strings.HasPrefix(id, "json.window.") || !usable(cmd) || !matches(cmd) {
				continue
			}
			seen[id] = true
			recents = append(recents, cmd)
		}
		var rest []*command
		for _, cmd := range sortedCommands(a, locale, query, commands) {
			if seen[cmd.ID] || !usable(cmd) || !matches(cmd) {
				continue
			}
			rest = append(rest, cmd)
		}
		appendGroup("PaletteFavorites", favorites)
		appendGroup("PaletteRecent", recents)
		appendGroup("PaletteAll", rest)
	}
	p.rows = rows
	// Group labels scroll with their rows. A sticky header needs an opaque
	// backing to hide the rows under it; that would introduce a separate
	// surface over the translucent panel. Selection already skips headers.
	p.list.Header = nil

	// Keep the chosen command if it is still listed; otherwise choose the
	// default for this query.
	selected := -1
	if p.selectionKey != "" {
		for index, row := range rows {
			if (row.Kind == paletteRowCommand || row.Kind == paletteRowResult) && row.Key == p.selectionKey {
				selected = index
				break
			}
		}
	}
	if selected < 0 && len(resultRows) > 0 {
		// The recognized query defaults to its first result, never to a
		// same-named command further down.
		for index, row := range rows {
			if row.Kind == paletteRowResult {
				selected = index
				break
			}
		}
	}
	if selected < 0 {
		var wanted *command
		if forced {
			wanted = prefix.Command
		} else {
			wanted, _ = bestCommand(a, locale, query, commands)
		}
		if wanted != nil {
			for index, row := range rows {
				if row.Kind == paletteRowCommand && row.Command == wanted {
					selected = index
					break
				}
			}
		}
	}
	if selected < 0 {
		for index, row := range rows {
			if row.Kind == paletteRowCommand {
				selected = index
				break
			}
		}
	}
	p.selected = selected
	p.request = request{}
	p.errorKey = p.runError
	if selected >= 0 && selected < len(rows) {
		row := rows[selected]
		if row.Kind == paletteRowResult {
			p.selectionKey = row.Key
			if row.ErrorKey != "" {
				p.errorKey = row.ErrorKey
			}
		}
		cmd := row.Command
		if cmd != nil {
			p.selectionKey = cmd.ID
			arg := awake.Arg{}
			rawArg := ""
			if cmd.Param == paramAwake {
				arg = argFor(cmd)
				if matchedPrefix && prefix.Command == cmd {
					rawArg = prefix.Arg
				}
			}
			if cmd.Param == paramText && matchedPrefix && prefix.Command == cmd {
				// A text argument keeps the query's original case; only the
				// lowercase copy is used for matching.
				rawArg = prefix.RawArg
			}
			p.request = request{Command: cmd, Arg: rawArg, Awake: arg}
			if arg.Error != "" {
				p.errorKey = arg.Error
			}
		}
	}
	if selected >= 0 && (p.scrollSelection || p.selectionKey != previousSelection) {
		p.list.ScrollIntoView(selected)
		p.scrollSelection = false
	}
}

// devtoolsCache returns the cached result for one query, computing it only
// once. A UUID generated here is therefore the value the user sees and copies.
func (a *application) devtoolsCache(p *paletteState, query string, now time.Time) devtoolsCacheEntry {
	if p.resultCache == nil {
		p.resultCache = map[string]devtoolsCacheEntry{}
	}
	if entry, ok := p.resultCache[query]; ok {
		return entry
	}
	output, recognized := devtools.Query(query, now, time.Local)
	entry := devtoolsCacheEntry{output: output, recognized: recognized}
	p.resultCache[query] = entry
	return entry
}

// devtoolsResultRows turns one output into palette rows. The hint resource
// keys are formatted here, so the package stays free of interface text.
func (a *application) devtoolsResultRows(locale string, output devtools.Output) []paletteRow {
	if output.ErrorKey != "" {
		label := a.text(locale, output.ErrorKey)
		return []paletteRow{{
			Kind:     paletteRowResult,
			Key:      "devtools.error",
			Title:    label,
			ErrorKey: output.ErrorKey,
			Result:   devtools.Row{},
		}}
	}
	return a.devtoolsRows(locale, output.Rows, "devtools.result")
}

func (a *application) devtoolsRows(locale string, results []devtools.Row, keyPrefix string) []paletteRow {
	rows := make([]paletteRow, 0, len(results))
	for index, result := range results {
		icon := "clock-3"
		if result.Kind == devtools.KindUUID {
			icon = "blocks"
		}
		rows = append(rows, paletteRow{
			Kind:   paletteRowResult,
			Key:    fmt.Sprintf("%s.%d", keyPrefix, index),
			Title:  result.Title,
			Hint:   a.devtoolsHint(locale, result),
			Icon:   icon,
			Result: result,
		})
	}
	return rows
}

func (a *application) devtoolsHint(locale string, result devtools.Row) string {
	if result.HintKey == "" {
		return ""
	}
	message := a.text(locale, result.HintKey)
	if len(result.HintArgs) == 0 {
		return message
	}
	return fmt.Sprintf(message, result.HintArgs...)
}

func findCommand(commands []*command, id string) *command {
	for _, cmd := range commands {
		if cmd.ID == id {
			return cmd
		}
	}
	return nil
}

// refreshPaletteActions lists the actions of the command chosen before ⌘K.
func (a *application) refreshPaletteActions(p *paletteState, locale string) {
	p.errorKey = p.runError
	if p.actionsResult {
		a.refreshResultActions(p, locale)
		return
	}
	cmd := p.actionsCommand
	if cmd == nil {
		p.mode = paletteCommandMode
		return
	}
	req := p.actionsRequest
	if req.Command != cmd {
		req = p.request
	}
	if req.Command != cmd {
		req = request{Command: cmd, Awake: awake.Arg{Kind: awake.KindDefault}}
	}
	if p.errorKey == "" {
		p.errorKey = req.Awake.Error
	}
	var actions []paletteAction
	if cmd.Actions != nil {
		actions = cmd.Actions(a, req)
	}
	rows := []paletteRow{{Kind: paletteRowActionTitle, Key: "palette-action-title", Title: cmd.Title(a, locale), Icon: cmd.Icon}}
	if len(actions) > 0 {
		rows = append(rows, paletteRow{Kind: paletteRowSeparator})
		for group := 0; group <= 2; group++ {
			for index := range actions {
				action := &actions[index]
				if action.Group != group {
					continue
				}
				title := ""
				if action.Title != nil {
					title = action.Title(a, locale)
				}
				checked := action.Checked != nil && action.Checked(a)
				rows = append(rows, paletteRow{Kind: paletteRowAction, Key: action.ID, Title: title, Icon: action.Icon, Action: action, Checked: checked})
			}
		}
	}
	p.rows = rows
	p.list.Header = nil

	selected := -1
	if p.actionSelection != "" {
		for index, row := range rows {
			if row.Kind == paletteRowAction && row.Key == p.actionSelection {
				selected = index
				break
			}
		}
	}
	if selected < 0 {
		for index, row := range rows {
			if row.Kind != paletteRowAction {
				continue
			}
			if row.Action.Available != nil && !row.Action.Available(a) {
				continue
			}
			selected = index
			break
		}
	}
	p.selected = selected
	p.actionSelection = ""
	if selected >= 0 {
		p.actionSelection = rows[selected].Key
	}
}

// refreshResultActions lists what a result row can do: copy, and for a UUID
// row the two presentation changes. Uppercase and no hyphens transform the
// cached batch, so the copy is the same values the user saw.
func (a *application) refreshResultActions(p *paletteState, locale string) {
	payload := p.actionsPayload
	actions := []paletteAction{{
		ID:    "devtools.copy",
		Title: localizedTitle("DevtoolsCopy"),
		Icon:  "copy",
		Run: func(a *application) error {
			if err := a.copyDevtoolsPayload(payload); err != nil {
				return err
			}
			p.done = true
			a.hidePalette()
			return nil
		},
	}}
	if p.actionsResultUUID {
		actions = append(actions,
			paletteAction{
				ID:    "devtools.upper",
				Title: localizedTitle("DevtoolsUpper"),
				Run: func(a *application) error {
					a.transformDevtoolsResult(p.actionsResultQuery, true, false)
					return nil
				},
			},
			paletteAction{
				ID:    "devtools.plain",
				Title: localizedTitle("DevtoolsPlain"),
				Run: func(a *application) error {
					a.transformDevtoolsResult(p.actionsResultQuery, false, true)
					return nil
				},
			},
		)
	}
	request := p.actionsDevRequest
	actions = append(actions,
		paletteAction{
			ID:    "devtools.paste",
			Title: localizedTitle("DevtoolsPaste"),
			Icon:  "external-link",
			Run: func(a *application) error {
				if err := a.pasteDevtoolsPayload(payload); err != nil {
					return err
				}
				p.done = true
				return nil
			},
		},
		paletteAction{
			ID:    "devtools.open",
			Title: localizedTitle("DevtoolsOpenWindow"),
			Icon:  "blocks",
			Run: func(a *application) error {
				p.done = true
				a.openDevtools(request)
				return nil
			},
		},
	)
	rows := []paletteRow{{Kind: paletteRowActionTitle, Key: "palette-action-title", Title: p.actionsTitle, Icon: p.actionsIcon}}
	rows = append(rows, paletteRow{Kind: paletteRowSeparator})
	for index := range actions {
		action := &actions[index]
		title := ""
		if action.Title != nil {
			title = action.Title(a, locale)
		}
		rows = append(rows, paletteRow{Kind: paletteRowAction, Key: action.ID, Title: title, Icon: action.Icon, Action: action})
	}
	p.rows = rows
	p.list.Header = nil
	selected := 0
	if p.actionSelection != "" {
		for index, row := range rows {
			if row.Kind == paletteRowAction && row.Key == p.actionSelection {
				selected = index
				break
			}
		}
	}
	if selected < len(rows) && rows[selected].Kind != paletteRowAction {
		for index, row := range rows {
			if row.Kind == paletteRowAction {
				selected = index
				break
			}
		}
	}
	if selected < 0 || selected >= len(rows) {
		p.selected = -1
		return
	}
	p.selected = selected
	p.actionSelection = ""
	p.actionSelection = rows[selected].Key
}

// runPaletteResultAction applies one action of a result row. Copy closes the
// panel; the two presentation changes return to the same row.
func (a *application) runPaletteResultAction(p *paletteState) {
	if !a.config.DevtoolsEnabled || p.actionsResultSession != p.devSession {
		a.invalidatePaletteAfterModuleChange()
		return
	}
	action := p.selectedAction()
	if action == nil || action.Run == nil {
		return
	}
	if err := action.Run(a); err != nil {
		p.runError, p.errorKey = "DevtoolsCopyFailed", "DevtoolsCopyFailed"
		a.invalidatePalette()
		return
	}
	switch action.ID {
	case "devtools.copy", "devtools.paste", "devtools.open":
		// Copy closes the panel, and paste and open close it themselves.
		return
	}
	p.mode = paletteCommandMode
	p.actionsResult = false
	p.actionsResultQuery = ""
	p.actionsCommand = nil
	p.actionSelection = ""
	p.scrollSelection = true
	a.invalidatePalette()
}

// transformDevtoolsResult changes the cached UUID rows of one query. It never
// generates a new value: the batch stays the one the user was looking at.
func (a *application) transformDevtoolsResult(query string, upper, plain bool) {
	if a.palette == nil || a.palette.resultCache == nil {
		return
	}
	entry, ok := a.palette.resultCache[query]
	if !ok {
		return
	}
	for index := range entry.output.Rows {
		row := &entry.output.Rows[index]
		if row.Kind != devtools.KindUUID {
			continue
		}
		text := row.Payload
		if upper {
			text = strings.ToUpper(text)
		}
		if plain {
			text = strings.ReplaceAll(text, "-", "")
		}
		row.Payload = text
		row.Title = strings.Split(text, "\n")[0]
	}
	if len(entry.output.Rows) > 0 {
		entry.output.Payload = entry.output.Rows[0].Payload
	}
	a.palette.resultCache[query] = entry
	a.palette.resultUpper, a.palette.resultPlain = upper, plain
}

// paletteHasActions reports whether the selected row has an action page. A
// result error has none, and an empty selection shows no keycaps either.
func (a *application) paletteHasActions(p *paletteState) bool {
	if p.mode == paletteActionMode {
		return true
	}
	row, ok := p.selectedRow()
	if !ok {
		return false
	}
	switch row.Kind {
	case paletteRowCommand:
		return row.Command != nil && row.Command.Actions != nil
	case paletteRowResult:
		return row.ErrorKey == ""
	}
	return false
}

// pastePaletteResult is ⌘回车 on a result row: copy the payload and paste it to
// the application that was in front. A row that copies nothing pastes nothing.
func (a *application) pastePaletteResult() {
	p := a.palette
	if p == nil || p.done {
		return
	}
	payload := ""
	switch {
	case p.mode == paletteActionMode && p.actionsResult:
		if !a.config.DevtoolsEnabled || p.actionsResultSession != p.devSession {
			a.invalidatePaletteAfterModuleChange()
			return
		}
		payload = p.actionsPayload
	case p.mode == paletteCommandMode:
		row, ok := p.selectedRow()
		if !ok || row.Kind != paletteRowResult || row.ErrorKey != "" || row.Result.Payload == "" {
			return
		}
		if !a.config.DevtoolsEnabled || p.resultSession != p.devSession {
			a.invalidatePaletteAfterModuleChange()
			return
		}
		payload = row.Result.Payload
	default:
		return
	}
	if err := a.pasteDevtoolsPayload(payload); err != nil {
		p.runError, p.errorKey = "DevtoolsCopyFailed", "DevtoolsCopyFailed"
		a.invalidatePalette()
		return
	}
	p.done = true
	a.hidePalette()
}

func (a *application) copyDevtoolsPayload(payload string) error {
	if a.devtoolsCopy == nil {
		return nil
	}
	return a.devtoolsCopy(payload)
}

// --- cubbix:// URLs ---

// paletteURLRequest is the validated intent of a deep link. Nothing here has
// run yet; parsing only drops malformed or unavailable requests.
type paletteURLRequest struct {
	Lid, HasLid bool
	Kind        string // awake, awake.stop, awake.toggle, json.new, palette, settings
	Minutes     int
	HasMinutes  bool
	Mode        string
	Text        string
	Blank       bool
	Page        int
	// Arg and Follow carry what a URL cannot express: a duration, a deadline or
	// a followed process typed at the command line. ParsePaletteURL never sets
	// them; only the CLI channel does.
	Arg    string
	Follow int
	// CLI marks a request from the command line tool: same actions, but the lid
	// rules and the quiet default differ from a URL's.
	CLI bool
}

// parsePaletteURL validates a cubbix:// URL. Host and path are case-folded
// and stripped of leading and trailing slashes before the whitelist check;
// query values are untrusted input.
func parsePaletteURL(raw string, awakeEnabled, jsonEnabled bool, jsonLimit int) (paletteURLRequest, bool) {
	// A percent-encoded byte takes at most three bytes. Bound work before
	// parsing, including ignored query fields, and reject invalid text.
	if jsonLimit < 0 || len(raw) > 3*jsonLimit+4096 || !utf8.ValidString(raw) {
		return paletteURLRequest{}, false
	}
	target, err := url.Parse(raw)
	if err != nil {
		return paletteURLRequest{}, false
	}
	if !strings.EqualFold(target.Scheme, "cubbix") || target.Opaque != "" || target.User != nil || target.Fragment != "" || strings.Contains(target.Host, ":") {
		return paletteURLRequest{}, false
	}
	host := strings.ToLower(strings.Trim(target.Host, "/"))
	path := strings.ToLower(strings.Trim(target.Path, "/"))
	// URL.Query silently discards malformed pairs; never partially execute a
	// request whose query could not be decoded in full. First values still win.
	query, err := url.ParseQuery(target.RawQuery)
	if err != nil {
		return paletteURLRequest{}, false
	}
	switch host {
	case "awake":
		if path != "" && path != "stop" && path != "toggle" {
			return paletteURLRequest{}, false
		}
		if !awakeEnabled {
			return paletteURLRequest{}, false
		}
		request := paletteURLRequest{Kind: "awake"}
		if path != "" {
			request.Kind += "." + path
		}
		if values, ok := query["lid"]; ok && len(values) > 0 {
			if values[0] != "0" && values[0] != "1" {
				return paletteURLRequest{}, false
			}
			request.HasLid = true
			request.Lid = values[0] == "1"
		}
		if values, ok := query["minutes"]; ok && len(values) > 0 {
			minutes, err := strconv.Atoi(values[0])
			if err != nil || minutes < 0 || minutes > awake.MinutesMaximum {
				return paletteURLRequest{}, false
			}
			request.Minutes, request.HasMinutes = minutes, true
		}
		if values, ok := query["mode"]; ok && len(values) > 0 {
			mode := values[0]
			if mode != "screen" && mode != "tasks" {
				return paletteURLRequest{}, false
			}
			request.Mode = mode
		}
		if request.Lid && (!request.HasMinutes || request.Minutes < 1 || request.Minutes > 480) {
			return paletteURLRequest{}, false
		}
		return request, true
	case "json":
		if path != "new" || !jsonEnabled {
			return paletteURLRequest{}, false
		}
		text := query.Get("text")
		if len(text) > jsonLimit || !utf8.ValidString(text) {
			return paletteURLRequest{}, false
		}
		if values, ok := query["blank"]; ok && values[0] != "1" {
			return paletteURLRequest{}, false
		}
		request := paletteURLRequest{Kind: "json.new", Text: text, Blank: query.Get("blank") == "1"}
		if _, present := query["text"]; present {
			// text wins over blank when both are present.
			request.Blank = false
		}
		return request, true
	case "palette":
		if path != "" {
			return paletteURLRequest{}, false
		}
		return paletteURLRequest{Kind: "palette"}, true
	case "settings":
		if path != "" {
			return paletteURLRequest{}, false
		}
		// "tiles" stays an alias for the keep awake page: the value is part of
		// the URL contract even though the interface calls it a module now.
		page := settingsGeneral
		switch strings.ToLower(query.Get("page")) {
		case "awake", "tiles":
			page = settingsAwake
		case "json":
			page = settingsJSON
		case "translate":
			page = settingsTranslate
		case "qr":
			page = settingsQR
		case "devtools":
			page = settingsDevtools
		case "shortcuts":
			page = settingsShortcuts
		case "about":
			page = settingsAbout
		}
		return paletteURLRequest{Kind: "settings", Page: page}, true
	}
	return paletteURLRequest{}, false
}

// handleURL decodes potentially large text on a worker, then dispatches only
// the validated intent to the UI thread. Availability is checked at delivery,
// since a tile may have been disabled while decoding.
func (a *application) handleURL(raw string) {
	if len(raw) > 3*jsonFileLimit+4096 {
		return
	}
	go func() {
		request, ok := parsePaletteURL(raw, true, true, jsonFileLimit)
		if !ok {
			return
		}
		a.urlDispatch(func() {
			// A deep link stays silent: a module that is off, or is closing,
			// drops the request instead of opening the settings window.
			if a.quitting || strings.HasPrefix(request.Kind, "awake") && (!a.config.AwakeEnabled || a.moduleClosing) || request.Kind == "json.new" && !a.config.JSONEnabled {
				return
			}
			if err := a.runURL(request); err != nil {
				// Never log URL text or leak external input into Settings.
				log.Print("Cubbix URL action failed")
			}
		})
	}()
}

// awakeLidGate decides whether this request may use the lid helper without
// ever asking for authorization. URLs keep their original rule; the command
// line accepts a deadline or a followed process instead of minutes.
func (a *application) awakeLidGate(req paletteURLRequest, session awakeSession, origin string) error {
	if !session.Lid {
		return nil
	}
	if origin == "url" {
		if a.lid == nil || !a.lid.Available() || a.lid.service.Status() != svcEnabled || !req.HasMinutes || req.Minutes < 1 || req.Minutes > 480 {
			return fmt.Errorf("LidAuthNeeded")
		}
		return nil
	}
	if a.lid == nil || !a.lid.Available() {
		// lidReason is empty when there is no controller at all; name the reason
		// rather than returning an empty error key.
		reason := a.lidReason()
		if reason == "" {
			reason = "LidUnsupported"
		}
		return errors.New(reason)
	}
	if a.lid.service.Status() != svcEnabled {
		return errors.New("LidAuthNeeded")
	}
	// Only a lid the request asked for needs a stated deadline; the remembered
	// preference follows the shortcut, which needs none.
	if req.HasLid && req.Lid && !lidDurationStated(session, time.Now()) {
		return errors.New("LidNeedsMinutes")
	}
	return nil
}

// lidDurationStated reports whether the user asked for a deadline the lid cap
// allows, or for a process to follow.
func lidDurationStated(session awakeSession, now time.Time) bool {
	switch {
	case session.PID > 0:
		return true
	case session.Minutes >= 1 && session.Minutes <= 480:
		return true
	case session.Until != "" && !session.End.IsZero():
		return !session.End.After(now.Add(480 * time.Minute))
	}
	return false
}

func (a *application) runURL(req paletteURLRequest) error {
	// A close in flight keeps the committed state. A new awake request must not
	// race it, but a settings page or another module may still open.
	if a.moduleClosing && strings.HasPrefix(req.Kind, "awake") {
		return errors.New("ModuleBusy")
	}
	switch req.Kind {
	case "awake":
		session := a.selectedSession()
		if req.Mode != "" {
			session.Mode = req.Mode
		}
		switch {
		case req.Follow > 0:
			// A followed process carries its own name and identity, read the way
			// the controller reads them, or the session would start and stop.
			process, ok := processByPID(req.Follow)
			if !ok {
				return errors.New("ProcessEnded")
			}
			session = awakeSession{Mode: session.Mode, PID: req.Follow, Identity: process.Identity, Name: process.Name}
		case req.Arg != "":
			arg := awake.ParseArg(req.Arg, time.Now())
			built, err := a.buildAwakeSession(arg, session.Mode)
			if err != nil {
				return err
			}
			session = built
		case req.HasMinutes:
			session = awakeSession{Mode: session.Mode, Minutes: req.Minutes}
			if req.Minutes > 0 {
				session.End = time.Now().Add(time.Duration(req.Minutes) * time.Minute)
			}
		}
		wanted := a.config.AwakeLid
		if req.HasLid {
			wanted = req.Lid
		}
		session.Lid = wanted
		origin := "url"
		if req.CLI {
			origin = "cli"
		}
		if err := a.awakeLidGate(req, session, origin); err != nil {
			return err
		}
		if !wanted && a.awake.snapshot().Session.Lid {
			if err := a.lid.stop(); err != nil {
				return err
			}
			a.setSessionLid(false)
		}
		a.lidURL = true
		defer func() { a.lidURL = false }()
		// The request never writes the remembered duration or deadline.
		err := a.startAwake(session)
		// A deep link has no other visible result, so it reports like a shortcut.
		if !wanted || err != nil {
			a.announceAwake(err)
		}
		return err
	case "awake.stop":
		err := a.stopAwake()
		a.announceAwake(err)
		return err
	case "awake.toggle":
		// Kept for cubbix://awake/toggle only. It is not the shortcut's path: a
		// remembered lid preference makes this branch answer LidAuthNeeded
		// because the URL carries no minutes. The command line's toggle goes
		// through runCLIToggle and toggleAwakeResult instead (006 §4.2), and a
		// test pins this behavior so nobody "unifies" it by accident.
		if a.awake.snapshot().Active {
			return a.runURL(paletteURLRequest{Kind: "awake.stop"})
		}
		req.Kind = "awake"
		return a.runURL(req)
	case "json.new":
		a.openJSONText(req.Text)
	case "palette":
		a.showPalette()
	case "settings":
		a.selectSettingsPage(req.Page)
		a.show()
	}
	return nil
}

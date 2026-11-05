package main

import (
	"errors"
	"fmt"
	"github.com/ConteMan/cubbix/internal/awake"
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
	a.palette.actionSelection = ""
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
	display := paletteDisplayNearest(a.paletteDisplaysNow(), a.paletteCursorPoint())
	position := a.paletteDefaultPoint(display)
	if seat, ok := a.config.PalettePositions[paletteSeatKeyFor(display)]; ok {
		remembered := palettePanelFromSeat(seat, display.WorkArea)
		// Clamp cannot fit an oversized panel. Keep the default fallback on
		// those axes so a formerly valid offset cannot leave it off-screen.
		if display.WorkArea.Width >= a.paletteSize().Width {
			position.X = remembered.X
		}
		if display.WorkArea.Height >= a.paletteSize().Height {
			position.Y = remembered.Y
		}
	}
	a.movePalettePanelTo(paletteClamp(position, a.paletteSize(), display.WorkArea))
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
		p.actionSelection = ""
		p.scrollSelection = true
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

func (a *application) movePaletteSelection(delta int) {
	p := a.palette
	if p == nil {
		return
	}
	for index := p.selected + delta; index >= 0 && index < len(p.rows); index += delta {
		row := p.rows[index]
		if row.Kind != paletteRowCommand && row.Kind != paletteRowAction {
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
	p.runError = ""
	if p.mode == paletteActionMode {
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

	var rows []paletteRow
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
			if row.Kind == paletteRowCommand && row.Key == p.selectionKey {
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
		cmd := rows[selected].Command
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

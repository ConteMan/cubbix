package main

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

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
	query           string
	lastQuery       string
	mode            paletteMode
	errorKey        string
	rows            []paletteRow
	selected        int
	selectionKey    string
	request         request
	actionsCommand  *command
	actionsRequest  request
	actionSelection string
	list            ui.ListState
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

// hideMenubarPanel closes the menu bar card panel. That panel is implemented
// in step 2 (001); this no-op keeps the two-panel mutex explicit until then,
// so showing the palette never stacks on a panel that does not exist yet.
func (a *application) hideMenubarPanel() {}

func (a *application) paletteVisible() bool {
	return a.palette != nil && a.palette.window != nil && a.palette.window.IsVisible()
}

func (a *application) showPalette() {
	a.hideMenubarPanel()
	if a.palette == nil {
		a.palette = &paletteState{}
	}
	if a.palette.window == nil {
		a.createPaletteWindow()
	}
	a.palette.query = ""
	a.palette.lastQuery = ""
	a.palette.mode = paletteCommandMode
	a.palette.errorKey = ""
	a.palette.selectionKey = ""
	a.palette.actionsCommand = nil
	a.palette.actionSelection = ""
	a.palette.list.ScrollTo(0, ui.Start)
	a.presentPalette()
}

func (a *application) presentPalette() {
	if a.palette == nil || a.palette.window == nil {
		return
	}
	a.placePalette()
	a.palette.window.Show()
	a.palette.window.Invalidate()
}

func (a *application) hidePalette() {
	if a.palette == nil || a.palette.window == nil {
		return
	}
	a.palette.window.Hide()
}

func (a *application) invalidatePalette() {
	if a.palette != nil && a.palette.window != nil {
		a.palette.window.Invalidate()
	}
}

// createPaletteWindow builds the nonactivating panel. It is created on the
// first use so the app never opens a hidden window at launch.
func (a *application) createPaletteWindow() {
	t := tokens(false, "zh-CN")
	window := mygo.NewWindow(mygo.WindowOptions{
		Title:             "Cubbix",
		Width:             int(t.SizePaletteWidth),
		Height:            int(t.SizePaletteHeight),
		Hidden:            true,
		Frameless:         true,
		Transparent:       true,
		DisableResize:     true,
		DisableMove:       true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		Panel:             true,
		Content:           ui.View(a.paletteView),
	})
	window.OnClose(func(event *mygo.CloseEvent) {
		event.PreventDefault()
		a.hidePalette()
	})
	window.OnBlur(a.hidePalette)
	a.palette.window = window
}

// placePalette centers the panel on the display under the pointer, near the
// top of its work area.
func (a *application) placePalette() {
	if a.palette == nil || a.palette.window == nil {
		return
	}
	display := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	windowWidth, windowHeight := a.palette.window.Size()
	x, y := palettePosition(display, windowWidth, windowHeight, tokens(false, "zh-CN").SizePaletteTopOffset)
	a.palette.window.SetPosition(x, y)
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
// two slots in the settings row.
func (a *application) registerPaletteShortcut() {
	a.paletteShortcutError = ""
	if reason := a.shortcutConflict(a.config.PaletteShortcut, "palette"); reason != "" {
		a.paletteShortcutError = reason
		return
	}
	if err := a.registerShortcut(a.config.PaletteShortcut, a.togglePalette); err != nil {
		a.paletteShortcutError = "ShortcutUnavailable"
	}
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
	if p.mode == paletteActionMode {
		action := p.selectedAction()
		if action == nil || action.Run == nil {
			return
		}
		if action.Available != nil && !action.Available(a) {
			return
		}
		// Favorites stay open: the user usually files several commands.
		if action.ID == "palette.favorite" {
			if err := action.Run(a); err != nil {
				p.errorKey = paletteErrorKey(err)
			}
			a.invalidatePalette()
			return
		}
		a.hidePalette()
		if err := action.Run(a); err != nil {
			p.errorKey = paletteErrorKey(err)
			a.presentPalette()
		}
		return
	}
	cmd := p.selectedCommand()
	if cmd == nil || cmd.Run == nil {
		return
	}
	req := p.request
	if req.Command != cmd {
		req = request{Command: cmd, Awake: awakeArg{Kind: awakeArgDefault}}
	}
	if req.Awake.Error != "" {
		// Invalid parameters never execute and never close the panel.
		p.errorKey = req.Awake.Error
		a.invalidatePalette()
		return
	}
	a.hidePalette()
	if err := cmd.Run(a, req); err != nil {
		p.errorKey = paletteErrorKey(err)
		a.presentPalette()
		return
	}
	a.recordRecent(cmd.ID)
}

// paletteErrorKey maps a command error to the key shown in place. Parameter
// errors keep their own text; everything else is a start failure.
func paletteErrorKey(err error) string {
	if err == nil {
		return ""
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
	query := normalizeQuery(p.query)
	if query != p.lastQuery {
		// A new query chooses its own default; arrows keep a choice until the
		// text changes again.
		p.lastQuery = query
		p.selectionKey = ""
	}
	prefix, matchedPrefix := splitCommandQuery(a, locale, query, commands)
	forced := matchedPrefix && prefix.Command.Param != paramNone && prefix.Arg != ""
	now := time.Now()

	argFor := func(cmd *command) awakeArg {
		if cmd.Param != paramAwake {
			return awakeArg{}
		}
		if matchedPrefix && prefix.Command == cmd {
			return parseAwakeArg(prefix.Arg, now)
		}
		return awakeArg{Kind: awakeArgDefault}
	}
	commandRow := func(cmd *command) paletteRow {
		row := paletteRow{Kind: paletteRowCommand, Key: cmd.ID, Title: cmd.Title(a, locale), Icon: cmd.Icon, Command: cmd}
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
	p.list.Header = func(index int) bool {
		return index >= 0 && index < len(p.rows) && p.rows[index].Kind == paletteRowHeader
	}

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
	p.errorKey = ""
	if selected >= 0 && selected < len(rows) {
		cmd := rows[selected].Command
		if cmd != nil {
			p.selectionKey = cmd.ID
			arg := awakeArg{}
			rawArg := ""
			if cmd.Param == paramAwake {
				arg = argFor(cmd)
				if matchedPrefix && prefix.Command == cmd {
					rawArg = prefix.Arg
				}
			}
			p.request = request{Command: cmd, Arg: rawArg, Awake: arg}
			p.errorKey = arg.Error
		}
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
		req = request{Command: cmd, Awake: awakeArg{Kind: awakeArgDefault}}
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
	Kind       string // awake, awake.stop, awake.toggle, json.new, palette, settings
	Minutes    int
	HasMinutes bool
	Mode       string
	Text       string
	Blank      bool
	Page       int
}

// parsePaletteURL validates a cubbix:// URL. Host and path are case-folded
// and stripped of leading and trailing slashes before the whitelist check;
// query values are untrusted input.
func parsePaletteURL(raw string, awakeEnabled, jsonEnabled bool, jsonLimit int) (paletteURLRequest, bool) {
	target, err := url.Parse(raw)
	if err != nil {
		return paletteURLRequest{}, false
	}
	if !strings.EqualFold(target.Scheme, "cubbix") {
		return paletteURLRequest{}, false
	}
	host := strings.ToLower(strings.Trim(target.Host, "/"))
	path := strings.ToLower(strings.Trim(target.Path, "/"))
	query := target.Query()
	switch host {
	case "awake":
		if path != "" && path != "stop" && path != "toggle" {
			return paletteURLRequest{}, false
		}
		if !awakeEnabled {
			return paletteURLRequest{}, false
		}
		switch path {
		case "stop":
			return paletteURLRequest{Kind: "awake.stop"}, true
		case "toggle":
			return paletteURLRequest{Kind: "awake.toggle"}, true
		}
		request := paletteURLRequest{Kind: "awake"}
		if values, ok := query["minutes"]; ok && len(values) > 0 {
			minutes, err := strconv.Atoi(values[0])
			if err != nil || minutes < 0 || minutes > awakeMinutesMaximum {
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
		return request, true
	case "json":
		if path != "new" || !jsonEnabled {
			return paletteURLRequest{}, false
		}
		text := query.Get("text")
		if len(text) > jsonLimit {
			return paletteURLRequest{}, false
		}
		request := paletteURLRequest{Kind: "json.new", Text: text, Blank: query.Get("blank") == "1"}
		if text != "" {
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
		page := 0
		switch strings.ToLower(query.Get("page")) {
		case "tiles":
			page = 2
		case "shortcuts":
			page = 3
		case "about":
			page = 1
		}
		return paletteURLRequest{Kind: "settings", Page: page}, true
	}
	return paletteURLRequest{}, false
}

// handleURL receives cubbix:// links from the system. The callback reaches
// the main thread first: the palette state is main-thread only.
func (a *application) handleURL(raw string) {
	request, ok := parsePaletteURL(raw, a.config.AwakeEnabled, a.config.JSONEnabled, jsonFileLimit)
	if !ok {
		return
	}
	a.runURL(request)
}

func (a *application) runURL(req paletteURLRequest) {
	switch req.Kind {
	case "awake":
		session := a.selectedSession()
		if req.Mode != "" {
			session.Mode = req.Mode
		}
		if req.HasMinutes {
			session = awakeSession{Mode: session.Mode}
			if req.Minutes > 0 {
				session.End = time.Now().Add(time.Duration(req.Minutes) * time.Minute)
			}
		}
		// The request never writes the remembered duration or deadline.
		_ = a.startAwake(session)
	case "awake.stop":
		a.awake.stop()
	case "awake.toggle":
		// The shortcut path reports failures in Settings; a deep link does not.
		if a.awake.snapshot().Active {
			a.awake.stop()
		} else {
			_ = a.startAwake(a.selectedSession())
		}
	case "json.new":
		a.openJSONText(req.Text)
	case "palette":
		a.showPalette()
	case "settings":
		a.page, a.notesPage = req.Page, false
		a.invalidateWindow()
		a.show()
	}
}

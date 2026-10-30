package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type awakeDialog struct {
	kind                          string
	window                        *mygo.Window
	hours, minutes, until, search string
	processes                     []runningProcess
	icons                         map[int]*ui.Bitmap
	selected                      int
	errorKey                      string
	loading                       bool
	filtered                      []runningProcess
	filteredQuery                 string
	filterReady                   bool
	searchDue                     time.Time
	list                          ui.ListState
	results                       chan processListResult
	cancelLoad                    func()
	iconCache                     *processIconCache
}

func (a *application) openAwakeDialog(kind string) {
	a.closeAwakeDialog()
	d := &awakeDialog{kind: kind, hours: "1", minutes: "30", until: "02:30", icons: map[int]*ui.Bitmap{}}
	a.dialog = d
	a.createAwakeDialogWindow(d)
	if kind == "follow" {
		d.iconCache = pickerIcons
		d.loadProcesses(listProcesses, strings.Split(a.config.AwakeAgents, ","), d.window.Invalidate)
	}
	// This is reached only by an explicit user action, never by tests or startup.
	d.window.Show()
	d.window.Focus()
}

func (a *application) createAwakeDialogWindow(d *awakeDialog) {
	t := tokens(false, "zh-CN")
	height := int(t.SizeRowHeight + 2*t.Space16 + t.FontSizeSmall*t.LineHeightBody + t.Space8)
	if d.kind == "follow" {
		height = int(t.SizeR05SettingsHeight + t.SizeRowHeight)
		d.loading = true
	}
	d.window = mygo.NewWindow(mygo.WindowOptions{Title: "Cubbix", Width: int(t.SizeDialogWidth), Height: height, Hidden: true, Frameless: true, DisableResize: true, DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true, Content: ui.View(a.awakeDialogView)})
	d.window.OnClosed(func() {
		if d.cancelLoad != nil {
			d.cancelLoad()
		}
		if a.dialog == d {
			a.dialog = nil
		}
	})
}

func (a *application) closeAwakeDialog() {
	if a.dialog != nil {
		d := a.dialog
		a.dialog = nil
		if d.cancelLoad != nil {
			d.cancelLoad()
		}
		if d.window != nil {
			d.window.Close()
		}
	}
}
func (a *application) confirmAwakeDialog() {
	d := a.dialog
	if d == nil {
		return
	}
	var s awakeSession
	switch d.kind {
	case "custom":
		duration, err := customDuration(d.hours, d.minutes)
		if err != nil {
			d.errorKey = "DurationError"
			return
		}
		a.config.AwakeMinutes = int(duration / time.Minute)
		a.config.AwakeUntil = ""
		s = a.selectedSession()
	case "until":
		end, err := untilTime(d.until, time.Now())
		if err != nil {
			d.errorKey = "TimeError"
			return
		}
		a.config.AwakeUntil = d.until
		s = awakeSession{Mode: a.config.AwakeMode, End: end}
	case "follow":
		if d.loading || d.errorKey == "ProcessListError" {
			return
		}
		if !d.filterReady || d.filteredQuery != d.search {
			d.filterProcesses()
		}
		for _, p := range d.filtered {
			if p.PID == d.selected {
				s = awakeSession{Mode: a.config.AwakeMode, PID: p.PID, Identity: p.Identity, Name: p.Name}
				break
			}
		}
		if s.PID == 0 || !a.awake.alive(s.PID, s.Identity) {
			d.errorKey = "ProcessEnded"
			return
		}
	}
	if err := a.awake.start(s); err != nil {
		d.errorKey = "AwakeStartError"
		return
	}
	a.save()
	a.closeAwakeDialog()
}
func (a *application) awakeDialogView(c *ui.Context) {
	locale, t := applyTheme(c)
	d := a.dialog
	if d == nil {
		return
	}
	if d.kind == "follow" {
		d.receiveProcesses()
		if !d.filterReady || (d.search != d.filteredQuery && !c.Now().Before(d.searchDue)) {
			selected := d.selected
			initial := !d.filterReady
			d.filterProcesses()
			if initial && selected != 0 {
				for _, p := range d.filtered {
					if p.PID == selected {
						d.selected = selected
						break
					}
				}
			}
		}
		if d.search != d.filteredQuery {
			c.After(d.searchDue.Sub(c.Now()))
		}
	}
	text := func(key string) string { return a.text(locale, key) }
	c.OnShortcut(0, ui.KeyEscape, a.closeAwakeDialog)
	c.OnShortcut(0, ui.KeyEnter, a.confirmAwakeDialog)
	ui.Column(c).Fill().Padding(t.Space16).Gap(t.Space8).Radius(t.RadiusWindow).Background(ui.Hex(t.ColorWindow)).DragWindow().Children(func() {
		field := func(key string, value *string, width float32, autofocus bool) {
			e := ui.TextInput(c.Key(key), value).Width(width).Height(t.SizeSegmentHeight).PaddingX(t.Space12).PaddingY(t.Space4).Radius(t.RadiusControl).Border(0, ui.Hex(t.ColorTransparent)).Background(ui.Hex(t.ColorControl)).FontSize(t.FontSizeBody).Label(text(key)).FocusRing(true)
			if autofocus {
				e.AutoFocus()
			}
			e.OnSubmit(a.confirmAwakeDialog)
		}
		switch d.kind {
		case "custom":
			ui.Text(c, text("R05CustomTitle")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
			ui.Row(c).FillWidth().Gap(t.Space12).AlignItems(ui.Center).Children(func() {
				field("R05Hours", &d.hours, t.SizeR05FieldWidth-t.Space40-t.Space24, true)
				ui.Text(c, text("R05Hours")).FontSize(t.FontSizeBody)
				field("R05Minutes", &d.minutes, t.SizeR05FieldWidth-t.Space40-t.Space24, false)
				ui.Text(c, text("R05Minutes")).FontSize(t.FontSizeBody)
				ui.Text(c, text("R05Return")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
			})
		case "until":
			ui.Text(c, text("R05UntilTitle")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
			ui.Row(c).FillWidth().Gap(t.Space8).AlignItems(ui.Center).Children(func() {
				field("R05UntilTitle", &d.until, t.SizeDialogWidth-2*t.Space16-t.SizeIcon-t.Space8, true)
				ui.Text(c, text("R05Return")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
			})
		case "follow":
			ui.Row(c).FillWidth().Height(t.SizeNavHeight).PaddingX(t.Space8).Gap(t.Space8).Radius(t.RadiusControl).Background(ui.Hex(t.ColorControl)).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icon("search")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
				ui.TextInput(c.Key("process-search"), &d.search).Grow(1).FillHeight().Padding(0).Border(0, ui.Hex(t.ColorTransparent)).Background(ui.Hex(t.ColorTransparent)).Placeholder(text("R05Search")).Label(text("R05Search")).AutoFocus().OnChange(func() {
					d.searchDue = c.Now().Add(100 * time.Millisecond)
				}).OnShortcut(0, ui.KeyDown, func() { d.moveProcessSelection(1) }).OnShortcut(0, ui.KeyUp, func() { d.moveProcessSelection(-1) }).OnSubmit(a.confirmAwakeDialog)
				ui.Text(c, text("R05Return")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
			})
			if d.loading {
				ui.Spinner(c).Size(t.SizeIcon, t.SizeIcon)
			}
			d.list.Key = func(i int) any { return d.filtered[i].PID }
			first, last := d.list.Visible()
			ui.List(c.Key("process-list"), &d.list, len(d.filtered), func(i int) {
				p := d.filtered[i]
				bitmap := d.icons[p.PID]
				if bitmap == nil && d.iconCache != nil && p.BundlePath != "" && i >= first && i <= last {
					var pending bool
					bitmap, pending = d.iconCache.request(p.BundlePath, c.Services().Invalidate)
					if pending {
						c.After(30 * time.Millisecond)
					}
				}
				background := t.ColorTransparent
				if p.PID == d.selected {
					background = t.ColorAccentSoft
				}
				ui.ButtonBase(c.Key(strconv.Itoa(p.PID))).FillWidth().Height(t.SizeRowHeight-t.Space8).Radius(t.RadiusCard).Background(ui.Hex(background)).FocusRing(true).Label(p.Name+" "+strconv.Itoa(p.PID)).OnClick(func() { d.selected = p.PID }).OnShortcut(0, ui.KeyEnter, func() { d.selected = p.PID; a.confirmAwakeDialog() }).OnShortcut(0, ui.KeyDown, func() { d.moveProcessSelection(1) }).OnShortcut(0, ui.KeyUp, func() { d.moveProcessSelection(-1) }).Children(func() {
					ui.Row(c).Fill().PaddingX(t.Space8).Gap(t.Space12).AlignItems(ui.Center).Children(func() {
						if bitmap != nil {
							ui.Image(c, bitmap).Size(t.SizeIconLarge, t.SizeIconLarge)
						} else {
							ui.Icon(c, icon("terminal")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
						}
						ui.Text(c, p.Name).FontSize(t.FontSizeBody).Grow(1)
						ui.Text(c, strconv.Itoa(p.PID)).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
					})
				})
			}).FillWidth().Grow(1).Gap(t.Space4)
		}
		if d.errorKey != "" {
			ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).Tooltip(text(d.errorKey))
		}
	})
	ui.Overlay(c, func() {
		e := ui.Box(c.Key("dialog-shortcuts")).Size(0, 0).PassThrough()
		if e.OverlayShortcut(0, ui.KeyEnter) {
			a.confirmAwakeDialog()
		}
		if e.OverlayShortcut(0, ui.KeyEscape) {
			a.closeAwakeDialog()
		}
	})
}

func shortcutSymbols(acc string) []string {
	var out []string
	for _, part := range strings.Split(acc, "+") {
		switch part {
		case "Ctrl":
			part = "⌃"
		case "Alt":
			part = "⌥"
		case "Shift":
			part = "⇧"
		case "Cmd":
			part = "⌘"
		case "Space":
			part = "␣"
		}
		out = append(out, part)
	}
	return out
}
func shortcutKey(mods ui.Modifiers, key ui.Key) string {
	name := ""
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		name = string(rune('A' + key - ui.KeyA))
	case key >= ui.Key0 && key <= ui.Key9:
		name = string(rune('0' + key - ui.Key0))
	case key >= ui.KeyF1 && key <= ui.KeyF12:
		name = fmt.Sprintf("F%d", key-ui.KeyF1+1)
	}
	switch key {
	case ui.KeyUp:
		name = "Up"
	case ui.KeyDown:
		name = "Down"
	case ui.KeyLeft:
		name = "Left"
	case ui.KeyRight:
		name = "Right"
	case ui.KeyTab:
		name = "Tab"
	case ui.KeyComma:
		name = ","
	case ui.KeySpace:
		name = "Space"
	case ui.KeyPeriod:
		name = "."
	case ui.KeySlash:
		name = "/"
	case ui.KeyMinus:
		name = "-"
	case ui.KeyEqual:
		name = "="
	}
	if name == "" {
		return ""
	}
	parts := []string{}
	for _, m := range []struct {
		value ui.Modifiers
		name  string
	}{{ui.Ctrl, "Ctrl"}, {ui.Alt, "Alt"}, {ui.Shift, "Shift"}, {ui.Cmd, "Cmd"}} {
		if mods&m.value != 0 {
			parts = append(parts, m.name)
		}
	}
	return strings.Join(append(parts, name), "+")
}
func sameShortcut(first, second string) bool {
	identity := func(acc string) string {
		parts := strings.Split(acc, "+")
		sort.Strings(parts[:len(parts)-1])
		return strings.Join(parts, "+")
	}
	return identity(first) == identity(second)
}
func knownShortcutConflict(acc string) string {
	groups := []struct {
		reason    string
		shortcuts []string
	}{
		{"R05Conflict", []string{"Cmd+,"}},
		{"ShortcutAppConflict", []string{"Cmd+Q", "Cmd+W", "Cmd+Z", "Cmd+Shift+Z", "Cmd+X", "Cmd+C", "Cmd+V", "Cmd+A", "Cmd+O", "Cmd+S", "Cmd+Shift+S", "Cmd+Shift+C", "Alt+Cmd+P"}},
		{"ShortcutSystemConflict", []string{"Cmd+Space", "Ctrl+Space", "Ctrl+Alt+Space", "Cmd+Alt+Space", "Ctrl+Cmd+Q", "Cmd+Tab", "Cmd+Shift+3", "Cmd+Shift+4", "Cmd+Shift+5", "Ctrl+Up", "Ctrl+Down", "Ctrl+Left", "Ctrl+Right", "F11", "F12"}},
	}
	for _, group := range groups {
		for _, candidate := range group.shortcuts {
			if sameShortcut(acc, candidate) {
				return group.reason
			}
		}
	}
	if !strings.Contains(acc, "+") {
		return "ShortcutModifierRequired"
	}
	return ""
}
func (a *application) recordShortcut(mods ui.Modifiers, key ui.Key) {
	if key == ui.KeyEscape {
		a.cancelShortcutRecording()
		return
	}
	acc := shortcutKey(mods, key)
	if acc == "" {
		return
	}
	a.shortcutDraft = acc
	if reason := knownShortcutConflict(acc); reason != "" {
		a.shortcutError = reason
		return
	}
	targetJSON := a.shortcutTarget == "json"
	old, other, enabled, callback := a.config.AwakeShortcut, a.config.JSONShortcut, a.config.AwakeEnabled, a.toggleAwake
	if targetJSON {
		old, other, enabled, callback = a.config.JSONShortcut, a.config.AwakeShortcut, a.config.JSONEnabled, a.newJSONWindow
	}
	if sameShortcut(acc, other) {
		a.shortcutError = "JSONShortcutConflict"
		return
	}
	if enabled {
		if err := a.registerShortcut(acc, callback); err != nil {
			a.shortcutError = "ShortcutUnavailable"
			return
		}
		if acc != old && !a.shortcutSuspended {
			a.unregisterShortcut(old)
		}
	}
	if targetJSON {
		a.config.JSONShortcut = acc
		a.jsonShortcutError = ""
		a.registerAwakeShortcut()
	} else {
		a.config.AwakeShortcut = acc
		a.registerJSONShortcut()
	}
	a.shortcutSuspended = false
	a.shortcutRecording = false
	a.shortcutError = ""
	a.shortcutDraft = ""
	a.save()
	a.refreshAwake()
	if a.refreshApplicationMenu != nil {
		a.refreshApplicationMenu()
	}
}
func (a *application) awakeSettings(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	if a.page == 2 {
		a.row(c, t, text("Awake"), func() {
			a.switchControl(c, t, "awake-enabled", text("Awake"), &a.config.AwakeEnabled, a.changeAwakeEnabled)
		})
		ui.Divider(c)
		a.row(c, t, text("JSON"), func() {
			a.switchControl(c, t, "json-enabled", text("JSON"), &a.config.JSONEnabled, a.changeJSONEnabled)
		})
	} else {
		a.shortcutSettingsRow(c, t, locale, "awake")
		ui.Divider(c)
		a.shortcutSettingsRow(c, t, locale, "json")
	}
}

func (a *application) beginShortcutRecording() { a.beginTileShortcutRecording("awake") }
func (a *application) beginTileShortcutRecording(target string) {
	if a.shortcutRecording {
		a.cancelShortcutRecording()
	}
	if a.config.AwakeEnabled {
		a.unregisterShortcut(a.config.AwakeShortcut)
	}
	if a.config.JSONEnabled {
		a.unregisterShortcut(a.config.JSONShortcut)
	}
	a.shortcutSuspended = true
	a.shortcutTarget = target
	a.shortcutRecording = true
	a.shortcutDraft = ""
	a.shortcutError = ""
	if a.refreshApplicationMenu != nil {
		a.refreshApplicationMenu()
	}
}
func (a *application) cancelShortcutRecording() {
	if !a.shortcutRecording {
		return
	}
	a.shortcutRecording = false
	a.shortcutDraft = ""
	a.shortcutError = ""
	if a.shortcutSuspended {
		a.registerAwakeShortcut()
		a.registerJSONShortcut()
	}
	a.shortcutSuspended = false
	if a.refreshApplicationMenu != nil {
		a.refreshApplicationMenu()
	}
}

func (a *application) shortcutSettingsRow(c *ui.Context, t tokenValues, locale, target string) {
	text := func(key string) string { return a.text(locale, key) }
	label, recordKey := "R05ShortcutLabel", "ShortcutRecordAction"
	acc := a.config.AwakeShortcut
	if target == "json" {
		label, recordKey = "JSONShortcutLabel", "JSONRecordShortcut"
		acc = a.config.JSONShortcut
	}
	active := a.shortcutRecording && a.shortcutTarget == target
	errorKey := ""
	if a.shortcutTarget == target || a.shortcutTarget == "" && target == "awake" {
		errorKey = a.shortcutError
	}
	if target == "json" && errorKey == "" {
		errorKey = a.jsonShortcutError
	}
	a.row(c, t, text(label), func() {
		if active && a.shortcutDraft != "" {
			acc = a.shortcutDraft
		}
		e := ui.ButtonBase(c.Key(target+"-recorder")).Size(t.SizeR05RecorderWidth, t.SizeSegmentHeight).PaddingX(t.Space12).Radius(t.RadiusControl).Background(ui.Hex(t.ColorControl)).FocusRing(true).Label(text(recordKey)).OnClick(func() { a.beginTileShortcutRecording(target) })
		e.HandleInput(func(ev ui.InputEvent) bool {
			if active && ev.Kind == ui.InputKeyDown {
				a.recordShortcut(ev.Mods, ev.Key)
				return true
			}
			return false
		})
		e.Children(func() {
			ui.Row(c).Fill().Gap(t.Space2).AlignItems(ui.Center).Children(func() {
				if active && a.shortcutDraft == "" {
					ui.Text(c, text("R05Recording")).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorTextSecondary))
				} else {
					for _, symbol := range shortcutSymbols(acc) {
						ui.Text(c, symbol).FontSize(t.FontSizeBody)
					}
				}
			})
		})
		if errorKey != "" {
			alert := ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).Focusable().Label(text(errorKey)).Description(text(errorKey))
			ui.TooltipBase(c, alert, func(tip ui.Element) {
				tip.Padding(t.Space6, t.Space8).Radius(t.RadiusControl).Background(ui.Hex(t.ColorTooltip)).TextColor(ui.Hex(t.ColorTooltipText)).Children(func() {
					ui.Row(c).Gap(t.Space8).AlignItems(ui.Center).Children(func() {
						ui.Row(c).Gap(t.Space2).AlignItems(ui.Center).Children(func() {
							for _, symbol := range shortcutSymbols(acc) {
								ui.Text(c, symbol).FontSize(t.FontSizeSmall)
							}
						})
						reason := errorKey
						if reason == "R05Conflict" {
							reason = "ShortcutSettingsSuffix"
						}
						ui.Text(c, text(reason)).FontSize(t.FontSizeSmall)
					})
				})
			})
		}
	})
}

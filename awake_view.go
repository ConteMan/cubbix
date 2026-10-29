package main

import (
	"context"
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
}

func (a *application) openAwakeDialog(kind string) {
	if a.dialog != nil && a.dialog.window != nil {
		a.dialog.window.Close()
	}
	d := &awakeDialog{kind: kind, hours: "1", minutes: "30", until: "02:30", icons: map[int]*ui.Bitmap{}}
	a.dialog = d
	a.createAwakeDialogWindow(d)
	// This is reached only by an explicit user action, never by tests or startup.
	d.window.Show()
	d.window.Focus()
	if kind == "follow" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			processes, err := listProcesses(ctx)
			sort.SliceStable(processes, func(i, j int) bool {
				rank := func(name string) int {
					if strings.Contains(strings.ToLower(name), "codex") || strings.Contains(strings.ToLower(name), "claude") {
						return 0
					}
					return 1
				}
				if rank(processes[i].Name) != rank(processes[j].Name) {
					return rank(processes[i].Name) < rank(processes[j].Name)
				}
				return strings.ToLower(processes[i].Name) < strings.ToLower(processes[j].Name)
			})
			mygo.RunOnMain(func() {
				if a.dialog != d {
					return
				}
				d.loading = false
				if err != nil {
					d.errorKey = "ProcessListError"
				} else {
					d.processes = processes
					for _, p := range processes {
						if icon := runningAppIcon(p.PID); icon != nil {
							d.icons[p.PID] = icon
						}
					}
					if len(processes) > 0 {
						d.selected = processes[0].PID
					}
				}
				d.window.Invalidate()
			})
		}()
	}
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
		if a.dialog == d {
			a.dialog = nil
		}
	})
}

func (a *application) closeAwakeDialog() {
	if a.dialog != nil {
		d := a.dialog
		a.dialog = nil
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
		for _, p := range d.processes {
			if p.PID == d.selected && matchesProcess(p, d.search) {
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
func matchesProcess(p runningProcess, query string) bool {
	return strings.Contains(strings.ToLower(p.Name+" "+strconv.Itoa(p.PID)), strings.ToLower(query))
}

func (a *application) awakeDialogView(c *ui.Context) {
	locale, t := applyTheme(c)
	d := a.dialog
	if d == nil {
		return
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
					d.selected = 0
					for _, p := range d.processes {
						if matchesProcess(p, d.search) {
							d.selected = p.PID
							break
						}
					}
				}).OnSubmit(a.confirmAwakeDialog)
				ui.Text(c, text("R05Return")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
			})
			if d.loading {
				ui.Spinner(c).Size(t.SizeIcon, t.SizeIcon)
			}
			ui.Scroll(c.Key("process-list")).FillWidth().Grow(1).Children(func() {
				ui.Column(c).FillWidth().Gap(t.Space4).Children(func() {
					for _, p := range d.processes {
						if !matchesProcess(p, d.search) {
							continue
						}
						background := t.ColorTransparent
						if p.PID == d.selected {
							background = t.ColorAccentSoft
						}
						ui.ButtonBase(c.Key(strconv.Itoa(p.PID))).FillWidth().Height(t.SizeRowHeight-t.Space8).Radius(t.RadiusCard).Background(ui.Hex(background)).FocusRing(true).Label(p.Name+" "+strconv.Itoa(p.PID)).OnClick(func() { d.selected = p.PID }).OnShortcut(0, ui.KeyEnter, func() { d.selected = p.PID; a.confirmAwakeDialog() }).Children(func() {
							ui.Row(c).Fill().PaddingX(t.Space8).Gap(t.Space12).AlignItems(ui.Center).Children(func() {
								if bitmap := d.icons[p.PID]; bitmap != nil {
									ui.Image(c, bitmap).Size(t.SizeIconLarge, t.SizeIconLarge)
								} else {
									ui.Icon(c, icon("terminal")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
								}
								ui.Text(c, p.Name).FontSize(t.FontSizeBody).Grow(1)
								ui.Text(c, strconv.Itoa(p.PID)).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
							})
						})
					}
				})
			})
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
func knownShortcutConflict(acc string) string {
	switch acc {
	case "Cmd+,":
		return "R05Conflict"
	case "Cmd+Q", "Cmd+W", "Cmd+Z", "Cmd+Shift+Z", "Cmd+X", "Cmd+C", "Cmd+V", "Cmd+A":
		return "ShortcutAppConflict"
	case "Cmd+Space", "Ctrl+Space", "Ctrl+Alt+Space", "Cmd+Alt+Space", "Ctrl+Cmd+Q", "Cmd+Tab", "Cmd+Shift+3", "Cmd+Shift+4", "Cmd+Shift+5", "Ctrl+Up", "Ctrl+Down", "Ctrl+Left", "Ctrl+Right", "F11", "F12":
		return "ShortcutSystemConflict"
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
	old := a.config.AwakeShortcut
	if a.config.AwakeEnabled {
		if err := a.registerShortcut(acc, a.toggleAwake); err != nil {
			a.shortcutError = "ShortcutUnavailable"
			return
		}
		if acc != old && !a.shortcutSuspended {
			a.unregisterShortcut(old)
		}
	}
	a.config.AwakeShortcut = acc
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
		ui.Row(c).FillWidth().Height(t.SizeRowHeight).AlignItems(ui.Center).Children(func() {
			ui.Text(c, text("JSON")).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorTextDisabled)).Grow(1)
			off := false
			ui.SwitchBase(c.Key("json-placeholder"), &off).Disabled(true).Size(t.SizeSwitchWidth, t.SizeSwitchHeight).Radius(t.RadiusControl).Background(ui.Hex(t.ColorControl)).Padding(t.Space2).AlignItems(ui.Center).Label(text("JSON")).Children(func() {
				ui.Box(c).Size(t.SizeSwitchThumb, t.SizeSwitchThumb).Radius(t.SizeSwitchThumb / 2).Background(ui.Hex(t.ColorTextDisabled))
			})
		})
	} else {
		a.row(c, t, text("R05ShortcutLabel"), func() {
			acc := a.config.AwakeShortcut
			if a.shortcutDraft != "" {
				acc = a.shortcutDraft
			}
			e := ui.ButtonBase(c.Key("awake-recorder")).Size(t.SizeR05RecorderWidth, t.SizeSegmentHeight).PaddingX(t.Space12).Radius(t.RadiusControl).Background(ui.Hex(t.ColorControl)).FocusRing(true).Label(text("ShortcutRecordAction")).OnClick(a.beginShortcutRecording)
			e.HandleInput(func(ev ui.InputEvent) bool {
				if a.shortcutRecording && ev.Kind == ui.InputKeyDown {
					a.recordShortcut(ev.Mods, ev.Key)
					return true
				}
				return false
			})
			e.Children(func() {
				ui.Row(c).Fill().Gap(t.Space2).AlignItems(ui.Center).Children(func() {
					if a.shortcutRecording && a.shortcutDraft == "" {
						ui.Text(c, text("R05Recording")).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorTextSecondary))
					} else {
						for _, symbol := range shortcutSymbols(acc) {
							ui.Text(c, symbol).FontSize(t.FontSizeBody)
						}
					}
				})
			})
			if a.shortcutError != "" {
				alert := ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).Focusable().Label(text(a.shortcutError)).Description(text(a.shortcutError))
				ui.TooltipBase(c, alert, func(tip ui.Element) {
					tip.Padding(t.Space6, t.Space8).Radius(t.RadiusControl).Background(ui.Hex(t.ColorTooltip)).TextColor(ui.Hex(t.ColorTooltipText)).Children(func() {
						ui.Row(c).Gap(t.Space8).AlignItems(ui.Center).Children(func() {
							ui.Row(c).Gap(t.Space2).AlignItems(ui.Center).Children(func() {
								for _, symbol := range shortcutSymbols(acc) {
									ui.Text(c, symbol).FontSize(t.FontSizeSmall)
								}
							})
							reason := a.shortcutError
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
}

func (a *application) beginShortcutRecording() {
	if !a.shortcutRecording && a.config.AwakeEnabled {
		a.unregisterShortcut(a.config.AwakeShortcut)
		a.shortcutSuspended = true
	}
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
	if a.shortcutSuspended && a.config.AwakeEnabled {
		a.registerAwakeShortcut()
	}
	a.shortcutSuspended = false
	if a.refreshApplicationMenu != nil {
		a.refreshApplicationMenu()
	}
}

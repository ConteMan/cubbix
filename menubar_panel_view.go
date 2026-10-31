package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// fontWeight parses a weight token such as "600".
func fontWeight(value string) int {
	weight, err := strconv.Atoi(value)
	if err != nil {
		return 400
	}
	return weight
}

// menubarPanelView draws the whole card panel: the tile cards, then the
// system actions. All colors and sizes come from tokens.go through applyTheme.
func (a *application) menubarPanelView(c *ui.Context) {
	locale, t := applyTheme(c)
	if a.panel == nil {
		return
	}
	rows := a.menubarPanelRows()
	// A vibrancy material draws the panel's own surface; the content stays
	// transparent so it shows through.
	c.Root().Background(ui.Transparent)
	panel := ui.Column(c).Fill().Padding(t.Space12).Gap(t.Space8)
	if !c.Vibrancy() {
		panel = panel.Background(ui.Hex(t.ColorMenuSurface)).Radius(t.RadiusWindow).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator))
	}
	panel.Children(func() {
		if rows.awake {
			a.panelAwakeCard(c, t, locale, rows)
		}
		if rows.json {
			a.panelJSONCard(c, t, locale, rows)
		}
		if rows.empty {
			ui.Box(c).FillWidth().Height(t.SizeNavHeight).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, a.text(locale, "PanelEmpty")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextDisabled))
			})
		}
		a.panelFooter(c, t, locale)
	})
}

// panelAwakeCard is the keep awake tile: title and master switch, the two
// modes, the preset pills, and the running status.
func (a *application) panelAwakeCard(c *ui.Context, t tokenValues, locale string, rows menubarPanelRows) {
	text := func(key string) string { return a.text(locale, key) }
	state := a.awake.snapshot()
	ui.Column(c).FillWidth().Padding(t.Space10).Gap(t.Space8).Radius(t.RadiusCard).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).Background(ui.Hex(t.ColorCard)).Children(func() {
		ui.Row(c).FillWidth().Height(t.SizeNavHeight).Gap(t.Space8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon("sun")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorText))
			ui.Text(c, text("Awake")).FontSize(t.FontSizeBody).FontWeight(fontWeight(t.FontWeightSemibold)).TextColor(ui.Hex(t.ColorText)).Grow(1)
			on := state.Active
			background, thumb, align := t.ColorControl, t.ColorTextSecondary, ui.Start
			label := text("R05Start")
			if on {
				background, thumb, align = t.ColorAccent, t.ColorThumb, ui.End
				label = text("R05Stop")
			}
			ui.SwitchBase(c.Key("panel-awake"), &on).Size(t.SizeSwitchWidth, t.SizeSwitchHeight).Radius(t.RadiusControl).Background(ui.Hex(background)).Padding(t.Space2).Justify(align).AlignItems(ui.Center).Label(label).FocusRing(true).OnChange(a.panelToggleAwake).Children(func() {
				ui.Box(c).Size(t.SizeSwitchThumb, t.SizeSwitchThumb).Radius(t.SizeSwitchThumb / 2).Background(ui.Hex(thumb))
			})
		})
		a.panelModeSwitch(c, t, locale)
		a.panelPills(c, t, locale)
		if rows.status {
			a.panelStatus(c, t, locale)
		}
	})
}

// panelModeSwitch is the two-segment mode control. It hot-swaps caffeinate
// while keep awake runs; the arrows also switch it from the keyboard.
func (a *application) panelModeSwitch(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	// The segment shows the mode in effect, which is the session's while keep
	// awake runs (a deep link may have chosen it), otherwise the remembered one.
	current := a.config.AwakeMode
	if state := a.awake.snapshot(); state.Active {
		current = state.Session.Mode
	}
	mode := 0
	if current == "tasks" {
		mode = 1
	}
	parts := ui.SegmentedBase(c.Key("panel-mode"), &mode, 2)
	parts.Track.FillWidth().Height(t.SizeSegmentHeight).Padding(t.Space2).Radius(t.RadiusControl).Background(ui.Hex(t.ColorControl)).Label(text("Awake")).OnChange(func() {
		a.panelSetMode(mode)
	}).Children(func() {
		for index, label := range []string{text("R05Screen"), text("R05Tasks")} {
			color, background := t.ColorText, t.ColorTransparent
			if index == mode {
				color, background = t.ColorOnAccent, t.ColorAccent
			}
			parts.Segment(index).Grow(1).FillHeight().Radius(t.RadiusControl).Background(ui.Hex(background)).FocusRing(true).Children(func() {
				ui.Text(c, label).FontSize(t.FontSizeBody).TextColor(ui.Hex(color)).SingleLine()
			})
		}
	})
}

// panelPills is the preset row. One click starts keep awake at that length, or
// resets a running one; the row is one keyboard stop, the arrows move along it.
func (a *application) panelPills(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	preset, presetOK := a.panelPreset(time.Now())
	running := a.awake.snapshot().Active
	row := ui.Row(c).FillWidth().Height(t.SizeButtonHeight).Gap(t.Space6).FocusGroup(ui.Horizontal)
	row.Children(func() {
		for _, pill := range []struct {
			minutes int
			key     string
		}{{30, "PanelDuration30m"}, {60, "PanelDuration1h"}, {120, "PanelDuration2h"}, {0, "PanelDurationInf"}} {
			minutes := pill.minutes
			selected := presetOK && preset == minutes
			background, color := t.ColorControl, t.ColorText
			if selected && running {
				background, color = t.ColorAccent, t.ColorOnAccent
			} else if selected {
				// Off: the remembered preset stays visible without emphasis.
				background = t.ColorAccentSoft
			}
			button := ui.ButtonBase(c.Key(fmt.Sprintf("panel-duration-%d", minutes))).Width(t.SizePillButtonWidth).Grow(1).FillHeight().Radius(t.RadiusControl).Background(ui.Hex(background)).FocusRing(true).Role(ui.RoleRadio).Checked(selected).Label(text(pill.key)).OnClick(func() { a.panelSetDuration(minutes) })
			if !selected && button.Hovered() {
				button.Background(ui.Hex(t.ColorControlHover))
			}
			button.Children(func() {
				ui.Text(c, text(pill.key)).FontSize(t.FontSizeBody).TextColor(ui.Hex(color)).SingleLine()
			})
		}
	})
}

// panelStatus reports the running session, or why it ended.
func (a *application) panelStatus(c *ui.Context, t tokenValues, locale string) {
	ui.Row(c).FillWidth().Height(t.Space20).Gap(t.Space6).AlignItems(ui.Center).Children(func() {
		if errorKey := a.menubarPanelErrorKey(); errorKey != "" {
			label := a.text(locale, errorKey)
			alert := ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).Focusable().Label(label).Description(label)
			ui.TooltipBase(c, alert, func(tip ui.Element) {
				tip.Padding(t.Space6, t.Space8).Radius(t.RadiusControl).Background(ui.Hex(t.ColorTooltip)).TextColor(ui.Hex(t.ColorTooltipText)).Children(func() {
					ui.Text(c, label).FontSize(t.FontSizeSmall)
				})
			})
			return
		}
		ui.Text(c, a.menubarPanelStatusLine(locale, time.Now())).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary)).Grow(1).SingleLine().Ellipsis("…")
	})
}

// panelJSONCard is the JSON tile: a quick new window and the open windows.
// Six windows fold into one count row. The title row's plus is the only way
// to open a window from the card.
func (a *application) panelJSONCard(c *ui.Context, t tokenValues, locale string, rows menubarPanelRows) {
	text := func(key string) string { return a.text(locale, key) }
	newTip := a.menubarPanelTooltip(locale, "JSONShortcutLabel", a.config.JSONShortcut)
	ui.Column(c).FillWidth().Padding(t.Space10).Gap(t.Space8).Radius(t.RadiusCard).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).Background(ui.Hex(t.ColorCard)).Children(func() {
		ui.Row(c).FillWidth().Height(t.SizeNavHeight).Gap(t.Space8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon("braces")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorText))
			ui.Text(c, text("JSON")).FontSize(t.FontSizeBody).FontWeight(fontWeight(t.FontWeightSemibold)).TextColor(ui.Hex(t.ColorText)).Grow(1)
			button := ui.ButtonBase(c.Key("panel-json-new")).Size(t.SizeIconButton, t.SizeIconButton).Radius(t.RadiusControl).FocusRing(true).Label(text("JSONShortcutLabel")).Tooltip(newTip).OnClick(a.panelNewJSON)
			if button.Hovered() {
				button.Background(ui.Hex(t.ColorControlHover))
			}
			button.Children(func() {
				ui.Icon(c, icon("plus")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
			})
		})
		if rows.listRows > 0 {
			ui.Column(c).FillWidth().Gap(t.Space2).Children(func() {
				if len(rows.documents) >= jsonWindowSubmenuAt {
					a.panelJSONListToggle(c, t, locale, len(rows.documents), rows.folded)
				}
				if !rows.folded {
					for _, d := range rows.documents {
						a.panelJSONRow(c, t, locale, d)
					}
				}
			})
		}
	})
}

// panelJSONListToggle folds the window list open or shut.
func (a *application) panelJSONListToggle(c *ui.Context, t tokenValues, locale string, count int, folded bool) {
	label := fmt.Sprintf(a.text(locale, "PanelJSONWindowsCount"), count)
	glyph := "chevron-right"
	if !folded {
		glyph = "chevron-down"
	}
	button := ui.ButtonBase(c.Key("panel-json-fold")).FillWidth().Height(t.SizeNavHeight).PaddingX(t.Space8).Radius(t.RadiusControl).FocusRing(true).Label(label).OnClick(a.panelToggleJSONWindows)
	if button.Hovered() {
		button.Background(ui.Hex(t.ColorNavHover))
	}
	button.Children(func() {
		ui.Row(c).Fill().Gap(t.Space8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon(glyph)).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
			ui.Text(c, label).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorText)).Grow(1).SingleLine().Ellipsis("…").MinWidth(0)
		})
	})
}

func (a *application) panelJSONRow(c *ui.Context, t tokenValues, locale string, d *jsonDocument) {
	name := d.name(a, locale)
	button := ui.ButtonBase(c.Key(fmt.Sprintf("panel-json-%d", d.number))).FillWidth().Height(t.SizeNavHeight).PaddingX(t.Space8).Radius(t.RadiusControl).FocusRing(true).Label(name).OnClick(func() { a.panelShowJSON(d) })
	if button.Hovered() {
		button.Background(ui.Hex(t.ColorNavHover))
	}
	button.Children(func() {
		ui.Row(c).Fill().Gap(t.Space8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon("file-type")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
			ui.Text(c, name).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorText)).Grow(1).SingleLine().Ellipsis("…").MinWidth(0)
			if d.pinned {
				ui.Icon(c, icon("pin")).Size(t.SizeIconSmall, t.SizeIconSmall).TextColor(ui.Hex(t.ColorTextSecondary)).Label(name)
			}
		})
	})
}

// panelFooter is the system bar: the version on the left, then the command
// palette, settings and quit. Shortcuts live in the tooltips only.
func (a *application) panelFooter(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	ui.Column(c).FillWidth().Children(func() {
		ui.Box(c).FillWidth().Height(t.StrokeHairline).Background(ui.Hex(t.ColorSeparator))
		ui.Row(c).FillWidth().Height(t.SizeNavHeight).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Cubbix v"+runningVersion()).FontSize(t.FontSizeCaption).TextColor(ui.Hex(t.ColorTextDisabled)).Grow(1).SingleLine()
			a.panelIconButton(c, t, "panel-palette", "command", a.menubarPanelTooltip(locale, "PaletteShortcutLabel", a.config.PaletteShortcut), a.showPalette)
			a.panelIconButton(c, t, "panel-settings", "settings-2", text("MenuSettings"), func() {
				a.hideMenubarPanel()
				a.show()
			})
			a.panelIconButton(c, t, "panel-quit", "power", text("Quit"), mygo.App.Quit)
		})
	})
}

func (a *application) panelIconButton(c *ui.Context, t tokenValues, key, glyph, label string, onclick func()) {
	button := ui.ButtonBase(c.Key(key)).Size(t.SizeIconButton, t.SizeIconButton).Radius(t.RadiusControl).FocusRing(true).Label(label).Tooltip(label).OnClick(onclick)
	if button.Hovered() {
		button.Background(ui.Hex(t.ColorControlHover))
	}
	button.Children(func() {
		ui.Icon(c, icon(glyph)).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
	})
}

package main

import (
	"github.com/egoist/mygo/ui"
)

// paletteView draws the whole panel: search row, group list, ⌘K keycap,
// action list and inline errors. All colors and sizes come from tokens.go
// through applyTheme.
func (a *application) paletteView(c *ui.Context) {
	locale, t := applyTheme(c)
	p := a.palette
	if p == nil {
		return
	}
	a.refreshPalette(p, locale)
	text := func(key string) string { return a.text(locale, key) }
	c.OnShortcut(0, ui.KeyEscape, a.paletteEscape)
	c.OnShortcut(ui.Cmd, ui.KeyW, a.hidePalette)
	c.OnShortcut(ui.Cmd, ui.KeyK, a.togglePaletteActions)

	ui.Column(c).Fill().Padding(t.Space8).Gap(t.Space8).Radius(t.RadiusWindow).Clip().Background(ui.Hex(t.ColorMenuSurface)).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).Children(func() {
		ui.Row(c).FillWidth().Height(t.SizePaletteSearchHeight).PaddingX(t.Space8).Gap(t.Space8).Radius(t.RadiusControl).Background(ui.Hex(t.ColorControl)).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon("search")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
			input := ui.TextInput(c.Key("palette-search"), &p.query).Grow(1).FillHeight().Padding(0).Border(0, ui.Hex(t.ColorTransparent)).Background(ui.Hex(t.ColorTransparent)).FontSize(t.FontSizeBody).Placeholder(text("PaletteSearch")).Label(text("PaletteSearch")).FocusRing(true)
			input.Focus()
			input.OnChange(func() {
				// Editing leaves the action list and clears the last error.
				p.mode = paletteCommandMode
				p.actionsCommand = nil
				p.actionSelection = ""
				p.errorKey = ""
				p.runError = ""
			})
			input.OnSubmit(a.runPalette)
			input.OnShortcut(0, ui.KeyDown, func() { a.movePaletteSelection(1) })
			input.OnShortcut(0, ui.KeyUp, func() { a.movePaletteSelection(-1) })
			if p.errorKey != "" {
				label := text(p.errorKey)
				alert := ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).Focusable().Label(label).Description(label)
				ui.TooltipBase(c, alert, func(tip ui.Element) {
					tip.Padding(t.Space6, t.Space8).Radius(t.RadiusControl).Background(ui.Hex(t.ColorTooltip)).TextColor(ui.Hex(t.ColorTooltipText)).Children(func() {
						ui.Text(c, label).FontSize(t.FontSizeSmall)
					})
				})
			}
			ui.Row(c).Height(t.Space20).PaddingX(t.Space6).Radius(t.RadiusControl).Background(ui.Hex(t.ColorControl)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "⌘K").FontSize(t.FontSizeCaption).TextColor(ui.Hex(t.ColorTextDisabled))
			})
		})
		ui.Box(c).FillWidth().Height(t.StrokeHairline).Background(ui.Hex(t.ColorSeparator))
		if len(p.rows) == 0 {
			ui.Column(c).FillWidth().Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(t.Space8).Children(func() {
				ui.Icon(c, icon("search")).Size(t.SizeIconLarge, t.SizeIconLarge).TextColor(ui.Hex(t.ColorTextDisabled))
				ui.Text(c, text("PaletteEmpty")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextDisabled))
			})
			return
		}
		label := text("PaletteAll")
		if p.mode == paletteActionMode {
			label = text("PaletteActions")
		}
		ui.List(c.Key("palette-list"), &p.list, len(p.rows), func(index int) {
			a.paletteRow(c, t, locale, p, index)
		}).FillWidth().Grow(1).Gap(0).Label(label)
	})
	// The panel closes on Esc and runs on Enter even when the list, not the
	// field, has the keyboard; both are fallbacks behind the field's handlers.
	ui.Overlay(c, func() {
		element := ui.Box(c.Key("palette-keys")).Size(0, 0).PassThrough()
		if element.OverlayShortcut(0, ui.KeyEnter) {
			a.runPalette()
		}
	})
}

func (a *application) paletteRow(c *ui.Context, t tokenValues, locale string, p *paletteState, index int) {
	row := p.rows[index]
	switch row.Kind {
	case paletteRowHeader:
		ui.Row(c).FillWidth().Height(t.SizePaletteGroupHeight).PaddingX(t.Space8).AlignItems(ui.Center).Background(ui.Hex(t.ColorMenuSurface)).Children(func() {
			ui.Text(c, a.text(locale, row.Group)).FontSize(t.FontSizeCaption).TextColor(ui.Hex(t.ColorTextDisabled))
		})
	case paletteRowSeparator:
		ui.Box(c).FillWidth().Height(t.StrokeHairline).Background(ui.Hex(t.ColorSeparator))
	case paletteRowActionTitle:
		ui.Row(c).FillWidth().Height(t.SizePaletteRowHeight).PaddingX(t.Space8).Gap(t.Space8).AlignItems(ui.Center).Children(func() {
			if row.Icon != "" {
				ui.Icon(c, icon(row.Icon)).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
			}
			ui.Text(c, row.Title).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorText)).Grow(1).SingleLine().Ellipsis("…").MinWidth(0)
		})
	default:
		selected := index == p.selected
		background := t.ColorTransparent
		button := ui.ButtonBase(c.Key(row.Key)).FillWidth().Height(t.SizePaletteRowHeight).Radius(t.RadiusCard).FocusRing(true).Label(row.Title)
		if selected {
			background = t.ColorAccentSoft
		} else if button.Hovered() {
			background = t.ColorNavHover
		}
		button.Background(ui.Hex(background)).OnClick(func() {
			if row.Kind == paletteRowAction && row.Action != nil && row.Action.Available != nil && !row.Action.Available(a) {
				return
			}
			p.selected = index
			if p.mode == paletteActionMode {
				p.actionSelection = row.Key
			} else {
				p.selectionKey = row.Key
			}
			a.runPalette()
		}).Children(func() {
			ui.Row(c).Fill().PaddingX(t.Space8).Gap(t.Space8).AlignItems(ui.Center).Children(func() {
				if row.Icon != "" {
					ui.Icon(c, icon(row.Icon)).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
				}
				ui.Text(c, row.Title).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorText)).Grow(1).SingleLine().Ellipsis("…").MinWidth(0)
				if row.Hint != "" {
					ui.Text(c, row.Hint).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
				}
				if row.Checked {
					ui.Icon(c, icon("check")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorText))
				}
				if row.ErrorKey != "" && selected {
					label := a.text(locale, row.ErrorKey)
					alert := ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).Focusable().Label(label).Description(label)
					ui.TooltipBase(c, alert, func(tip ui.Element) {
						tip.Padding(t.Space6, t.Space8).Radius(t.RadiusControl).Background(ui.Hex(t.ColorTooltip)).TextColor(ui.Hex(t.ColorTooltipText)).Children(func() {
							ui.Text(c, label).FontSize(t.FontSizeSmall)
						})
					})
				}
			})
		})
	}
}

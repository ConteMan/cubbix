package main

import (
	"github.com/egoist/mygo/ui"
)

// paletteView draws the whole panel: search row, group list, the ⌘K keycaps,
// action list and inline errors. All colors and sizes come from tokens.go
// through applyTheme.
//
// The window is the panel plus SizePaletteShadowMargin on every side, because
// the panel draws its own shadow. Three elements answer for a drag: the icon
// at the start of the search row, the panel's own blank space (its padding,
// the gaps and the separator), and the shadow margin around it. The field's
// text and the list keep their presses, so selecting text and picking a row
// still work.
func (a *application) paletteView(c *ui.Context) {
	locale, t := applyTheme(c)
	p := a.palette
	if p == nil {
		return
	}
	a.refreshPalette(p, locale)
	empty := len(p.rows) == 0
	text := func(key string) string { return a.text(locale, key) }
	c.OnShortcut(0, ui.KeyEscape, a.paletteEscape)
	c.OnShortcut(ui.Cmd, ui.KeyW, a.hidePalette)
	c.OnShortcut(ui.Cmd, ui.KeyK, a.togglePaletteActions)
	c.Root().Background(ui.Transparent)

	// The margin belongs to the window, not the panel: a press there reads as
	// a press outside the panel.
	root := ui.Box(c.Key("palette-root")).Fill().Padding(t.SizePaletteShadowMargin).KeepFocus()
	p.rootBounds = root.Bounds()
	root.HandleInput(a.paletteDragHandler(&p.rootBounds, nil))

	root.Children(func() {
		panel := ui.Column(c.Key("palette-panel")).Fill().Padding(t.Space8).Gap(t.Space8).Radius(t.RadiusWindow).Clip().
			Background(ui.Hex(t.ColorMenuSurface)).
			Border(t.StrokeHairline, ui.Hex(t.ColorPanelStrokeOuter)).
			InsetShadow(0, 0, 0, t.StrokeHairline, ui.Hex(t.ColorPanelStrokeInner)).
			Shadow(0, t.Space8, t.Space24, 0, ui.Hex(t.ColorPanelShadow)).
			KeepFocus()
		// Blank panel space drags; the search row and the list answer for
		// themselves, so their presses fall through to the field and the rows.
		p.panelBounds = panel.Bounds()
		panel.HandleInput(a.paletteDragHandler(&p.panelBounds, func(ev ui.InputEvent) bool {
			return !a.paletteContentHit(ev, p.panelBounds)
		}))
		panel.Children(func() {
			searchRow := ui.Row(c).FillWidth().Height(t.SizePaletteSearchHeight).PaddingX(t.Space8).Gap(t.Space8).Radius(t.RadiusControl).Background(ui.Hex(t.ColorControl)).AlignItems(ui.Center)
			p.searchBounds = searchRow.Bounds()
			searchRow.Children(func() {
				grab := ui.Box(c.Key("palette-grab")).Size(t.SizePaletteSearchHeight, t.SizePaletteSearchHeight).KeepFocus()
				p.grabBounds = grab.Bounds()
				grab.HandleInput(a.paletteDragHandler(&p.grabBounds, nil))
				grab.Children(func() {
					ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
						ui.Icon(c, icon("search")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
					})
				})
				// The field keeps the input and draws the placeholder itself: the
				// input's own would sit on the caret.
				field := ui.Box(c.Key("palette-field")).Grow(1).FillHeight()
				field.Children(func() {
					input := ui.TextInput(c.Key("palette-search"), &p.query).Fill().Padding(0).Border(0, ui.Hex(t.ColorTransparent)).Background(ui.Hex(t.ColorTransparent)).FontSize(t.FontSizeBody).Label(text("PaletteSearch")).FocusRing(true)
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
					if p.query == "" && !input.Composing() {
						// A hint, not a control: the pointer reaches the input under it.
						ui.Row(c).Absolute().Left(t.SizePalettePlaceholderInset).Top(0).Bottom(0).AlignItems(ui.Center).PassThrough().Children(func() {
							ui.Text(c, text("PaletteSearch")).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine().Ellipsis("…")
						})
					}
				})
				if p.errorKey != "" {
					label := text(p.errorKey)
					alert := ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).Focusable().Label(label).Description(label)
					ui.TooltipBase(c, alert, func(tip ui.Element) {
						tip.Padding(t.Space6, t.Space8).Radius(t.RadiusControl).Background(ui.Hex(t.ColorTooltip)).TextColor(ui.Hex(t.ColorTooltipText)).Children(func() {
							ui.Text(c, label).FontSize(t.FontSizeSmall)
						})
					})
				}
				keycaps(c, t, paletteActionKeys)
			})
			if empty {
				// Nothing to pick: the whole body drags.
				p.listBounds = ui.Rect{}
				ui.Column(c).FillWidth().Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(t.Space8).Children(func() {
					ui.Icon(c, icon("search")).Size(t.SizeIconLarge, t.SizeIconLarge).TextColor(ui.Hex(t.ColorTextDisabled))
					ui.Text(c, text("PaletteEmpty")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextDisabled))
				})
			} else {
				label := text("PaletteAll")
				if p.mode == paletteActionMode {
					label = text("PaletteActions")
				}
				// The list keeps a slot on its right for the scroll bar, which the
				// framework draws over the last few DIPs of the box: rows end short
				// of it, so neither their text nor their highlight goes under it.
				list := ui.List(c.Key("palette-list"), &p.list, len(p.rows), func(index int) {
					a.paletteRow(c, t, locale, p, index)
				}).FillWidth().Grow(1).Gap(0).Padding(0, t.SizePaletteScrollbarSlot, 0, 0).Label(label)
				p.listBounds = list.Bounds()
			}
		})
	})
	if empty {
		return
	}
	// The panel closes on Esc and runs on Enter even when the list, not the
	// field, has the keyboard; both are fallbacks behind the field's handlers.
	ui.Overlay(c, func() {
		element := ui.Box(c.Key("palette-keys")).Size(0, 0).PassThrough()
		if element.OverlayShortcut(0, ui.KeyEnter) {
			a.runPalette()
		}
	})
}

// paletteRow draws one row of the list: a group header, a separator, the
// chosen command's title, or a command or action button.
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
				// The title and the tile it belongs to sit together on the left; the
				// title gives way first, the tile keeps its words.
				ui.Row(c).Grow(1).AlignItems(ui.Center).Gap(t.Space8).Children(func() {
					ui.Text(c, row.Title).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorText)).Shrink(1).SingleLine().Ellipsis("…").MinWidth(0)
					if row.Category != "" {
						ui.Text(c, row.Category).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
					}
				})
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

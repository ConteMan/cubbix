package main

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/ConteMan/cubbix/internal/devtools"
)

// devtoolsView draws the tool window: a toolbar with the two segments and the
// pin, the time field, the UUID controls, and one copyable row per result. All
// colours and sizes come from tokens.go through applyTheme.
func (a *application) devtoolsView(c *ui.Context) {
	locale, t := applyTheme(c)
	s := a.devtools
	if s == nil {
		return
	}
	if a.devtoolsCopyExpired(c.Now()) {
		s.copiedKey, s.copiedUntil = "", time.Time{}
	}
	if s.copiedUntil.After(c.Now()) {
		c.After(s.copiedUntil.Sub(c.Now()))
	}
	// A live clock is recomputed once per second on the frame clock, never from
	// a real-time read inside the frame: the two clocks may differ, and a
	// comparison across them would ask for a frame every time.
	if s.tab == devtoolsTimeTab && s.timeLive() {
		now := c.Now()
		if s.snapshot.IsZero() || !now.Before(s.snapshot.Add(time.Second)) {
			a.refreshDevtoolsTimeAt(now)
		} else {
			c.After(s.snapshot.Add(time.Second).Sub(now))
		}
	}
	// The window keeps its state when it loses the keyboard: only ⌘W, Esc and
	// the close button end it.
	c.OnShortcut(0, ui.KeyEscape, a.closeDevtools)
	c.OnShortcut(ui.Cmd, ui.KeyW, a.closeDevtools)
	c.OnShortcut(ui.Alt|ui.Cmd, ui.KeyP, func() { a.setDevtoolsPinned(!s.pinned) })
	// ⌘回车 copies and pastes the selected row; ⌘C and Enter copy it. The time
	// field reads all three before its own submit, so nothing runs twice.
	c.OnShortcut(ui.Cmd, ui.KeyEnter, func() { a.pasteDevtoolsSelected() })
	c.OnShortcut(0, ui.KeyEnter, func() { a.copyDevtoolsSelected() })
	c.OnShortcut(ui.Cmd, ui.KeyC, func() { a.copyDevtoolsSelected() })
	if s.tab == devtoolsUUIDTab {
		// The UUID tab has no editable field, so the arrows walk the rows. The
		// time tab keeps them for the caret of its input.
		c.OnShortcut(0, ui.KeyDown, func() { a.moveDevtoolsSelection(1) })
		c.OnShortcut(0, ui.KeyUp, func() { a.moveDevtoolsSelection(-1) })
	}
	c.Root().Background(ui.Hex(t.ColorCard))
	ui.Column(c.Key("devtools-root")).Fill().Children(func() {
		a.devtoolsToolbar(c, t, locale, s)
		ui.Box(c).FillWidth().Height(t.StrokeHairline).Background(ui.Hex(t.ColorSeparator))
		ui.Column(c.Key("devtools-panel")).FillWidth().Grow(1).MinHeight(0).
			Padding(t.Space12).Gap(t.Space8).Children(func() {
			if s.denied {
				a.devtoolsSelectionHint(c, t, locale)
			}
			if s.tab == devtoolsTimeTab {
				a.devtoolsTimeField(c, t, locale, s)
			} else {
				a.devtoolsUUIDControls(c, t, locale, s)
			}
			a.devtoolsList(c, t, locale, s)
		})
	})
}

func (a *application) devtoolsToolbar(c *ui.Context, t tokenValues, locale string, s *devtoolsState) {
	text := func(key string) string { return a.text(locale, key) }
	ui.Row(c.Key("devtools-toolbar")).FillWidth().Height(t.SizeJSONToolbarHeight).Shrink(0).
		Padding(t.Space8, t.Space12, t.Space8, t.SizeJSONTitleInset).Gap(t.Space8).AlignItems(ui.Center).Children(func() {
		ui.Row(c.Key("devtools-title-drag")).Grow(1).FillHeight().DragWindow().AlignItems(ui.Center).Children(func() {
			ui.Text(c, text("Devtools")).FontSize(t.FontSizeSmall).
				TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
		})
		parts := ui.SegmentedBase(c.Key("devtools-tabs"), &s.tab, 2)
		parts.Track.Size(2*t.SizePillButtonWidth, t.SizeSegmentHeight).Padding(t.Space2).Radius(t.RadiusControl).
			Background(ui.Hex(t.ColorControl)).Label(text("Devtools")).OnChange(func() {
			a.selectDevtoolsTab(s.tab)
		}).Children(func() {
			for index, key := range []string{"DevtoolsTime", "DevtoolsUUID"} {
				color, background := t.ColorText, t.ColorTransparent
				if index == s.tab {
					color, background = t.ColorOnAccent, t.ColorAccent
				}
				parts.Segment(index).Grow(1).FillHeight().Radius(t.RadiusControl).Background(ui.Hex(background)).
					FocusRing(true).Label(text(key)).Children(func() {
					ui.Text(c, text(key)).FontSize(t.FontSizeSmall).TextColor(ui.Hex(color)).SingleLine()
				})
			}
		})
		label := text("R07Pin")
		if s.pinned {
			label = text("R07Unpin")
		}
		pin := a.qrIconButton(c, t, "devtools-pin", "pin", label)
		if s.pinned {
			pin.Background(ui.Hex(t.ColorAccentSoft))
		}
		pin.OnClick(func() { a.setDevtoolsPinned(!s.pinned) })
	})
}

// devtoolsTimeField is the single input of the time segment. It refreshes on
// every keystroke: this module has no debounce.
func (a *application) devtoolsTimeField(c *ui.Context, t tokenValues, locale string, s *devtoolsState) {
	text := func(key string) string { return a.text(locale, key) }
	ui.Box(c).FillWidth().Height(t.SizePaletteSearchHeight).Shrink(0).Children(func() {
		input := ui.TextInput(c.Key("devtools-input"), &s.input).Fill().Padding(t.Space4, t.Space12).
			Border(0, ui.Hex(t.ColorTransparent)).Radius(t.RadiusControl).Background(ui.Hex(t.ColorControl)).
			FontSize(t.FontSizeBody).LineHeight(t.LineHeightBody).
			Placeholder(text("DevtoolsInput")).Label(text("DevtoolsInput")).FocusRing(true)
		input.OnChange(a.syncDevtoolsInput)
		input.OnSubmit(func() { a.copyDevtoolsSelected() })
		input.OnShortcut(ui.Cmd, ui.KeyEnter, func() { a.pasteDevtoolsSelected() })
		if s.focus {
			s.focus = false
			input.Focus()
		}
	})
}

// devtoolsUUIDControls is the UUID segment's own toolbar: generate again, and
// the two presentation switches, which are stored in the config.
func (a *application) devtoolsUUIDControls(c *ui.Context, t tokenValues, locale string, s *devtoolsState) {
	text := func(key string) string { return a.text(locale, key) }
	ui.Row(c.Key("devtools-uuid-controls")).FillWidth().Height(t.SizeSegmentHeight).Shrink(0).Gap(t.Space8).AlignItems(ui.Center).Children(func() {
		label := text("DevtoolsRegenerate")
		button := a.qrIconButton(c, t, "devtools-regenerate", "refresh-cw", label)
		button.OnClick(a.regenerateDevtoolsUUID)
		ui.Row(c).Grow(1).FillHeight()
		a.devtoolsSwitch(c, t, "devtools-upper", text("DevtoolsUpper"), s.upper, func() {
			a.writeDevtoolsSwitches(!s.upper, s.plain)
		})
		a.devtoolsSwitch(c, t, "devtools-plain", text("DevtoolsPlain"), s.plain, func() {
			a.writeDevtoolsSwitches(s.upper, !s.plain)
		})
	})
}

// devtoolsSwitch is one small labelled switch of the UUID toolbar. It has no
// second line and no explanation: the label is the whole control.
func (a *application) devtoolsSwitch(c *ui.Context, t tokenValues, key, label string, value bool, changed func()) {
	background, thumb, align := t.ColorControl, t.ColorTextSecondary, ui.Start
	if value {
		background, thumb, align = t.ColorAccent, t.ColorThumb, ui.End
	}
	ui.Row(c).Shrink(0).Height(t.SizeSegmentHeight).Gap(t.Space6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
		ui.SwitchBase(c.Key(key), &value).Size(t.SizeSwitchWidth, t.SizeSwitchHeight).Radius(t.RadiusControl).
			Background(ui.Hex(background)).Padding(t.Space2).Justify(align).AlignItems(ui.Center).
			Label(label).FocusRing(true).OnChange(changed).Children(func() {
			ui.Box(c).Size(t.SizeSwitchThumb, t.SizeSwitchThumb).Radius(t.SizeSwitchThumb / 2).Background(ui.Hex(thumb))
		})
	})
}

// devtoolsSelectionHint is the one-line permission guide. It is this module's
// own row: a denied read never writes the translate or QR state.
func (a *application) devtoolsSelectionHint(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	label := text("DevtoolsSelectionDenied")
	ui.Row(c.Key("devtools-selection-hint")).FillWidth().Height(t.SizeSegmentHeight).Shrink(0).
		Gap(t.Space8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Grow(1).MinWidth(0).FontSize(t.FontSizeSmall).
			LineHeight(t.LineHeightBody).TextColor(ui.Hex(t.ColorTextSecondary)).
			SingleLine().Label(label).Tooltip(label)
		a.translateTextButton(c, t, "devtools-selection-grant", text("QRGrantAccess"), a.grantAccessibility)
	})
}

// devtoolsList draws the current tab's rows. A batch is one row with every
// value on its own line; a single value is one row with its title and hint.
func (a *application) devtoolsList(c *ui.Context, t tokenValues, locale string, s *devtoolsState) {
	rows := a.devtoolsWindowRows(locale)
	if len(rows) == 0 {
		return
	}
	ui.Scroll(c.Key("devtools-scroll")).FillWidth().Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Gap(t.Space4).Children(func() {
			for index, row := range rows {
				a.devtoolsRow(c, t, locale, s, index, row)
			}
		})
	})
}

func (a *application) devtoolsRow(c *ui.Context, t tokenValues, locale string, s *devtoolsState, index int, row devtoolsRow) {
	selected := index == s.selected
	background := t.ColorTransparent
	button := ui.ButtonBase(c.Key(row.Key)).FillWidth().Shrink(0).Radius(t.RadiusCard).FocusRing(true).Label(row.Title)
	if selected {
		background = t.ColorAccentSoft
	} else if button.Hovered() {
		background = t.ColorNavHover
	}
	button.Background(ui.Hex(background)).Padding(t.Space4, t.Space8)
	if row.ErrorKey == "" {
		button.OnClick(func() {
			s.selected = index
			a.copyDevtoolsRow(row)
		})
	} else {
		button.OnClick(func() {
			s.selected = index
			a.refreshDevtools()
		})
	}
	button.Children(func() {
		ui.Row(c).Fill().Gap(t.Space8).AlignItems(ui.Center).Children(func() {
			if row.Label != "" {
				ui.Text(c, row.Label).Shrink(0).FontSize(t.FontSizeCaption).
					TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
			}
			if len(row.Lines) > 0 {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					for _, line := range row.Lines {
						ui.Text(c, line).FillWidth().Font(t.FontFamilyCode).FontSize(t.FontSizeSmall).
							TextColor(ui.Hex(t.ColorText)).SingleLine().Ellipsis("…")
					}
				})
			} else {
				ui.Text(c, row.Title).Grow(1).MinWidth(0).Font(t.FontFamilyCode).FontSize(t.FontSizeSmall).
					TextColor(ui.Hex(t.ColorText)).SingleLine().Ellipsis("…")
			}
			if row.Hint != "" {
				ui.Text(c, row.Hint).Shrink(0).MaxWidth(t.SizePaletteWidth / 2).FontSize(t.FontSizeSmall).
					TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine().Ellipsis("…")
			}
			switch {
			case row.ErrorKey != "":
				label := a.text(locale, row.ErrorKey)
				alert := ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).
					TextColor(ui.Hex(t.ColorDanger)).Label(label).Description(label)
				ui.TooltipBase(c, alert, func(tip ui.Element) {
					tip.Padding(t.Space6, t.Space8).Radius(t.RadiusControl).Background(ui.Hex(t.ColorTooltip)).
						TextColor(ui.Hex(t.ColorTooltipText)).Children(func() {
						ui.Text(c, label).FontSize(t.FontSizeSmall)
					})
				})
			case s.copyFailedKey == row.Key:
				label := a.text(locale, "DevtoolsCopyFailed")
				ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).
					TextColor(ui.Hex(t.ColorDanger)).Label(label).Description(label).Tooltip(label)
			case s.copiedKey == row.Key:
				ui.Icon(c, icon("check")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorText))
			}
		})
	})
}

// devtoolsRequestForQuery is the complete request behind one query's result
// rows, so the palette's "open in window" never re-generates by its defaults.
func devtoolsRequestForQuery(query string) devtoolsRequest {
	query = normalizeQuery(query)
	if generation, ok := devtools.ParseGeneration(query); ok {
		return devtoolsRequest{Kind: devtools.KindUUID, Version: generation.Version, Count: generation.Count}
	}
	if output, ok := devtools.ParseUUID(query); ok {
		// A UUID literal opens on its own value, not as a fresh batch.
		return devtoolsRequest{Kind: devtools.KindUUID, Text: output.Payload}
	}
	return devtoolsRequest{Kind: devtools.KindTime, Text: query}
}

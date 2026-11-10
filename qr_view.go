package main

import (
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// qrBackground is the symbol's own white, not the interface's: the bitmap is
// drawn black on white, and an inverted symbol is unreadable in some scanners.
// It is the one colour the view writes by hand, and it never follows the theme.
const qrBackground = "#FFFFFF"

func (a *application) qrView(c *ui.Context) {
	locale, t := applyTheme(c)
	s := a.qr
	if s == nil {
		return
	}
	if a.qrCopyExpired(c.Now()) {
		s.copiedUntil = time.Time{}
	}
	if s.copiedUntil.After(c.Now()) {
		c.After(s.copiedUntil.Sub(c.Now()))
	}
	c.OnShortcut(0, ui.KeyEscape, a.closeQR)
	c.OnShortcut(ui.Cmd, ui.KeyW, a.closeQR)
	c.OnShortcut(ui.Cmd|ui.Alt, ui.KeyP, func() { a.setQRPinned(!s.pinned) })
	c.Root().Background(ui.Hex(t.ColorCard))
	ui.Column(c.Key("qr-root")).Fill().Children(func() {
		a.qrToolbar(c, t, locale, s)
		ui.Box(c).FillWidth().Height(t.StrokeHairline).Background(ui.Hex(t.ColorSeparator))
		ui.Column(c.Key("qr-panel")).FillWidth().Grow(1).MinHeight(0).
			Padding(t.Space12).Gap(t.Space8).Children(func() {
			a.qrInputArea(c, t, locale, s)
			if s.denied {
				a.qrSelectionHint(c, t, locale)
			}
			a.qrSymbolArea(c, t, locale, s)
		})
	})
	// The debounce needs no timer of its own: the view asks for the frame that
	// starts the encode, and the encode invalidates the window again.
	if !s.due.IsZero() {
		if now := c.Now(); now.Before(s.due) {
			c.After(s.due.Sub(now))
		} else {
			text := s.input
			s.due = time.Time{}
			a.encodeQR(text)
		}
	}
}

func (a *application) qrToolbar(c *ui.Context, t tokenValues, locale string, s *qrState) {
	text := func(key string) string { return a.text(locale, key) }
	ui.Row(c.Key("qr-toolbar")).FillWidth().Height(t.SizeJSONToolbarHeight).Shrink(0).
		Padding(t.Space8, t.Space12, t.Space8, t.SizeJSONTitleInset).Gap(t.Space8).AlignItems(ui.Center).Children(func() {
		ui.Row(c.Key("qr-title-drag")).Grow(1).FillHeight().DragWindow().AlignItems(ui.Center).Children(func() {
			ui.Text(c, text("QR")).FontSize(t.FontSizeSmall).
				TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
		})
		label := text("R07Pin")
		if s.pinned {
			label = text("R07Unpin")
		}
		pin := a.qrIconButton(c, t, "qr-pin", "pin", label)
		if s.pinned {
			pin.Background(ui.Hex(t.ColorAccentSoft))
		}
		pin.OnClick(func() { a.setQRPinned(!s.pinned) })
		usable := s.code != nil && s.encoded == s.input
		a.qrActionButton(c, t, "qr-copy", "copy", text("QRCopy"), usable, s.copiedUntil.After(c.Now()),
			text("QRCopied"), a.copyQRImage)
		a.qrActionButton(c, t, "qr-save", "download", text("QRSave"), usable, false, "", a.saveQRImage)
	})
}

// qrIconButton draws one icon button of the toolbar. The caller decides whether
// it is clickable and what marks it as active.
func (a *application) qrIconButton(c *ui.Context, t tokenValues, key, glyph, label string) ui.Element {
	button := ui.ButtonBase(c.Key(key)).Size(t.SizeIconButton, t.SizeIconButton).
		Shrink(0).Radius(t.RadiusControl).FocusRing(true).Label(label).Tooltip(label)
	if button.Hovered() {
		button.Background(ui.Hex(t.ColorControlHover))
	}
	button.Children(func() {
		ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Icon(c, icon(glyph)).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
		})
	})
	return button
}

// qrActionButton is a toolbar button that needs something encoded. A short
// success state swaps its glyph for a check, as the translate copy button does.
func (a *application) qrActionButton(c *ui.Context, t tokenValues, key, glyph, label string, usable, done bool, doneLabel string, run func()) {
	shown, name := label, glyph
	if done {
		shown, name = doneLabel, "check"
	}
	button := a.qrIconButton(c, t, key, name, shown)
	if !usable {
		button.Disabled(true).Opacity(t.OpacityDisabled)
		return
	}
	button.OnClick(run)
}

func (a *application) qrInputArea(c *ui.Context, t tokenValues, locale string, s *qrState) {
	text := func(key string) string { return a.text(locale, key) }
	ui.Box(c).FillWidth().Height(t.SizeQRInputHeight).Shrink(0).Children(func() {
		input := ui.TextArea(c.Key("qr-input"), &s.input).Fill().MinHeight(0).Padding(t.Space4, t.Space12).
			Border(0, ui.Hex(t.ColorTransparent)).Radius(t.RadiusControl).Background(ui.Hex(t.ColorControl)).
			FontSize(t.FontSizeBody).LineHeight(t.LineHeightBody).
			Placeholder(text("QRInputPlaceholder")).Label(text("QRInputPlaceholder")).FocusRing(true)
		input.OnChange(a.syncQRInput)
		if s.focus {
			s.focus = false
			input.Focus()
		}
	})
}

// qrSelectionHint is the one-line permission guide under the input. It is the
// module's own row, so a denied read never writes the translate state.
func (a *application) qrSelectionHint(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	label := text("QRSelectionDenied")
	ui.Row(c.Key("qr-selection-hint")).FillWidth().Height(t.SizeSegmentHeight).Shrink(0).
		Gap(t.Space8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Grow(1).MinWidth(0).FontSize(t.FontSizeSmall).
			LineHeight(t.LineHeightBody).TextColor(ui.Hex(t.ColorTextSecondary)).
			SingleLine().Label(label).Tooltip(label)
		a.translateTextButton(c, t, "qr-selection-grant", text("QRGrantAccess"), a.grantAccessibility)
	})
}

// qrSymbolArea is the white square with the bitmap centred in it. The bitmap
// already carries the quiet zone and is built from whole device pixels, so it is
// never stretched: a fractional scale would blur every module edge.
func (a *application) qrSymbolArea(c *ui.Context, t tokenValues, locale string, s *qrState) {
	text := func(key string) string { return a.text(locale, key) }
	box := a.qrBoxSide()
	ui.Row(c).FillWidth().Grow(1).MinHeight(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		square := ui.Box(c.Key("qr-symbol-box")).Size(box, box).Shrink(0).Radius(t.RadiusCard).
			Background(ui.Hex(qrBackground))
		square.Children(func() {
			// The box is white in both themes, so what it holds is drawn with
			// the light theme's text colours: a dark-mode grey on white would
			// be unreadable.
			a.qrSymbolContent(c, tokens(false, locale), locale, s)
		})
		s.boxBounds = square.Bounds()
	})
	if s.saveFailed {
		label := text("QRSaveFailed")
		ui.Row(c.Key("qr-save-failed")).FillWidth().Gap(t.Space6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).
				TextColor(ui.Hex(t.ColorTextSecondary)).Tooltip(label)
			ui.Text(c, label).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
		})
	}
}

func (a *application) qrSymbolContent(c *ui.Context, t tokenValues, locale string, s *qrState) {
	text := func(key string) string { return a.text(locale, key) }
	switch s.phase {
	case qrEmpty:
		ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(t.Space8).Children(func() {
			ui.Icon(c, icon("qr-code")).Size(t.SizeIconLarge, t.SizeIconLarge).
				TextColor(ui.Hex(t.ColorTextDisabled))
			ui.Text(c, text("QREmpty")).FontSize(t.FontSizeSmall).LineHeight(t.LineHeightBody).
				TextColor(ui.Hex(t.ColorTextDisabled)).SingleLine()
		})
	case qrReady:
		if s.bitmap == nil || s.code == nil || s.scale <= 0 {
			return
		}
		// The bitmap's pixels are shown as DIPs, so the element is sized in
		// DIPs by dividing the device-pixel side by the display scale.
		side := float32(qrBitmapSide(s.code.Modules(), s.px)) / float32(s.scale)
		ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			image := ui.Image(c, s.bitmap).Size(side, side).Shrink(0).
				Label(text("QRDragHint")).Tooltip(text("QRDragHint"))
			image.DragDataFrom(func() transfer.Data {
				png, err := a.qrPNG()
				if err != nil {
					return transfer.Data{}
				}
				return transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, png)))
			})
		})
	default:
		key := "QRFailed"
		if s.phase == qrTooLong {
			key = "QRTooLong"
		}
		ui.Column(c).Fill().Padding(t.Space16).AlignItems(ui.Center).Justify(ui.Center).
			Gap(t.Space8).Children(func() {
			ui.Icon(c, icon("triangle-alert")).Size(t.SizeIconLarge, t.SizeIconLarge).
				TextColor(ui.Hex(t.ColorTextSecondary))
			ui.Text(c, text(key)).FontSize(t.FontSizeBody).LineHeight(t.LineHeightBody).
				TextColor(ui.Hex(t.ColorText)).SingleLine().Label(text(key))
			if s.phase == qrTooLong {
				ui.Text(c, text("QRTooLongHint")).FontSize(t.FontSizeSmall).
					LineHeight(t.LineHeightBody).TextColor(ui.Hex(t.ColorTextSecondary)).
					SingleLine().Tooltip(text("QRTooLongHint"))
			}
		})
	}
}

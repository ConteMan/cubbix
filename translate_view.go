package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// translateResultLines estimates how many rows the译文 needs. The result
// block scrolls internally once it reaches SizeTranslateResultMax, so the
// window never grows past SizeTranslateMaxHeight.
func translateResultLines(value string) int {
	const columns = 56
	lines := 0
	for _, line := range splitTranslateLines(value) {
		lines++
		count := len([]rune(line))
		if count > columns {
			lines += (count - 1) / columns
		}
	}
	if lines < 1 {
		lines = 1
	}
	return lines
}

func clampTranslate(value, low, high float32) float32 {
	return float32(math.Max(float64(low), math.Min(float64(high), float64(value))))
}

// translateResultHeight is 44 for the empty state; from loading onwards the
// lower bound is 64 (two rows), and the upper bound is SizeTranslateResultMax.
// The dictionary block below it has its own fixed height.
func (a *application) translateResultHeight(t tokenValues, s *translateState) float32 {
	needed := 2*t.Space12 + float32(translateResultLines(s.output))*t.FontSizeBody*t.LineHeightBody
	switch s.phase {
	case translateFailed, translateNotConfigured:
		needed += t.Space8 + t.SizeSegmentHeight
	case translateLoading, translateStreaming:
		needed += t.FontSizeSmall * t.LineHeightBody
	}
	return clampTranslate(needed, 64, t.SizeTranslateResultMax)
}

// dictionaryHeaderHeight is the folded row (chevron + label).
func dictionaryHeaderHeight(t tokenValues) float32 { return t.Space20 + t.Space4 }

// dictionaryBodyHeight is the expanded preview: the first few lines, bounded
// by SizeTranslateDictionaryMax. Longer definitions scroll inside the block.
func dictionaryBodyHeight(t tokenValues) float32 {
	return clampTranslate(t.SizeTranslateDictionaryPreviewLines*t.FontSizeSmall*t.LineHeightBody,
		0, t.SizeTranslateDictionaryMax)
}

// dictionaryBlockHeight includes the block's own padding. Collapsed it is
// only the header row; expanded it adds the preview body.
func dictionaryBlockHeight(t tokenValues, expanded bool) float32 {
	height := 2*(t.Space12+t.StrokeHairline) + dictionaryHeaderHeight(t)
	if expanded {
		height += dictionaryBodyHeight(t)
	}
	return height
}

// translateHeight is the panel height (without the shadow margin). Its only
// inputs are tokens and state, so it can be asserted without a window.
func translateNonResultHeight(t tokenValues, s *translateState) float32 {
	input := t.SizeTranslateInputBase + float32(translateInputLines(s.input)-1)*t.SizeTranslateInputLine
	height := t.Space12 + input + t.Space8 + t.SizeTranslateDirectionHeight + t.Space8 + t.Space12 + 2*t.StrokeHairline
	if s.clipped > 0 {
		height += t.FontSizeSmall*t.LineHeightBody + t.Space8
	}
	// The separator sits between the translation and the dictionary, with a
	// Space8 gap on each side contributed by the panel column.
	if strings.TrimSpace(s.dictionary) != "" {
		height += t.Space8 + t.StrokeHairline + t.Space8 + dictionaryBlockHeight(t, s.dictionaryExpanded)
	}
	return height
}

func (a *application) translateHeight(t tokenValues, s *translateState) float32 {
	if s == nil {
		return t.SizeTranslateMinHeight
	}
	height := translateNonResultHeight(t, s) + a.translateResultHeightForState(t, s)
	return clampTranslate(height, t.SizeTranslateMinHeight, t.SizeTranslateMaxHeight)
}

func (a *application) translateView(c *ui.Context) {
	locale, t := applyTheme(c)
	s := a.translate
	if s == nil {
		return
	}
	text := func(key string) string { return a.text(locale, key) }
	if a.translateCopyExpired(c.Now()) {
		s.copiedUntil = time.Time{}
	}
	c.OnShortcut(0, ui.KeyEscape, a.closeTranslate)
	c.Root().Background(ui.Transparent)
	ui.Box(c.Key("translate-root")).Fill().Padding(t.SizePaletteShadowMargin).Children(func() {
		ui.Column(c.Key("translate-panel")).Fill().Padding(t.Space12).Gap(t.Space8).Radius(t.RadiusWindow).
			Background(ui.Hex(t.ColorMenuSurface)).
			Border(t.StrokeHairline, ui.Hex(t.ColorPanelStrokeOuter)).
			InsetShadow(0, 0, 0, t.StrokeHairline, ui.Hex(t.ColorPanelStrokeInner)).
			Shadow(0, t.Space8, t.Space24, 0, ui.Hex(t.ColorPanelShadow)).Children(func() {
			a.translateInputArea(c, t, locale, s)
			if s.clipped > 0 {
				ui.Text(c, fmt.Sprintf(text("TranslateTooLong"), maxTranslateInput)).
					Height(t.FontSizeSmall * t.LineHeightBody).FontSize(t.FontSizeSmall).
					LineHeight(t.LineHeightBody).TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
			}
			a.translateDirectionRow(c, t, locale, s)
			a.translateResultBlock(c, t, locale, s)
			if strings.TrimSpace(s.dictionary) != "" {
				ui.Box(c.Key("translate-dictionary-separator")).FillWidth().
					Height(t.StrokeHairline).Background(ui.Hex(t.ColorSeparator))
				a.translateDictionaryBlock(c, t, locale, s)
			}
		})
	})
}

func (a *application) translateInputArea(c *ui.Context, t tokenValues, locale string, s *translateState) {
	text := func(key string) string { return a.text(locale, key) }
	height := t.SizeTranslateInputBase + float32(translateInputLines(s.input)-1)*t.SizeTranslateInputLine
	ui.Row(c).FillWidth().Height(height).Gap(t.Space8).Children(func() {
		ui.Box(c).Grow(1).FillHeight().MinWidth(0).Children(func() {
			input := ui.TextArea(c.Key("translate-source"), &s.input).Fill().Padding(0).
				Border(0, ui.Hex(t.ColorTransparent)).Background(ui.Hex(t.ColorTransparent)).
				FontSize(t.FontSizeBody).LineHeight(t.LineHeightBody).
				Placeholder(text("TranslateInputPlaceholder")).Label(text("TranslateInputPlaceholder")).FocusRing(true)
			input.OnTextKey(func(k ui.TextKey) bool {
				if k.Key != ui.KeyEnter || k.Mods != 0 {
					return false // Shift+Enter breaks the line.
				}
				s.input = k.Text()
				a.submitTranslate()
				k.SetText(s.input)
				return true
			})
			input.OnChange(a.syncTranslateInput)
			if s.focus {
				s.focus = false
				input.Focus()
			}
		})
		close := ui.ButtonBase(c.Key("translate-close")).Size(t.SizeIconButton, t.SizeIconButton).
			Radius(t.RadiusControl).FocusRing(true).Label(text("TranslateClose")).
			Tooltip(text("TranslateClose")).OnClick(a.closeTranslate)
		close.Children(func() {
			ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Icon(c, icon("x")).Size(t.SizeIconSmall, t.SizeIconSmall).TextColor(ui.Hex(t.ColorTextSecondary))
			})
		})
	})
}

func (a *application) translateDirectionRow(c *ui.Context, t tokenValues, locale string, s *translateState) {
	text := func(key string) string { return a.text(locale, key) }
	_, target := a.translateDirection(s)
	label := text("TranslateToEnglish")
	if target == "zh-Hans" {
		label = text("TranslateToChinese")
	}
	ui.ButtonBase(c.Key("translate-direction")).FillWidth().Height(t.SizeTranslateDirectionHeight).
		Justify(ui.Start).AlignItems(ui.Center).
		FocusRing(true).Label(text("TranslateSwapDirection")).Tooltip(text("TranslateSwapDirection")).
		OnClick(a.swapTranslateDirection).Children(func() {
		ui.Text(c, label).FontSize(t.FontSizeSmall).LineHeight(t.LineHeightBody).
			TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
	})
}

func (a *application) translateResultBlock(c *ui.Context, t tokenValues, locale string, s *translateState) {
	ui.Row(c).FillWidth().Height(a.translateResultHeightForState(t, s)).Radius(t.RadiusCard).
		Background(ui.Hex(t.ColorCard)).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).
		Padding(t.Space12).Gap(t.Space8).AlignItems(ui.Start).Children(func() {
		ui.Scroll(c).Grow(1).FillHeight().MinWidth(0).AlignItems(ui.Start).Justify(ui.Start).Children(func() {
			ui.Column(c).FillWidth().AlignItems(ui.Start).Justify(ui.Start).Children(func() {
				a.translateResultContent(c, t, locale, s)
			})
		})
		if s.phase == translateLoading || s.phase == translateStreaming || s.phase == translateDone {
			a.translateCopyButton(c, t, locale, s)
		}
	})
}

func (a *application) translateResultHeightForState(t tokenValues, s *translateState) float32 {
	if s.phase == translateEmpty {
		return 44
	}
	return min(a.translateResultHeight(t, s), t.SizeTranslateMaxHeight-translateNonResultHeight(t, s))
}

// translateDictionaryBlock draws the local definition below the translation.
// The header row is the fold button; expanded, the plain text is split into
// readable lines and scrolls inside a bounded preview area.
func (a *application) translateDictionaryBlock(c *ui.Context, t tokenValues, locale string, s *translateState) {
	text := func(key string) string { return a.text(locale, key) }
	label := text("TranslateDictionary")
	glyph, tooltip := "chevron-down", text("TranslateDictionaryCollapse")
	if !s.dictionaryExpanded {
		glyph, tooltip = "chevron-right", text("TranslateDictionaryExpand")
	}
	ui.Column(c.Key("translate-dictionary")).FillWidth().
		Height(dictionaryBlockHeight(t, s.dictionaryExpanded)).Radius(t.RadiusCard).
		Background(ui.Hex(t.ColorCard)).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).
		Padding(t.Space12).Children(func() {
		header := ui.ButtonBase(c.Key("translate-dictionary-toggle")).FillWidth().
			Height(dictionaryHeaderHeight(t)).Justify(ui.Start).AlignItems(ui.Center).
			FocusRing(true).Label(label).Tooltip(tooltip).OnClick(a.toggleDictionary)
		header.Children(func() {
			ui.Row(c).Gap(t.Space4).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icon(glyph)).Size(t.SizeIconSmall, t.SizeIconSmall).
					TextColor(ui.Hex(t.ColorTextSecondary))
				ui.Text(c, label).FontSize(t.FontSizeSmall).LineHeight(t.LineHeightBody).
					TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
			})
		})
		if !s.dictionaryExpanded {
			return
		}
		ui.Scroll(c.Key("translate-dictionary-body")).FillWidth().
			Height(dictionaryBodyHeight(t)).AlignItems(ui.Start).Justify(ui.Start).Children(func() {
			ui.Column(c).FillWidth().AlignItems(ui.Start).Children(func() {
				for _, segment := range dictionarySegments(s.dictionary) {
					ui.Text(c, segment).FillWidth().FontSize(t.FontSizeSmall).
						LineHeight(t.LineHeightBody).TextColor(ui.Hex(t.ColorTextSecondary)).
						Selectable().Label(segment)
				}
			})
		})
	})
}

func (a *application) translateResultContent(c *ui.Context, t tokenValues, locale string, s *translateState) {
	text := func(key string) string { return a.text(locale, key) }
	switch s.phase {
	case translateEmpty:
		ui.Text(c, text("TranslateEmpty")).FontSize(t.FontSizeSmall).
			TextColor(ui.Hex(t.ColorTextDisabled)).SingleLine()
	case translateLoading:
		ui.Row(c).FillWidth().Gap(t.Space8).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(t.SizeIcon, t.SizeIcon).Label(text("TranslateBusy"))
			ui.Text(c, text("TranslateBusy")).FontSize(t.FontSizeSmall).
				TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
		})
	case translateStreaming, translateDone:
		ui.Row(c).FillWidth().Gap(t.Space4).AlignItems(ui.Start).Children(func() {
			ui.Text(c, s.output).Grow(1).MinWidth(0).FontSize(t.FontSizeBody).
				LineHeight(t.LineHeightBody).TextColor(ui.Hex(t.ColorText)).Selectable()
			if s.phase == translateStreaming {
				ui.Box(c).Size(t.Space4, t.FontSizeBody*t.LineHeightBody).Shrink(0).
					Background(ui.Hex(t.ColorTextSecondary))
			}
		})
	case translateFailed:
		key := s.errorKey
		if key == "" {
			key = "TranslateServerError"
		}
		ui.Column(c).FillWidth().Gap(t.Space8).Children(func() {
			ui.Row(c).FillWidth().Gap(t.Space8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger))
				ui.Text(c, text(key)).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorText)).SingleLine()
			})
			a.translateTextButton(c, t, "translate-retry", text("TranslateRetry"), a.submitTranslate)
		})
	case translateNotConfigured:
		ui.Column(c).FillWidth().Gap(t.Space8).Children(func() {
			ui.Text(c, text("TranslateNotConfigured")).FontSize(t.FontSizeBody).
				TextColor(ui.Hex(t.ColorText)).SingleLine()
			a.translateTextButton(c, t, "translate-open-settings", text("TranslateOpenSettings"), a.openTranslateSettings)
		})
	}
}

func (a *application) translateTextButton(c *ui.Context, t tokenValues, key, label string, run func()) {
	button := ui.ButtonBase(c.Key(key)).Height(t.SizeSegmentHeight).PaddingX(t.Space8).
		Radius(t.RadiusControl).FocusRing(true).Label(label).Tooltip(label).OnClick(run)
	button.Children(func() {
		ui.Text(c, label).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorText)).SingleLine()
	})
}

func (a *application) translateCopyButton(c *ui.Context, t tokenValues, locale string, s *translateState) {
	text := func(key string) string { return a.text(locale, key) }
	usable := (s.phase == translateStreaming || s.phase == translateDone) && strings.TrimSpace(s.output) != ""
	now := c.Now()
	copied := !s.copiedUntil.IsZero() && now.Before(s.copiedUntil)
	if copied {
		c.After(s.copiedUntil.Sub(now))
	}
	glyph, label := "copy", text("TranslateCopy")
	if copied {
		glyph, label = "check", text("TranslateCopied")
	}
	button := ui.ButtonBase(c.Key("translate-copy")).Size(t.SizeIconButton, t.SizeIconButton).
		Shrink(0).Radius(t.RadiusControl).FocusRing(true).Label(label).Tooltip(label)
	if usable {
		button = button.OnClick(a.copyTranslateResult)
	} else {
		button = button.Disabled(true).Opacity(t.OpacityDisabled)
	}
	button.Children(func() {
		ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Icon(c, icon(glyph)).Size(t.SizeIconSmall, t.SizeIconSmall).TextColor(ui.Hex(t.ColorTextSecondary))
		})
	})
}

// translateCopyExpired drops a stale copy marker without needing a timer:
// the view asks for a frame while the marker is visible and clears it later.
func (a *application) translateCopyExpired(now time.Time) bool {
	s := a.translate
	if s == nil || s.copiedUntil.IsZero() {
		return false
	}
	return !now.Before(s.copiedUntil)
}

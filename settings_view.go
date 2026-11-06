package main

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Settings pages are stable numbers: a saved window, a URL or the command
// line may name one, and the sidebar follows this order.
const (
	settingsGeneral   = 0
	settingsAbout     = 1
	settingsAwake     = 2
	settingsShortcuts = 3
	settingsJSON      = 4
)

// settingsNav is the sidebar order: module pages first, global pages after.
var settingsNav = []struct {
	page       int
	glyph, key string
}{
	{settingsGeneral, "settings-2", "General"},
	{settingsAwake, "sun", "Awake"},
	{settingsJSON, "braces", "JSON"},
	{settingsShortcuts, "keyboard", "R05Shortcuts"},
	{settingsAbout, "info", "About"},
}

func settingsPageName(page int) string {
	switch page {
	case settingsAwake:
		return "awake"
	case settingsAbout:
		return "about"
	case settingsShortcuts:
		return "shortcuts"
	case settingsJSON:
		return "json"
	}
	return "general"
}

// settingsLayout is everything a page's height and its rows both depend on. It
// is read once per frame so the view and the measurement can never disagree.
type settingsLayout struct {
	page                   int
	awakeOn, jsonOn, ready bool
	showPlacement          bool
	fixedDisplay           bool
	hasCLI                 bool
	cli                    cliPaths
	cliState               cliLinkState
	// displays is this frame's display snapshot: the placement rows and their
	// height conditions both come from it, never from a second read.
	displays []mygo.Display
	// display is the screen the fixed choice resolved to, and displayMissing
	// says it was not found, so the window will fall back.
	display        mygo.Display
	displayMissing bool
}

// newSettingsLayout takes the same snapshot the frame draws, displays
// included: the two placement rows and the page height read one snapshot.
func newSettingsLayout(a *application, page int) settingsLayout {
	displays := a.displaySnapshot()
	l := settingsLayout{
		page:     page,
		awakeOn:  a.config.AwakeEnabled,
		jsonOn:   a.config.JSONEnabled,
		ready:    a.updates.snapshot().Phase == updateReady,
		displays: displays,
		// A single display needs no choice, so the whole group stays hidden
		// and the stored choice is kept for when a second one returns.
		showPlacement: len(displays) >= 2,
		fixedDisplay:  a.config.WindowPlacement == "display",
	}
	if l.showPlacement && l.fixedDisplay {
		display, matched := fixedDisplay(displays, a.config)
		l.display, l.displayMissing = display, !matched
	}
	if paths, ok := cliPathsFor(); ok {
		l.cli = paths
		l.cliState = paths.status()
		l.hasCLI = l.cliState != cliAbsent
	}
	return l
}

// rowHeight is the height of one settings row. Only level 2 grows: the second
// line needs Space16 more, split into Space8 above and below.
func rowHeight(t tokenValues, level int) float32 {
	if level == 2 {
		return t.SizeRowHeight + t.Space16
	}
	return t.SizeRowHeight
}

func cardHeight(t tokenValues, rows ...float32) float32 {
	var height float32
	for index, row := range rows {
		if index > 0 {
			height += t.StrokeHairline
		}
		height += row
	}
	return height
}

// sectionHeight is the header card plus one group title and card per section.
// Each section pays Space8 before its title, which doubles as the gap between
// two groups.
func sectionHeight(t tokenValues, cards ...float32) float32 {
	height := t.SizePageHeader
	for _, card := range cards {
		height += t.Space8 + t.SizeGroupHeader + t.Space8 + card
	}
	return height
}

// pageHeight is the natural height of one page, before the window padding.
func pageHeight(t tokenValues, page int, awakeOn, showPlacement, fixedDisplay, hasCLI, ready bool) float32 {
	switch page {
	case settingsGeneral:
		rows := []float32{rowHeight(t, 0), rowHeight(t, 1)}
		if hasCLI {
			rows = append(rows, rowHeight(t, 1))
		}
		if showPlacement {
			rows = append(rows, rowHeight(t, 3))
			if fixedDisplay {
				rows = append(rows, rowHeight(t, 0))
			}
		}
		return sectionHeight(t, cardHeight(t, rows...))
	case settingsAbout:
		height := sectionHeight(t, cardHeight(t, rowHeight(t, 0), rowHeight(t, 0), rowHeight(t, 2)))
		if ready {
			height += t.Space8 + t.SizeRowHeight + t.Space24
		}
		return height
	case settingsAwake:
		if awakeOn {
			return sectionHeight(t, cardHeight(t, rowHeight(t, 2)), cardHeight(t, rowHeight(t, 3)))
		}
		return sectionHeight(t, cardHeight(t, rowHeight(t, 3)))
	case settingsShortcuts:
		return sectionHeight(t, cardHeight(t, rowHeight(t, 3), rowHeight(t, 3)),
			cardHeight(t, rowHeight(t, 3), rowHeight(t, 3)))
	case settingsJSON:
		return sectionHeight(t, cardHeight(t, rowHeight(t, 3)))
	}
	return 0
}

// sidebarNaturalHeight is the tallest the sidebar can be: padding, the
// traffic light slot, the brand row, the separator and five navigation items.
func sidebarNaturalHeight(t tokenValues) float32 {
	nav := float32(len(settingsNav))*t.SizeNavItem + float32(len(settingsNav)-1)*t.Space4
	return 2*t.Space12 + t.Space12 + 3*t.Space16 + t.Space20 + t.StrokeHairline + nav
}

// settingsMinimumHeight keeps every page's tallest state visible at once. It
// never follows the page on screen, the display count or the CLI state.
func settingsMinimumHeight(t tokenValues) float32 {
	content := max(
		pageHeight(t, settingsGeneral, true, true, true, true, false),
		pageHeight(t, settingsAbout, true, false, false, false, true),
		pageHeight(t, settingsAwake, true, false, false, false, false),
		pageHeight(t, settingsShortcuts, true, false, false, false, false),
		pageHeight(t, settingsJSON, true, false, false, false, false),
	)
	height := max(t.SizeWindowMinHeight, sidebarNaturalHeight(t), content+2*t.Space24)
	return ceilingTo(t, height)
}

// settingsMinimumSize is the window's floor. Both axes come from tokens, so no
// page can quietly lower it.
func settingsMinimumSize() (int, int) {
	t := tokens(false, "zh-CN")
	return int(ceilingTo(t, t.SizeWindowWidth)), int(settingsMinimumHeight(t))
}

func settingsSize(width, height int) (int, int) {
	minWidth, minHeight := settingsMinimumSize()
	return max(minWidth, width), max(minHeight, height)
}

func ceilingTo(t tokenValues, value float32) float32 {
	return float32(math.Ceil(float64(value/t.Space4))) * t.Space4
}

// settingsNaturalHeight is the page's own height plus the window padding,
// rounded up. It is not the window minimum: a page never shrinks the window.
func settingsNaturalHeight(t tokenValues, page int, awakeOn, showPlacement, fixedDisplay, hasCLI, ready bool) int {
	return int(ceilingTo(t, pageHeight(t, page, awakeOn, showPlacement, fixedDisplay, hasCLI, ready)+2*t.Space24))
}

func settingsHeight(t tokenValues, page int, awakeOn, showPlacement, fixedDisplay, hasCLI, ready bool) int {
	return int(max(settingsMinimumHeight(t), ceilingTo(t, pageHeight(t, page, awakeOn, showPlacement, fixedDisplay, hasCLI, ready)+2*t.Space24)))
}

func (l settingsLayout) height(t tokenValues) float32 {
	return pageHeight(t, l.page, l.awakeOn, l.showPlacement, l.fixedDisplay, l.hasCLI, l.ready)
}

// settingsRowOpts describes one settings row. The zero value is a plain row:
// no icon, no hint, no tooltip, no status.
type settingsRowOpts struct {
	icon     string // optional leading Lucide icon
	hint     string // level 1 inline hint or level 2 second line
	level    int    // 0 plain, 1 inline hint, 2 second line, 3 info tooltip
	tooltip  string // level 3 explanation, also the accessible description
	status   string // text drawn after a neutral status dot
	statusOn bool   // the dot is solid (ColorText) instead of disabled
	chevron  bool   // a trailing › marks a row that opens a subpage
	disabled bool   // the label and hint are disabled; controls decide for themselves
}

// settingsRow draws one row of a settings card. The caller supplies the control
// so the component never reads a module's state.
func (a *application) settingsRow(c *ui.Context, t tokenValues, label string, o settingsRowOpts, right func()) {
	control := func() {
		if right == nil {
			return
		}
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(t.Space8).Children(right)
	}
	labelBox := func(children func()) ui.Element {
		e := ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(t.Space8)
		if o.disabled {
			e = e.Disabled(true).Opacity(t.OpacityDisabled)
		}
		e.Children(children)
		return e
	}
	switch o.level {
	case 2:
		indent := float32(0)
		if o.icon != "" {
			indent = t.SizeIcon + t.Space8
		}
		ui.Column(c).FillWidth().Height(rowHeight(t, 2)).PaddingY(t.Space8).Gap(t.Space2).Children(func() {
			// The first line carries the control; the second one spans the row
			// below it, indented under the label when an icon leads the row.
			ui.Row(c).FillWidth().Height(t.SizeSegmentHeight).AlignItems(ui.Center).Gap(t.Space8).Children(func() {
				labelBox(func() {
					if o.icon != "" {
						ui.Icon(c, icon(o.icon)).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
					}
					ui.Text(c, label).FontSize(t.FontSizeBody).SingleLine()
				})
				control()
			})
			ui.Row(c).FillWidth().Height(t.FontSizeSmall * t.LineHeightBody).AlignItems(ui.Center).Children(func() {
				if indent > 0 {
					ui.Box(c).Width(indent).Shrink(0)
				}
				ui.Text(c, o.hint).FillWidth().MinWidth(0).SingleLine().Tooltip(o.hint).
					FontSize(t.FontSizeSmall).LineHeight(t.LineHeightBody).TextColor(ui.Hex(t.ColorTextSecondary))
			})
		})
		return
	case 3:
		ui.Row(c).FillWidth().Height(t.SizeRowHeight).AlignItems(ui.Center).Gap(t.Space8).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(t.Space6).Children(func() {
				title := ui.Text(c, label).FontSize(t.FontSizeBody).SingleLine()
				if o.disabled {
					// Only the title is disabled: the explanation beside it is
					// the reason the row cannot be used and must stay readable.
					title = title.Disabled(true).Opacity(t.OpacityDisabled)
				}
				if o.tooltip != "" {
					a.infoIcon(c, t, o.tooltip)
				}
			})
			control()
		})
		return
	case 1:
		ui.Row(c).FillWidth().Height(t.SizeRowHeight).AlignItems(ui.Center).Gap(t.Space8).Children(func() {
			if o.icon != "" {
				ui.Icon(c, icon(o.icon)).Size(t.SizeIcon, t.SizeIcon).Shrink(0).TextColor(ui.Hex(t.ColorTextSecondary))
			}
			// The two texts share one line box and sit on the same bottom edge;
			// the row itself stays SpaceRowHeight high and centres them.
			group := ui.Row(c).Grow(1).Height(t.FontSizeBody * t.LineHeightBody).MinWidth(0).AlignItems(ui.End).Gap(t.Space8)
			if o.disabled {
				group = group.Disabled(true).Opacity(t.OpacityDisabled)
			}
			group.Children(func() {
				ui.Text(c, label).FontSize(t.FontSizeBody).SingleLine()
				// The hint is one step smaller than the title and sits on the
				// same baseline, so the two never look vertically centered.
				ui.Text(c, o.hint).FontSize(t.FontSizeSmall).LineHeight(t.LineHeightBody).
					SingleLine().TextColor(ui.Hex(t.ColorTextSecondary))
			})
			control()
		})
		return
	default:
		ui.Row(c).FillWidth().Height(t.SizeRowHeight).AlignItems(ui.Center).Gap(t.Space8).Children(func() {
			labelBox(func() { ui.Text(c, label).FontSize(t.FontSizeBody).SingleLine() })
			if o.status != "" {
				a.statusDot(c, t, o.statusOn, o.status)
				ui.Text(c, o.status).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
			}
			if o.chevron {
				ui.Icon(c, icon("chevron-right")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
			}
			control()
		})
	}
}

// infoIcon is the level-3 anchor. It is its own enabled node: a disabled row
// body cannot swallow the explanation, and the tooltip stays reachable by
// pointer and by keyboard because the icon is focusable.
func (a *application) infoIcon(c *ui.Context, t tokenValues, message string) {
	e := ui.Icon(c, icon("info")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary)).
		Focusable().Label(message).Description(message)
	ui.TooltipBase(c, e, func(tip ui.Element) {
		tip.Padding(t.Space6, t.Space8).Radius(t.RadiusControl).MaxWidth(t.SizeMenubarPanelWidth).
			Background(ui.Hex(t.ColorTooltip)).TextColor(ui.Hex(t.ColorTooltipText)).Children(func() {
			ui.Text(c, message).FontSize(t.FontSizeSmall).LineHeight(t.LineHeightBody)
		})
	})
}

// alertIcon draws the failure marker. Its reason is formatted before it gets
// here, so no tooltip ever shows a resource key or an unreplaced %s.
func (a *application) alertIcon(c *ui.Context, t tokenValues, reason string) {
	e := ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).
		Focusable().Label(reason).Description(reason)
	ui.TooltipBase(c, e, func(tip ui.Element) {
		tip.Padding(t.Space6, t.Space8).Radius(t.RadiusControl).MaxWidth(t.SizeMenubarPanelWidth).
			Background(ui.Hex(t.ColorTooltip)).TextColor(ui.Hex(t.ColorTooltipText)).Children(func() {
			ui.Text(c, reason).FontSize(t.FontSizeSmall).LineHeight(t.LineHeightBody)
		})
	})
}

// statusDot is a neutral 6 × 6 mark: solid for on, muted for off. No green.
func (a *application) statusDot(c *ui.Context, t tokenValues, on bool, tip string) {
	color := t.ColorTextDisabled
	if on {
		color = t.ColorSuccess
	}
	anchor := ui.Row(c).Height(t.SizeIcon).AlignItems(ui.Center).Focusable().Label(tip).Description(tip)
	ui.TooltipBase(c, anchor, func(bubble ui.Element) {
		bubble.Padding(t.Space6, t.Space8).Radius(t.RadiusControl).MaxWidth(t.SizeMenubarPanelWidth).
			Background(ui.Hex(t.ColorTooltip)).TextColor(ui.Hex(t.ColorTooltipText)).Children(func() {
			ui.Text(c, tip).FontSize(t.FontSizeSmall)
		})
	})
	anchor.Children(func() {
		ui.Box(c).Size(t.Space6, t.Space6).Radius(t.Space6 / 2).Background(ui.Hex(color))
	})
}

// pageHeader is the card at the top of every settings page: icon, name, one
// line of description, and the page's own action on the right.
func (a *application) pageHeader(c *ui.Context, t tokenValues, locale, glyph, titleKey, descKey, alert string, right func()) {
	text := func(key string) string { return a.text(locale, key) }
	ui.Row(c).FillWidth().Height(t.SizePageHeader).Padding(t.Space8, t.Space16).Gap(t.Space12).AlignItems(ui.Center).
		Radius(t.RadiusCard).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).Background(ui.Hex(t.ColorCard)).Children(func() {
		ui.Row(c).Size(t.SizeIconButton, t.SizeIconButton).Shrink(0).Radius(t.RadiusControl).
			Background(ui.Hex(t.ColorControl)).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon(glyph)).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space2).Children(func() {
			ui.Text(c, text(titleKey)).FontSize(t.FontSizeBody).FontWeight(fontWeight(t.FontWeightSemibold)).
				LineHeight(t.LineHeightBody).SingleLine()
			ui.Text(c, text(descKey)).FontSize(t.FontSizeSmall).LineHeight(t.LineHeightBody).
				TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
		})
		if alert != "" {
			a.alertIcon(c, t, alert)
		}
		if right != nil {
			right()
		}
	})
}

func (a *application) groupTitle(c *ui.Context, t tokenValues, label string) {
	ui.Text(c, label).Height(t.SizeGroupHeader).FontSize(t.FontSizeSmall).FontWeight(fontWeight(t.FontWeightMedium)).
		LineHeight(t.LineHeightBody).TextColor(ui.Hex(t.ColorTextSecondary))
}

// settingsSection draws one group title and its card. The Space8 before the
// title is the only gap between two groups.
func (a *application) settingsSection(c *ui.Context, t tokenValues, label string, card func()) {
	ui.Box(c).Height(t.Space8)
	a.groupTitle(c, t, label)
	ui.Box(c).Height(t.Space8)
	ui.Column(c).FillWidth().PaddingX(t.Space16).Radius(t.RadiusCard).
		Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).Background(ui.Hex(t.ColorCard)).Children(card)
}

// settingsAlert combines the page's failures into one description. A recovery
// or stop error comes first, and a save failure is never dropped.
func (a *application) settingsAlert(locale string, page int) string {
	text := func(key string) string { return a.text(locale, key) }
	var keys []string
	add := func(key string) {
		if key == "" || slices.Contains(keys, key) {
			return
		}
		keys = append(keys, key)
	}
	if page == settingsAwake {
		add(a.moduleError)
		if a.lid != nil {
			// Recovery and helper failures belong to the module that owns the
			// lid, not to the general page. An idle or working state keeps no
			// stale reason on screen.
			snapshot := a.lid.snapshot()
			switch snapshot.state {
			case lidRestoreFailed:
				add("LidRestoreFailed")
			case lidHelperGone, lidError, lidBlocked, lidStopping:
				add(snapshot.reason)
			}
		}
		add(a.awake.snapshot().Error)
	}
	if page == settingsGeneral && a.settingsError != "ConfigError" {
		// CLI link failures and login errors belong to the general page only.
		add(a.settingsError)
	}
	if a.settingsError == "ConfigError" {
		add("ConfigError")
	}
	var parts []string
	for _, key := range keys {
		message := text(key)
		if key == "CLILinkConflict" {
			if paths, ok := cliPathsFor(); ok {
				message = strings.ReplaceAll(message, "%s", paths.link)
			} else {
				message = text("CLIInstallFailed")
			}
		}
		if message == "" || slices.Contains(parts, message) {
			continue
		}
		parts = append(parts, message)
	}
	return strings.Join(parts, " · ")
}

// settingsContent builds the page named by the layout.
func (a *application) settingsContent(c *ui.Context, t tokenValues, locale string, l settingsLayout) {
	alert := a.settingsAlert(locale, l.page)
	switch l.page {
	case settingsGeneral:
		a.generalPage(c, t, locale, l, alert)
	case settingsAbout:
		a.aboutPage(c, t, locale, l, alert)
	case settingsAwake:
		a.awakeModulePage(c, t, locale, l, alert)
	case settingsJSON:
		a.jsonModulePage(c, t, locale, l, alert)
	default:
		a.shortcutsPage(c, t, locale, l, alert)
	}
}

func (a *application) generalPage(c *ui.Context, t tokenValues, locale string, l settingsLayout, alert string) {
	text := func(key string) string { return a.text(locale, key) }
	a.pageHeader(c, t, locale, "settings-2", "General", "GeneralPageDesc", alert, nil)
	a.settingsSection(c, t, text("GroupPreferences"), func() {
		a.settingsRow(c, t, text("Language"), settingsRowOpts{level: 0}, func() {
			parts := ui.SegmentedBase(c.Key("language"), &a.language, 3)
			parts.Track.Size(t.SizeLanguageSelectorWidth, t.SizeSegmentHeight).Padding(t.Space2).Radius(t.RadiusControl).
				Background(ui.Hex(t.ColorControl)).Label(text("Language")).OnChange(a.changeLanguage).Children(func() {
				for index, label := range []string{text("System"), text("Chinese"), text("English")} {
					color, background := t.ColorText, t.ColorTransparent
					if index == a.language {
						color, background = t.ColorOnAccent, t.ColorAccent
					}
					parts.Segment(index).Grow(1).FillHeight().Radius(t.RadiusControl).Background(ui.Hex(background)).
						FocusRing(true).Children(func() {
						ui.Text(c, label).FontSize(t.FontSizeBody).TextColor(ui.Hex(color))
					})
				}
			})
		})
		ui.Divider(c)
		a.settingsRow(c, t, text("LaunchAtLogin"), settingsRowOpts{level: 1, hint: text("LaunchHint")}, func() {
			a.switchControl(c, t, "login", text("LaunchAtLogin"), &a.config.OpenAtLogin, a.changeLogin, "", false)
		})
		if l.hasCLI {
			ui.Divider(c)
			a.cliSettingsRow(c, t, locale, l)
		}
		if l.showPlacement {
			ui.Divider(c)
			a.windowPlacementRow(c, t, locale)
			if l.fixedDisplay {
				ui.Divider(c)
				a.windowDisplayRow(c, t, locale, l)
			}
		}
	})
}

// windowPlacementRow chooses how the window picks its display. Neither choice
// moves the window that is already open: the change applies the next time it
// opens.
func (a *application) windowPlacementRow(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	modes := []string{"cursor", "display"}
	keys := []string{"WindowPlacementCursor", "WindowPlacementDisplay"}
	index := 0
	if a.config.WindowPlacement == "display" {
		index = 1
	}
	a.settingsRow(c, t, text("WindowPlacement"), settingsRowOpts{level: 3, tooltip: text("WindowPlacementInfo")}, func() {
		// The value lives in this frame's index; the callback writes the mode
		// back through changeWindowPlacement.
		parts := ui.SegmentedBase(c.Key("window-placement"), &index, len(modes))
		parts.Track.Size(t.SizeLanguageSelectorWidth, t.SizeSegmentHeight).Padding(t.Space2).Radius(t.RadiusControl).
			Background(ui.Hex(t.ColorControl)).Label(text("WindowPlacement")).
			OnChange(func() { a.changeWindowPlacement(modes[index]) }).Children(func() {
			for segment, key := range keys {
				color, background := t.ColorText, t.ColorTransparent
				if segment == index {
					color, background = t.ColorOnAccent, t.ColorAccent
				}
				parts.Segment(segment).Grow(1).FillHeight().Radius(t.RadiusControl).
					Background(ui.Hex(background)).FocusRing(true).Label(text(key)).Children(func() {
					ui.Text(c, text(key)).FontSize(t.FontSizeBody).TextColor(ui.Hex(color)).SingleLine()
				})
			}
		})
	})
}

// maxDisplayMenuRows bounds the display menu: more rows scroll instead of
// growing past the window.
const maxDisplayMenuRows = 6

// windowDisplayRow chooses the fixed display. A choice that can no longer be
// identified keeps the fallback on screen next to an alert: the stored choice
// is read again only when the user picks a display.
func (a *application) windowDisplayRow(c *ui.Context, t tokenValues, locale string, l settingsLayout) {
	text := func(key string) string { return a.text(locale, key) }
	choices := displayChoices(l.displays)
	current := ""
	for _, choice := range choices {
		if choice.Value == strconv.FormatInt(l.display.ID, 10) {
			current = choice.Label
		}
	}
	if current == "" && len(choices) > 0 {
		current = choices[0].Label
	}
	a.settingsRow(c, t, text("WindowDisplay"), settingsRowOpts{level: 0}, func() {
		if l.displayMissing {
			a.alertIcon(c, t, text("WindowDisplayMissing"))
		}
		// Bind the persisted ID so explicitly choosing its resolved fallback also
		// produces OnChange for keyboard activation.
		selected := seatKey(a.config.WindowDisplayID)
		// A changed snapshot gets a fresh select state, closing any popup whose
		// highlight and option IDs described the previous topology.
		sel := ui.SelectBase(c.Key(fmt.Sprintf("window-display-%d", a.displayRevision)), &selected)
		sel.Trigger.Width(t.SizeLanguageSelectorWidth).Height(t.SizeSegmentHeight).PaddingX(t.Space8).
			Radius(t.RadiusControl).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).
			Background(ui.Hex(t.ColorControl)).FocusRing(true).
			Label(text("WindowDisplay") + ": " + current).Tooltip(current).
			OnChange(func() { a.commitWindowDisplayID(selected) }).Children(func() {
			ui.Text(c, current).FontSize(t.FontSizeBody).SingleLine().Grow(1).MinWidth(0)
			ui.Icon(c, icon("chevron-down")).Size(t.SizeIcon, t.SizeIcon).
				TextColor(ui.Hex(t.ColorTextSecondary)).Shrink(0)
		})
		highlighted := false
		sel.Popup(func(panel ui.Element) {
			panel.Padding(t.Space4).Radius(t.RadiusCard).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).
				Background(ui.Hex(t.ColorCard))
			listHeight := float32(min(len(choices), maxDisplayMenuRows)) * t.SizeRowHeight
			if _, windowHeight := c.Size(); windowHeight > 0 {
				listHeight = min(listHeight, max(t.SizeRowHeight, windowHeight-2*t.Space24))
			}
			ui.Scroll(c).Width(t.SizeLanguageSelectorWidth).Height(listHeight).Children(func() {
				for _, choice := range choices {
					item := sel.Item(choice.Value).Height(t.SizeRowHeight).PaddingX(t.Space8).
						Radius(t.RadiusControl).Label(choice.Title)
					if item.Highlighted() {
						highlighted = true
						item.Background(ui.Hex(t.ColorAccentSoft))
					}
					// Explicit same-value selection is still a user choice.
					item.OnClick(func() { a.commitWindowDisplayID(choice.Value) })
					item.Children(func() {
						ui.Text(c, choice.Label).FontSize(t.FontSizeBody).SingleLine().Tooltip(choice.Title)
					})
				}
			})
		})
		if sel.Open() && !highlighted {
			// The stored ID may be offline or resolved by name. Start keyboard
			// navigation on the display actually shown by the trigger.
			sel.Highlight(seatKey(l.display.ID))
		}
	})
}

func (a *application) awakeModulePage(c *ui.Context, t tokenValues, locale string, l settingsLayout, alert string) {
	text := func(key string) string { return a.text(locale, key) }
	on := a.config.AwakeEnabled
	busy := a.moduleBusy()
	a.pageHeader(c, t, locale, "sun", "Awake", "AwakePageDesc", alert, func() {
		if busy {
			ui.Spinner(c).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary)).Label(text("Awake"))
		}
		sw := a.switchControl(c, t, "awake-enabled", text("Awake"), &on, func() {
			a.changeAwakeEnabled(!a.config.AwakeEnabled)
		}, "", busy)
		if a.focusModuleSwitch {
			// Recover a lost focus once. A user who moved to the sidebar or
			// another control during the async close keeps that focus.
			a.focusModuleSwitch = false
			if !c.Root().FocusWithin() {
				sw.Focus()
			}
		}
	})
	// A closed module hides its own settings, but never its shortcut: the row
	// stays visible and disabled, so the release is a fact the user can see.
	// While a close runs, the committed state still draws; its controls are
	// disabled until the transaction has settled.
	if on {
		a.settingsSection(c, t, text("GroupSettings"), func() {
			a.lidBatteryRow(c, t, locale, busy)
		})
	}
	a.settingsSection(c, t, text("GroupShortcuts"), func() {
		a.shortcutSettingsRow(c, t, locale, "awake")
	})
}

func (a *application) jsonModulePage(c *ui.Context, t tokenValues, locale string, l settingsLayout, alert string) {
	text := func(key string) string { return a.text(locale, key) }
	on := a.config.JSONEnabled
	busy := a.moduleBusy()
	a.pageHeader(c, t, locale, "braces", "JSON", "JSONPageDesc", alert, func() {
		if busy {
			ui.Spinner(c).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary)).Label(text("JSON"))
		}
		a.switchControl(c, t, "json-enabled", text("JSON"), &on, func() { a.changeJSONEnabled(!a.config.JSONEnabled) }, "", busy)
	})
	a.settingsSection(c, t, text("GroupShortcuts"), func() {
		a.shortcutSettingsRow(c, t, locale, "json")
	})
}

func (a *application) shortcutsPage(c *ui.Context, t tokenValues, locale string, l settingsLayout, alert string) {
	text := func(key string) string { return a.text(locale, key) }
	a.pageHeader(c, t, locale, "keyboard", "R05Shortcuts", "ShortcutsPageDesc", alert, nil)
	a.settingsSection(c, t, "Cubbix", func() {
		a.shortcutSettingsRow(c, t, locale, "palette")
		ui.Divider(c)
		a.shortcutSettingsRow(c, t, locale, "panel")
	})
	a.settingsSection(c, t, text("FeedbackGroup"), func() {
		a.feedbackModeRow(c, t, locale)
		ui.Divider(c)
		a.feedbackPositionRow(c, t, locale)
	})
}

func (a *application) aboutPage(c *ui.Context, t tokenValues, locale string, l settingsLayout, alert string) {
	text := func(key string) string { return a.text(locale, key) }
	update := a.updates.snapshot()
	a.pageHeader(c, t, locale, "info", "About", "AboutPageDesc", alert, func() {
		check := ui.ButtonBase(c.Key("check")).Height(t.SizeButtonHeight).PaddingX(t.Space8).Radius(t.RadiusControl).
			FocusRing(true).Label(text("CheckAction")).Tooltip(text("CheckAction")).
			Disabled(update.Phase == updateChecking || update.Phase == updateDownloading || update.Phase == updateReady).
			OnClick(func() { a.updates.request(true) })
		check.Children(func() {
			ui.Text(c, text("CheckAction")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorText))
		})
	})
	a.settingsSection(c, t, text("UpdateGroup"), func() {
		a.settingsRow(c, t, text("Version"), settingsRowOpts{level: 0}, func() {
			ui.Text(c, version()).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorTextSecondary))
			a.updateStatus(c, t, update, text)
		})
		ui.Divider(c)
		ui.ButtonBase(c.Key("notes")).FillWidth().Height(t.SizeRowHeight).FocusRing(true).OnClick(a.showNotes).Children(func() {
			ui.Row(c).Fill().AlignItems(ui.Center).Gap(t.Space8).Children(func() {
				ui.Text(c, text("WhatsNew")).FontSize(t.FontSizeBody).Grow(1)
				ui.Icon(c, icon("chevron-right")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary)).Tooltip(text("WhatsNew"))
				if update.NotesError != "" {
					a.alertIcon(c, t, text(update.NotesError))
				}
			})
		})
		ui.Divider(c)
		hint := a.updateScheduleHint(locale)
		a.settingsRow(c, t, text("AutomaticUpdates"), settingsRowOpts{icon: "refresh-cw", level: 2, hint: hint}, func() {
			a.switchControl(c, t, "auto", text("AutomaticUpdates"), &a.config.AutomaticUpdates, a.changeAutomaticUpdates, hint, false)
		})
	})
	if l.ready {
		ui.Box(c).Height(t.Space8)
		a.readyCard(c, t, text)
	}
}

// updateScheduleHint is the second line of the automatic update row: when
// checks happen, and when the last one finished if that is known.
func (a *application) updateScheduleHint(locale string) string {
	text := func(key string) string { return a.text(locale, key) }
	hint := text("UpdateSchedule")
	if !a.config.AutomaticUpdates {
		hint = text("UpdateScheduleOff")
	}
	checked := a.updates.lastCheck()
	if checked.IsZero() {
		return hint
	}
	now := a.updates.nowTime()
	if checked.After(now) {
		// A clock that went backwards hides the record without rewriting it.
		return hint
	}
	return hint + " · " + fmt.Sprintf(text("UpdateLastCheck"), a.checkedAtText(locale, checked, now))
}

// checkedAtText reads a completion time in local calendar days: today is just
// the clock, yesterday names itself, older ones add the date, and another year
// adds the year. yesterday uses AddDate so a daylight saving change cannot
// shift the day.
func (a *application) checkedAtText(locale string, checked, now time.Time) string {
	checked = checked.In(now.Location())
	year, month, day := now.Date()
	checkedYear, checkedMonth, checkedDay := checked.Date()
	yesterday := now.AddDate(0, 0, -1)
	if checkedYear == year && checkedMonth == month && checkedDay == day {
		return checked.Format("15:04")
	}
	if y, m, d := yesterday.Date(); checkedYear == y && checkedMonth == m && checkedDay == d {
		return fmt.Sprintf(a.text(locale, "UpdateYesterday"), checked.Format("15:04"))
	}
	if checkedYear == year {
		if locale == "en" {
			return checked.Format("Jan 2, 15:04")
		}
		return checked.Format("1月2日 15:04")
	}
	if locale == "en" {
		return checked.Format("Jan 2, 2006, 15:04")
	}
	return checked.Format("2006年1月2日 15:04")
}

// requestModuleFocus asks for the module switch once. A closed module removes
// the group that held the focus, so the switch has to take it back.
func (a *application) requestModuleFocus() { a.focusModuleSwitch = a.page == settingsAwake }

// updateIndicator is the version row's right side: a failure alert first, then
// a loader, then the neutral dot that means "up to date".
func (a *application) updateStatus(c *ui.Context, t tokenValues, update updateState, text func(string) string) {
	switch {
	case update.ErrorKey != "":
		a.alertIcon(c, t, text(update.ErrorKey))
	case update.Phase == updateChecking || update.Phase == updateDownloading:
		key := "TooltipChecking"
		if update.Phase == updateDownloading {
			key = "UpdateDownloading"
		}
		ui.Spinner(c).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary)).Tooltip(text(key)).Label(text(key))
	case update.Phase == updateCurrent:
		a.statusDot(c, t, true, text("TooltipSuccess"))
		ui.Text(c, text("TooltipSuccess")).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
	}
}

// sidebar draws the brand block and the five page entries. The brand mark sits
// in the same 28-wide column as the navigation icons, and one hairline
// separates the two blocks.
func (a *application) sidebar(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	ui.Column(c).Width(t.SizeSidebarWidth).FillHeight().Padding(t.Space12).Background(ui.Hex(t.ColorSidebar)).DragWindow().Children(func() {
		ui.Box(c).Height(t.Space12)
		ui.Box(c).Height(t.Space16)
		ui.Row(c).Height(t.Space20).PaddingX(t.Space8).AlignItems(ui.Center).Gap(t.Space8).Children(func() {
			ui.Row(c).Width(t.SizeIconButton).Height(t.Space20).Shrink(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, brand(t.ColorText)).Size(t.SizeBrandMark, t.SizeBrandMark).TextColor(ui.Hex(t.ColorText))
			})
			ui.Text(c, "Cubbix").FontSize(t.FontSizeBody).FontWeight(fontWeight(t.FontWeightSemibold)).LineHeight(t.LineHeightBody)
		})
		ui.Box(c).Height(t.Space16)
		ui.Row(c).FillWidth().PaddingX(t.Space8).Children(func() {
			ui.Box(c).FillWidth().Height(t.StrokeHairline).Background(ui.Hex(t.ColorSeparator))
		})
		ui.Box(c).Height(t.Space16)
		ui.Column(c).FillWidth().Gap(t.Space4).Children(func() {
			for _, item := range settingsNav {
				entry := item
				selected := entry.page == a.page && !a.notesPage
				background := ui.Hex(t.ColorTransparent)
				if selected {
					background = ui.Hex(t.ColorAccentSoft)
				}
				button := ui.ButtonBase(c.Key("nav-" + entry.key)).Height(t.SizeNavItem).FillWidth().Radius(t.RadiusCard).
					FocusRing(true).Transition(ui.ElementTransition{Duration: 140 * time.Millisecond, Colors: true})
				if !selected && button.Hovered() {
					background = ui.Hex(t.ColorNavHover)
				}
				button.Background(background).Label(text(entry.key)).OnClick(func() {
					a.selectSettingsPage(entry.page)
				}).Children(func() {
					ui.Row(c).Fill().AlignItems(ui.Center).PaddingX(t.Space8).Gap(t.Space8).Children(func() {
						ui.Row(c).Size(t.SizeIconButton, t.SizeIconButton).Radius(t.RadiusControl).
							Background(ui.Hex(t.ColorControl)).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
							ui.Icon(c, icon(entry.glyph)).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
						})
						// Keep the 12 pt name whole: "Keep awake" measures
						// half a point wider than the space left by the 28 icon
						// cell and the Space8 gaps, and a truncation there would
						// hide the module's name.
						ui.Text(c, text(entry.key)).FontSize(t.FontSizeSmall).FontWeight(fontWeight(t.FontWeightMedium)).SingleLine().Shrink(0)
					})
				})
			}
		})
	})
}

// selectSettingsPage is the one page switch: the sidebar, a URL, the command
// line and the palette all end here, and every one of them ends a recording
// first.
func (a *application) selectSettingsPage(page int) {
	a.cancelShortcutRecording()
	a.focusModuleSwitch = false
	a.page, a.notesPage = page, false
	a.invalidateWindow()
}

// settingsView is the settings column for the page on screen.
func (a *application) settingsView(c *ui.Context, t tokenValues, locale string) {
	l := newSettingsLayout(a, a.page)
	a.settingsContent(c, t, locale, l)
}

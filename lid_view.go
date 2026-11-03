package main

import (
	"github.com/egoist/mygo/ui"
)

func (a *application) panelLid(c *ui.Context, t tokenValues, locale string) {
	text := func(k string) string { return a.text(locale, k) }
	s := a.lid.snapshot()
	on := s.state == lidOn
	tip := text("R05Lid")
	if key := a.lidReason(); key != "" {
		tip = text(key)
	} else if !on && s.service == svcNotRegistered {
		tip = text("LidInstallNeeded")
	}
	ui.Row(c).FillWidth().Height(t.SizeNavHeight).AlignItems(ui.Center).Gap(t.Space8).Children(func() {
		ui.Icon(c, icon("laptop")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary)).Tooltip(tip)
		ui.Box(c).Grow(1)
		if a.lidBusy || s.state == lidEnabling || s.state == lidStopping {
			ui.Icon(c, icon("loader-circle")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary)).Label(text("R05Lid"))
			return
		}
		background, thumb, align := t.ColorControl, t.ColorTextSecondary, ui.Start
		if on {
			background, thumb, align = t.ColorAccent, t.ColorThumb, ui.End
		}
		ui.SwitchBase(c.Key("panel-lid"), &on).Size(t.SizeSwitchWidth, t.SizeSwitchHeight).Radius(t.RadiusControl).Background(ui.Hex(background)).Padding(t.Space2).Justify(align).AlignItems(ui.Center).Label(text("R05Lid")).Tooltip(tip).Disabled(!a.lid.Available() || s.reason == "LidForeign" || s.reason == "LidBatteryTooLow").FocusRing(true).OnChange(a.toggleLid).Children(func() {
			ui.Box(c).Size(t.SizeSwitchThumb, t.SizeSwitchThumb).Radius(t.SizeSwitchThumb / 2).Background(ui.Hex(thumb))
		})
	})
}
func (a *application) lidRecoveryControls(c *ui.Context, t tokenValues, locale string) {
	text := func(k string) string { return a.text(locale, k) }
	if a.lid == nil {
		return
	}
	if a.lid.canForce() {
		ui.ButtonBase(c.Key("lid-force-restore")).Height(t.SizeButtonHeight).PaddingX(t.Space8).Radius(t.RadiusControl).FocusRing(true).Label(text("LidForceRestore")).OnClick(a.forceLid).Children(func() {
			ui.Text(c, text("LidForceRestore")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorDanger))
		})
		ui.Text(c, text("LidForceRestoreHint")).FillWidth().FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorText)).MaxWidth(t.SizeMenubarPanelWidth - 2*t.Space12 - 2*t.Space8)
	}
	if a.lid.snapshot().service == svcRequiresApproval {
		ui.ButtonBase(c.Key("lid-settings")).Height(t.SizeButtonHeight).PaddingX(t.Space8).FocusRing(true).Label(text("R05LidAllow")).OnClick(a.lid.service.OpenSettings).Children(func() {
			ui.Text(c, text("R05LidAllow")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorText))
		})
	}
}
func (a *application) lidBatteryRow(c *ui.Context, t tokenValues, locale string) {
	value := a.config.AwakeLidBatteryMin
	index := 1
	for i, n := range []int{10, 20, 30} {
		if n == value {
			index = i
		}
	}
	a.row(c, t, a.text(locale, "LidBatteryLimit"), func() {
		parts := ui.SegmentedBase(c.Key("lid-battery-limit"), &index, 3)
		parts.Track.Size(t.SizeLanguageSelectorWidth, t.SizeSegmentHeight).Padding(t.Space2).Radius(t.RadiusControl).Background(ui.Hex(t.ColorControl)).Label(a.text(locale, "LidBatteryLimit")).OnChange(func() { a.config.AwakeLidBatteryMin = []int{10, 20, 30}[index]; a.save() }).Children(func() {
			for i, label := range []string{"10%", "20%", "30%"} {
				bg, color := t.ColorTransparent, t.ColorText
				if i == index {
					bg, color = t.ColorAccent, t.ColorOnAccent
				}
				parts.Segment(i).Grow(1).FillHeight().Radius(t.RadiusControl).Background(ui.Hex(bg)).FocusRing(true).Label(label).Children(func() { ui.Text(c, label).FontSize(t.FontSizeBody).TextColor(ui.Hex(color)) })
			}
		})
	})
}

// Tooltips are pointer-transparent in MyGo. Use an explicit popover for
// recovery controls, so both the pointer and keyboard can operate them.
func (a *application) panelLidAlert(c *ui.Context, t tokenValues, locale, label string) {
	if a.panel == nil {
		return
	}
	alert := ui.ButtonBase(c.Key("lid-alert")).Size(t.SizeIcon, t.SizeIcon).Label(label).Description(label).FocusRing(true).OnClick(func() { a.panel.lidDetails = !a.panel.lidDetails; a.refreshMenubarPanel() })
	alert.Children(func() {
		ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger))
	})
	ui.PopoverBase(c.Key("lid-recovery-popover"), alert, &a.panel.lidDetails, func(tip ui.Element) {
		tip.Width(t.SizeMenubarPanelWidth-2*t.Space12).Padding(t.Space6, t.Space8).Radius(t.RadiusControl).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).Background(ui.Hex(t.ColorCard)).TextColor(ui.Hex(t.ColorText)).Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space8).Children(func() {
				ui.Text(c, label).FillWidth().FontSize(t.FontSizeSmall)
				a.lidRecoveryControls(c, t, locale)
			})
		})
	})
}

package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// buildFeedbackWindow creates the hint panel on the first trigger. It is a
// nonactivating panel: showing it never activates the app, takes the keyboard
// or switches Space, and the pointer goes through it.
func (a *application) buildFeedbackWindow(p *feedbackPresenter) {
	t := tokens(false, "zh-CN")
	window := mygo.NewWindow(mygo.WindowOptions{
		Title:             "Cubbix",
		Width:             int(t.SizeToastSlotWidth),
		Height:            int(t.SizeToastHeight),
		Hidden:            true,
		Frameless:         true,
		Transparent:       true,
		DisableShadow:     true,
		DisableResize:     true,
		DisableMove:       true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		DisableClose:      true,
		Panel:             true,
		Content:           ui.View(a.feedbackView),
	})
	window.SetIgnoreMouseEvents(true)
	window.SetHasShadow(false)
	window.SetVisibleOnAllWorkspaces(true)
	p.window = window
}

// feedbackView draws the fixed-size slot and, while showing, the capsule
// inside it. The capsule is the only element that enters and leaves; the slot
// stays so the exit animation has a parent that is still built.
func (a *application) feedbackView(c *ui.Context) {
	_, t := applyTheme(c)
	p := a.feedback
	if p == nil {
		return
	}
	c.Root().Background(ui.Transparent)
	align := ui.Start
	entryY := -t.Space4
	switch p.corner() {
	case "top-right":
		align = ui.End
	case "bottom-right":
		align, entryY = ui.End, t.Space4
	case "center":
		align, entryY = ui.Center, 0
	case "bottom-left":
		entryY = t.Space4
	}
	ui.Column(c).FillWidth().Height(t.SizeToastHeight).AlignItems(align).Justify(ui.Center).Children(func() {
		if !p.showing {
			return
		}
		message := p.message
		iconColor := t.ColorText
		if message.Danger {
			iconColor = t.ColorDanger
		} else if message.Muted {
			iconColor = t.ColorTextSecondary
		}
		ui.Row(c).Key("feedback-toast").Height(t.SizeToastHeight).MaxWidth(t.SizeToastSlotWidth).PaddingX(t.Space12).Gap(t.Space8).Radius(t.RadiusWindow).AlignItems(ui.Center).Background(ui.Hex(t.ColorMenuSurface)).Transition(ui.ElementTransition{
			Duration: feedbackEnterDuration,
			Ease:     ui.EaseOut,
			Enter:    &ui.Motion{Y: entryY, Scale: 0.96},
			Exit:     &ui.Motion{},
		}).Children(func() {
			if message.Icon != "" {
				ui.Icon(c, icon(message.Icon)).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(iconColor)).ContentTransition(ui.ContentTransition{})
			}
			// One line: a longer process name is cut, never wrapped, so the
			// slot height and the position stay fixed.
			ui.Text(c, message.Text).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorText)).SingleLine().Ellipsis("…").ContentTransition(ui.ContentTransition{}).MaxWidth(t.SizeToastSlotWidth - 2*t.Space12 - t.Space8 - t.SizeIcon)
		})
	})
}

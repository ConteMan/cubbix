package main

import (
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func (a *application) changeAutomaticUpdates() {
	a.save()
	a.updates.setAutomatic(a.config.AutomaticUpdates)
}

func (a *application) restartUpdate() {
	if a.updates.snapshot().Phase == updateReady && a.relaunch != nil {
		// M0 has no documents or awake sessions. Future tiles must register
		// quit guards and persist their sessions before Relaunch is allowed.
		a.relaunch()
	}
}

func (a *application) updateIndicator(c *ui.Context, t tokenValues, update updateState, text func(string) string) {
	switch {
	case update.ErrorKey != "":
		ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).Tooltip(text(update.ErrorKey))
	case update.Phase == updateChecking || update.Phase == updateDownloading:
		key := "TooltipChecking"
		if update.Phase == updateDownloading {
			key = "UpdateDownloading"
		}
		ui.Spinner(c).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary)).Tooltip(text(key))
	case update.Phase == updateCurrent:
		ui.Icon(c, icon("check")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorText)).Tooltip(text("TooltipSuccess"))
	}
}

func (a *application) readyCard(c *ui.Context, t tokenValues, text func(string) string) {
	ui.Row(c).FillWidth().Height(t.SizeRowHeight+t.Space24).Padding(t.Space16).Gap(t.Space12).AlignItems(ui.Center).Radius(t.RadiusCard).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).Background(ui.Hex(t.ColorCard)).Children(func() {
		ui.Icon(c, icon("download")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
		ui.Column(c).Grow(1).Gap(t.Space4).Children(func() {
			ui.Text(c, text("DownloadReady")).FontSize(t.FontSizeBody)
			ui.Text(c, text("DownloadHint")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
		})
		ui.ButtonBase(c.Key("restart-update")).Height(t.SizeButtonHeight).PaddingX(t.Space12).Radius(t.RadiusControl).Background(ui.Hex(t.ColorAccent)).FocusRing(true).Label(text("RestartUpdate")).OnClick(a.restartUpdate).Children(func() {
			ui.Text(c, text("RestartUpdate")).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorOnAccent))
		})
	})
}

func (a *application) showNotes() {
	if a.notesWindow == nil {
		t := tokens(false, "zh-CN")
		a.notesWindow = mygo.NewWindow(mygo.WindowOptions{Title: a.text(mygo.App.Locale(), "WhatsNew"), Width: int(t.SizeWindowWidth), Height: int(t.SizeWindowHeightAbout), Frameless: true, DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true, Content: ui.View(a.notesView)})
		a.notesWindow.OnClose(func(event *mygo.CloseEvent) {
			if !a.quitting {
				event.PreventDefault()
				a.notesWindow.Hide()
			}
		})
	}
	a.notesWindow.Show()
	a.notesWindow.Focus()
	a.updates.loadNotes()
}

func (a *application) notesView(c *ui.Context) {
	locale, t := applyTheme(c)
	text := func(key string) string { return a.text(locale, key) }
	closeNotes := func() {
		if a.notesWindow != nil {
			a.notesWindow.Hide()
		}
	}
	c.OnShortcut(ui.Cmd, ui.KeyW, closeNotes)
	c.OnShortcut(0, ui.KeyEscape, closeNotes)
	c.OnShortcut(ui.Cmd, ui.KeyQ, func() { mygo.App.Quit() })
	state := a.updates.snapshot()
	ui.Column(c).Fill().Padding(t.Space24).Gap(t.Space16).Background(ui.Hex(t.ColorWindow)).Children(func() {
		ui.Row(c).FillWidth().Height(t.SizeButtonHeight).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Text(c, text("WhatsNew")).FontSize(t.FontSizeBody).Grow(1)
			if state.NotesLoading {
				ui.Spinner(c).Size(t.SizeIcon, t.SizeIcon).Tooltip(text("NotesLoading"))
			}
			if state.NotesError != "" {
				ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).Tooltip(text(state.NotesError))
			}
			ui.ButtonBase(c.Key("close-notes")).Size(t.SizeIconButton, t.SizeIconButton).Label(text("CloseNotes")).Tooltip(text("CloseNotes")).OnClick(closeNotes).Children(func() {
				ui.Icon(c, icon("x")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
			})
		})
		ui.Scroll(c).FillWidth().Grow(1).Children(func() {
			if state.Notes != "" {
				markdown.View(c, state.Notes, nil).FillWidth()
			} else if !state.NotesLoading && state.NotesError == "" {
				ui.Text(c, text("NotesNoNewer")).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorTextSecondary))
			}
		})
	})
}

// QA fixtures exercise native layouts and menus without a published release
// or replacing the installed app. They are enabled only by explicit QA mode.
func (a *application) qaUpdateState() bool {
	switch os.Getenv("CUBBIX_QA_UPDATE") {
	case "ready":
		a.updates.state = updateState{Phase: updateReady, Version: "0.0.2"}
	case "network":
		a.updates.state.ErrorKey = "UpdateNetworkError"
	case "signature":
		a.updates.state.ErrorKey = "UpdateSignatureError"
	default:
		return false
	}
	a.updates.notify()
	return true
}

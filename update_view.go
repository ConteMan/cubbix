package main

import (
	"os"

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
	a.page, a.notesPage = 1, true
	a.invalidateWindow()
	a.updates.loadNotes()
}

func (a *application) backNotes() {
	a.notesPage = false
	a.invalidateWindow()
}

func (a *application) notesView(c *ui.Context) {
	locale, t := applyTheme(c)
	text := func(key string) string { return a.text(locale, key) }
	state := a.updates.snapshot()
	ui.Row(c).FillWidth().Height(t.SizeIconButton).Gap(t.Space8).AlignItems(ui.Center).Children(func() {
		ui.ButtonBase(c.Key("back-notes")).Size(t.SizeIconButton, t.SizeIconButton).Radius(t.RadiusControl).FocusRing(true).Label(text("BackNotes")).Tooltip(text("BackNotes")).OnClick(a.backNotes).Children(func() {
			ui.Icon(c, icon("chevron-left")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
		})
		ui.Text(c, text("WhatsNew")).FontSize(t.FontSizeBody).Grow(1)
		if state.NotesLoading {
			ui.Spinner(c).Size(t.SizeIcon, t.SizeIcon).Tooltip(text("NotesLoading"))
		}
		if state.NotesError != "" {
			ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).Tooltip(text(state.NotesError))
		}
	})
	ui.Scroll(c.Key("release-history")).FillWidth().Grow(1).Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space24).Children(func() {
			for _, entry := range releaseHistory(locale, state.Notes) {
				ui.Column(c.Key(entry.Version)).FillWidth().Gap(t.Space8).Children(func() {
					ui.Row(c).FillWidth().Gap(t.Space12).AlignItems(ui.Center).Children(func() {
						ui.Text(c, entry.Version).FontSize(t.FontSizeBody).Bold()
						ui.Text(c, entry.Date).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
						if entry.Version == runningVersion() {
							ui.Text(c, text("CurrentVersion")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
						}
					})
					markdown.View(c, entry.Notes, nil).FillWidth()
				})
			}
		})
	})
}

// QA fixtures exercise native layouts and menus without a published release
// or replacing the installed app. They are enabled only by explicit QA mode.
func (a *application) qaUpdateState() bool {
	switch os.Getenv("CUBBIX_QA_UPDATE") {
	case "ready":
		a.updates.state = updateState{Phase: updateReady, Version: "0.0.4"}
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

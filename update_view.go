package main

import (
	"log"
	"os"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// syncUpdateCheck persists the completion time of the last check. The manager
// records it on a worker; only the main thread writes configuration, and only
// when the stored value really differs. A save failure keeps the in-memory time
// and shows ConfigError on the window, so the header never claims a restart
// would find it.
func (a *application) syncUpdateCheck() {
	checked := a.updates.lastCheck()
	if checked.IsZero() {
		return
	}
	value := checked.UTC().Format(time.RFC3339Nano)
	if a.config.UpdateLastCheck == value {
		return
	}
	a.config.UpdateLastCheck = value
	if err := saveConfig(a.path, a.config); err != nil {
		a.settingsError = "ConfigError"
		log.Print(err)
		return
	}
	if a.settingsError == "ConfigError" {
		a.settingsError = ""
	}
}

func (a *application) changeAutomaticUpdates() {
	a.save()
	a.updates.setAutomatic(a.config.AutomaticUpdates)
}

func (a *application) restartUpdate() {
	if a.updates.snapshot().Phase != updateReady || a.relaunch == nil {
		return
	}
	s := a.awake.snapshot()
	if a.lid != nil {
		if err := a.lid.stop(); err != nil {
			a.moduleError = "LidRestoreFailed"
			a.notifyLid("LidRestoreFailed")
			return
		}
	}
	// The user asked for the restart, so a check that just finished is saved
	// before the process goes away.
	a.syncUpdateCheck()
	a.config.RestoreAwake = s.Active
	a.config.AwakeSession = nil
	if s.Active {
		copy := s.Session
		a.config.AwakeSession = &copy
	}
	if err := saveConfig(a.path, a.config); err != nil {
		a.settingsError = "ConfigError"
		a.invalidateWindow()
		return
	}
	// Relaunch runs the normal quit sequence after persisting awake state.
	a.quitting = true
	a.relaunch()
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

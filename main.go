package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

//go:embed design/locales/*.json assets/*.tiff brand/dist/mark.svg design/assets/lucide-0.468.0/*.svg
var assets embed.FS

const appVersion = "0.0.3"

type application struct {
	registerShortcut       func(string, func()) error
	shortcutSuspended      bool
	refreshApplicationMenu func()
	unregisterShortcut     func(string)
	awake                  *awakeController
	dialog                 *awakeDialog
	shortcutRecording      bool
	shortcutDraft          string
	shortcutError          string
	stopAwakeServices      context.CancelFunc
	notifyBattery          func(string, string)
	config                 Config
	path                   string
	catalog                *i18n.Catalog
	window                 *mygo.Window
	tray                   *mygo.Tray
	page                   int
	language               int
	updates                *updateManager
	notesPage              bool
	relaunch               func()
	settingsError          string
	quitting               bool
	setLogin               func(bool) error
	setLocale              func(string)
}

func newApplication(path string) (*application, error) {
	config, err := loadConfig(path)
	if err != nil {
		return nil, err
	}
	locales, err := fs.Sub(assets, "design/locales")
	if err != nil {
		return nil, err
	}
	catalog, err := i18n.NewCatalog(locales, "zh-CN")
	if err != nil {
		return nil, err
	}
	a := &application{config: config, path: path, catalog: catalog, awake: newAwakeController(), notifyBattery: sendBatteryNotification, registerShortcut: mygo.GlobalShortcut.Register, unregisterShortcut: mygo.GlobalShortcut.Unregister}
	a.updates = newUpdateManager(checkMyGoUpdate, newReleaseNotesClient(appVersion).load, nil)
	for index, language := range []string{"system", "zh-CN", "en"} {
		if config.Language == language {
			a.language = index
		}
	}
	return a, nil
}

func (a *application) text(locale, key string) string { return a.catalog.Format(locale, key, nil) }

func (a *application) save() {
	if err := saveConfig(a.path, a.config); err != nil {
		a.settingsError = "ConfigError"
		log.Print(err)
	} else {
		a.settingsError = ""
	}
	a.invalidateWindow()
}

func (a *application) changeLanguage() {
	language := []string{"system", "zh-CN", "en"}[a.language]
	a.config.Language = language
	a.save()
	if a.setLocale != nil {
		if language == "system" {
			a.setLocale("")
		} else {
			a.setLocale(language)
		}
	}
}

func (a *application) changeLogin() {
	if a.setLogin != nil {
		if err := a.setLogin(a.config.OpenAtLogin); err != nil {
			if a.config.OpenAtLogin {
				if rollbackErr := a.setLogin(false); rollbackErr != nil {
					log.Print(rollbackErr)
				}
			}
			a.config.OpenAtLogin = !a.config.OpenAtLogin
			a.settingsError = "LoginError"
			a.invalidateWindow()
			log.Print(err)
			return
		}
	}
	a.save()
}

func settingsHeight(t tokenValues, page int, hasError bool) int {
	content := 2*t.SizeRowHeight + t.StrokeHairline
	if page == 1 {
		content = t.FontSizeSmall*t.LineHeightBody + t.Space8 + 3*t.SizeRowHeight + 2*t.StrokeHairline
	}
	if hasError {
		content += t.Space8 + t.SizeIcon
	}
	height := max(t.SizeWindowMinHeight, content+2*t.Space24)
	return int(math.Ceil(float64(height/t.Space4)) * float64(t.Space4))
}

// The ready card and a settings error are the tallest non-scrolling page.
func settingsMinimumSize() (int, int) {
	t := tokens(false, "zh-CN")
	return int(t.SizeWindowWidth), settingsHeight(t, 1, true) + int(2*t.Space24+t.SizeRowHeight)
}

func settingsSize(width, height int) (int, int) {
	minWidth, minHeight := settingsMinimumSize()
	return max(minWidth, width), max(minHeight, height)
}

func (a *application) invalidateWindow() {
	if a.window != nil {
		// Page changes invalidate content without replacing the user's chosen size.
		a.window.Invalidate()
	}
}

func (a *application) rememberWindowSize() {
	if a.window == nil {
		return
	}
	width, height := a.window.Size()
	width, height = settingsSize(width, height)
	if a.config.WindowWidth == width && a.config.WindowHeight == height {
		return
	}
	a.config.WindowWidth, a.config.WindowHeight = width, height
	a.save()
}

func (a *application) visibility(visible bool) {
	policy := mygo.ActivationPolicyAccessory
	if visible {
		policy = mygo.ActivationPolicyRegular
	}
	mygo.App.SetActivationPolicy(policy)
}

func (a *application) hide() {
	a.cancelShortcutRecording()
	if a.window == nil {
		return
	}
	a.rememberWindowSize()
	a.window.Hide()
	a.visibility(false)
}

func (a *application) escape() {
	if a.shortcutRecording {
		a.cancelShortcutRecording()
		a.invalidateWindow()
		return
	}
	if a.notesPage {
		a.backNotes()
		return
	}
	a.hide()
}

func (a *application) menu() *mygo.Menu {
	return a.menuForLocale(mygo.App.Locale())
}

func (a *application) menuForLocale(locale string) *mygo.Menu {
	items := append(a.awakeMenu(locale), []*mygo.MenuItem{
		{Label: a.text(locale, "MenuSettings"), Accelerator: "Cmd+,", Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }},
	}...)
	if a.updates.snapshot().Phase == updateReady {
		items = append(items, &mygo.MenuItem{ID: "restart-update", Label: a.text(locale, "MenuRestartUpdate"), Click: func(*mygo.MenuItem, *mygo.Window) { a.restartUpdate() }})
	}
	items = append(items, &mygo.MenuItem{Label: a.text(locale, "Quit"), Accelerator: "Cmd+Q", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }})
	return mygo.NewMenu(items)
}

func (a *application) show() {
	if a.window == nil {
		return
	}
	a.visibility(true)
	a.window.Show()
	a.window.Focus()
}

func (a *application) applicationMenu(locale string) *mygo.Menu {
	text := func(key string) string { return a.text(locale, key) }
	if a.shortcutRecording {
		return mygo.NewMenu([]*mygo.MenuItem{{Label: "Cubbix", Submenu: []*mygo.MenuItem{{Label: text("R05Recording"), Disabled: true}}}})
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Cubbix", Submenu: []*mygo.MenuItem{
			{Label: text("MenuAbout"), Click: func(*mygo.MenuItem, *mygo.Window) { a.page = 1; a.notesPage = false; a.invalidateWindow(); a.show() }},
			{Label: text("MenuSettings"), Accelerator: "Cmd+,", Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }},
			mygo.Separator(),
			{Label: text("Quit"), Role: mygo.RoleQuit, Accelerator: "Cmd+Q"},
		}},
		{Label: text("MenuEdit"), Submenu: []*mygo.MenuItem{
			{Label: text("MenuUndo"), Role: mygo.RoleUndo},
			{Label: text("MenuRedo"), Role: mygo.RoleRedo},
			mygo.Separator(),
			{Label: text("MenuCut"), Role: mygo.RoleCut},
			{Label: text("MenuCopy"), Role: mygo.RoleCopy},
			{Label: text("MenuPaste"), Role: mygo.RolePaste},
			{Label: text("MenuSelectAll"), Role: mygo.RoleSelectAll},
		}},
	})
}

func (a *application) createSettingsWindow() {
	t := tokens(false, "zh-CN")
	width, height := settingsSize(a.config.WindowWidth, a.config.WindowHeight)
	minWidth, minHeight := settingsMinimumSize()
	a.window = mygo.NewWindow(mygo.WindowOptions{Title: "Cubbix", Width: width, Height: height, MinWidth: minWidth, MinHeight: minHeight, Hidden: true, TitleBarStyle: mygo.TitleBarHidden, TrafficLightPosition: &mygo.Point{X: int(t.Space12), Y: int(t.Space12)}, DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true, Content: ui.View(a.view)})
	a.window.OnShow(func() { a.visibility(true) })
	a.window.OnHide(func() { a.cancelShortcutRecording(); a.visibility(false) })
	a.window.OnClosed(func() { a.visibility(false) })
	a.window.OnResize(a.rememberWindowSize)

	a.window.OnClose(func(event *mygo.CloseEvent) {
		if !a.quitting {
			event.PreventDefault()
			a.hide()
		}
	})
}

func (a *application) ready() {
	mygo.App.SetLanguages("zh-CN", "en")
	a.setLocale = mygo.App.SetLocale
	a.setLogin = mygo.App.SetOpenAtLogin
	a.relaunch = mygo.App.Relaunch
	a.refreshApplicationMenu = func() { mygo.App.SetMenu(a.applicationMenu(mygo.App.Locale())) }
	a.updates.notes = newReleaseNotesClient(runningVersion()).load
	if os.Getenv("CUBBIX_QA") != "" {
		if os.Getenv("MYGO_LOCALE") == "en" {
			a.language = 2
		} else {
			a.language = 1
		}
		if os.Getenv("CUBBIX_QA_THEME") == "dark" {
			mygo.Theme.SetSource(mygo.ThemeDark)
		} else {
			mygo.Theme.SetSource(mygo.ThemeLight)
		}
	}
	a.changeLanguage()
	a.config.OpenAtLogin = mygo.App.OpenAtLogin()
	a.createSettingsWindow()
	mygo.App.SetMenu(a.applicationMenu(mygo.App.Locale()))
	mygo.App.OnActivate(func(visible bool) {
		if !visible {
			a.show()
		}
	})
	icon, err := assets.ReadFile("assets/menubar-template.tiff")
	if err != nil {
		log.Fatal(err)
	}
	a.tray, err = mygo.NewTray(mygo.TrayOptions{ID: "com.conteman.cubbix.tray", Icon: icon, IconIsTemplate: true, Menu: a.menu()})
	if err != nil {
		log.Fatal(err)
	}
	mygo.App.OnLocaleChanged(func(locale string) {
		a.refreshAwake()
		mygo.App.SetMenu(a.applicationMenu(locale))
		a.window.Invalidate()
	})
	a.updates.changed = func() {
		mygo.RunOnMain(func() {
			if a.quitting {
				return
			}
			a.invalidateWindow()
			a.tray.SetMenu(a.menu())
		})
	}
	a.startAwakeServices()
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		if a.stopAwakeServices != nil {
			a.stopAwakeServices()
		}
		a.awake.stop()
		mygo.GlobalShortcut.UnregisterAll()
		a.updates.close()
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.tray.Destroy() })
	if os.Getenv("CUBBIX_QA") == "" || !a.qaUpdateState() {
		a.updates.start(a.config.AutomaticUpdates, updateInterval)
	}
	if os.Getenv("CUBBIX_QA") != "" {
		if os.Getenv("CUBBIX_QA_PAGE") == "notes" {
			a.page, a.notesPage = 1, true
		}
		if os.Getenv("CUBBIX_QA_PAGE") == "about" {
			a.page = 1
			a.invalidateWindow()
		}
		log.Printf("status item window: %+v", a.tray.Bounds())
		a.window.ShowInactive()
	}
}

func icon(name string) *ui.SVG {
	data, err := assets.ReadFile("design/assets/lucide-0.468.0/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(data)
}

func brand(color string) *ui.SVG {
	data, err := assets.ReadFile("brand/dist/mark.svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG([]byte(strings.ReplaceAll(string(data), "#202522", color)))
}

func runningVersion() string {
	if v := mygo.App.Version(); stableVersion(v) {
		return v
	}
	return appVersion
}

func version() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && len(setting.Value) >= 7 {
				return runningVersion() + " (" + setting.Value[:7] + ")"
			}
		}
	}
	return runningVersion()
}

func applyTheme(c *ui.Context) (string, tokenValues) {
	locale := c.Locale()
	if locale == "" {
		locale = "zh-CN"
	}
	t := tokens(c.Theme().Dark, locale)
	theme := *c.Theme()
	theme.Background, theme.Surface, theme.SurfaceHover = ui.Hex(t.ColorWindow), ui.Hex(t.ColorControl), ui.Hex(t.ColorControlHover)
	theme.Border, theme.Text, theme.TextMuted = ui.Hex(t.ColorSeparator), ui.Hex(t.ColorText), ui.Hex(t.ColorTextSecondary)
	theme.Accent, theme.AccentText, theme.Danger = ui.Hex(t.ColorAccent), ui.Hex(t.ColorOnAccent), ui.Hex(t.ColorDanger)
	theme.SurfacePressed, theme.AccentHover, theme.AccentPressed = ui.Hex(t.ColorControlHover), ui.Hex(t.ColorAccent), ui.Hex(t.ColorAccent)
	theme.Selection, theme.Focus = ui.Hex(t.ColorAccentSoft), ui.Hex(t.ColorTextSecondary)
	theme.Inverse, theme.InverseText = ui.Hex(t.ColorTooltip), ui.Hex(t.ColorTooltipText)
	theme.Success, theme.Warning, theme.Scrollbar = ui.Hex(t.ColorSuccess), ui.Hex(t.ColorDanger), ui.Hex(t.ColorTextSecondary)
	theme.Radius, theme.Spacing, theme.FontSize = t.RadiusControl, t.Space4, t.FontSizeBody
	theme.Font = t.FontFamily
	c.SetTheme(&theme)
	return locale, t
}

func (a *application) view(c *ui.Context) {
	locale, t := applyTheme(c)
	update := a.updates.snapshot()
	text := func(key string) string { return a.text(locale, key) }
	if !a.shortcutRecording {
		c.OnShortcut(ui.Cmd, ui.KeyW, a.hide)
	}
	c.OnShortcut(0, ui.KeyEscape, a.escape)
	if !a.shortcutRecording {
		c.OnShortcut(ui.Cmd, ui.KeyQ, func() { mygo.App.Quit() })
	}
	ui.Row(c).Fill().Background(ui.Hex(t.ColorWindow)).Children(func() {
		ui.Column(c).Width(t.SizeSidebarWidth).FillHeight().Padding(t.Space12).Gap(t.Space16).Background(ui.Hex(t.ColorSidebar)).DragWindow().Children(func() {
			ui.Box(c).Height(t.Space24)
			ui.Row(c).Height(t.SizeRowHeight).AlignItems(ui.Center).PaddingX(t.Space8).Gap(t.Space8).Children(func() {
				ui.Icon(c, brand(t.ColorText)).Size(t.SizeBrandMark, t.SizeBrandMark).TextColor(ui.Hex(t.ColorText))
				ui.Text(c, "Cubbix").FontSize(t.FontSizeBody).Bold()
			})
			ui.Column(c).FillWidth().Gap(t.Space4).Children(func() {
				for _, navIndex := range []int{0, 2, 3, 1} {
					item := []struct{ key, glyph string }{{"General", "settings-2"}, {"About", "info"}, {"R05Tiles", "blocks"}, {"R05Shortcuts", "keyboard"}}[navIndex]
					index := navIndex
					selected := index == a.page
					background := ui.Hex(t.ColorTransparent)
					if selected {
						background = ui.Hex(t.ColorAccentSoft)
					}
					button := ui.ButtonBase(c.Key(item.key)).Height(t.SizeNavHeight).FillWidth().Radius(t.RadiusCard).FocusRing(true)
					if !selected && button.Hovered() {
						background = ui.Hex(t.ColorNavHover)
					}
					button.Background(background).OnClick(func() {
						a.cancelShortcutRecording()
						a.page, a.notesPage = index, false
						a.invalidateWindow()
					}).Children(func() {
						ui.Row(c).Fill().AlignItems(ui.Center).PaddingX(t.Space8).Gap(t.Space8).Children(func() {
							ui.Icon(c, icon(item.glyph)).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
							ui.Text(c, text(item.key)).FontSize(t.FontSizeBody)
						})
					})
				}
			})
		})
		ui.Column(c).Grow(1).FillHeight().Padding(t.Space24).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Column(c).FillWidth().MaxWidth(t.SizeContentMaxWidth).FillHeight().Gap(t.Space8).Children(func() {
				if a.notesPage {
					a.notesView(c)
					return
				}
				if a.page == 1 {
					ui.Text(c, text("UpdateGroup")).Height(t.FontSizeSmall * t.LineHeightBody).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
				}
				ui.Column(c).FillWidth().PaddingX(t.Space16).Radius(t.RadiusCard).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).Background(ui.Hex(t.ColorCard)).Children(func() {
					if a.page >= 2 {
						a.awakeSettings(c, t, locale)
					} else if a.page == 0 {
						a.row(c, t, text("Language"), func() {
							parts := ui.SegmentedBase(c.Key("language"), &a.language, 3)
							parts.Track.Size(t.SizeLanguageSelectorWidth, t.SizeSegmentHeight).Padding(t.Space2).Radius(t.RadiusControl).Background(ui.Hex(t.ColorControl)).Label(text("Language")).OnChange(a.changeLanguage).Children(func() {
								for index, label := range []string{text("System"), text("Chinese"), text("English")} {
									color, background := t.ColorText, t.ColorTransparent
									if index == a.language {
										color, background = t.ColorOnAccent, t.ColorAccent
									}
									parts.Segment(index).Grow(1).FillHeight().Radius(t.RadiusControl).Background(ui.Hex(background)).FocusRing(true).Children(func() { ui.Text(c, label).FontSize(t.FontSizeBody).TextColor(ui.Hex(color)) })
								}
							})
						})
						ui.Divider(c)
						a.row(c, t, text("LaunchAtLogin"), func() { a.switchControl(c, t, "login", text("LaunchAtLogin"), &a.config.OpenAtLogin, a.changeLogin) })
					} else {
						a.row(c, t, text("Version"), func() {
							ui.Text(c, version()).FontSize(t.FontSizeBody).TextColor(ui.Hex(t.ColorTextSecondary))
							ui.ButtonBase(c.Key("check")).Size(t.SizeIconButton, t.SizeIconButton).Disabled(update.Phase == updateChecking || update.Phase == updateDownloading || update.Phase == updateReady).Label(text("CheckAction")).Tooltip(text("CheckAction")).OnClick(func() { a.updates.request(true) }).Children(func() {
								ui.Icon(c, icon("refresh-cw")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
							})
							a.updateIndicator(c, t, update, text)
						})
						ui.Divider(c)
						ui.ButtonBase(c.Key("notes")).FillWidth().Height(t.SizeRowHeight).FocusRing(true).OnClick(a.showNotes).Children(func() {
							ui.Row(c).Fill().AlignItems(ui.Center).Gap(t.Space8).Children(func() {
								ui.Text(c, text("WhatsNew")).FontSize(t.FontSizeBody).Grow(1)
								ui.Icon(c, icon("chevron-right")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary)).Tooltip(text("WhatsNew"))
								if update.NotesError != "" {
									ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).Tooltip(text(update.NotesError))
								}
							})
						})
						ui.Divider(c)
						a.row(c, t, text("AutomaticUpdates"), func() {
							a.switchControl(c, t, "auto", text("AutomaticUpdates"), &a.config.AutomaticUpdates, a.changeAutomaticUpdates)
						})
					}
				})
				if a.page == 1 && update.Phase == updateReady {
					ui.Box(c).Height(t.Space8)
					a.readyCard(c, t, text)
				}
				errorKey := a.settingsError
				if errorKey == "" {
					errorKey = a.awake.snapshot().Error
				}
				if errorKey != "" {
					ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).Tooltip(text(errorKey))
				}
			})
		})
	})
	// The subpage owns Esc even while a tooltip is visible. Register last so
	// its handler takes precedence over the tooltip's dismiss shortcut.
	if a.notesPage {
		ui.Overlay(c, func() {
			if ui.Box(c.Key("notes-escape")).Size(0, 0).PassThrough().OverlayShortcut(0, ui.KeyEscape) {
				a.backNotes()
			}
		})
	}
}

func (a *application) switchControl(c *ui.Context, t tokenValues, key, label string, value *bool, changed func()) {
	background, thumb, align := t.ColorControl, t.ColorTextSecondary, ui.Start
	if *value {
		background, thumb, align = t.ColorAccent, t.ColorThumb, ui.End
	}
	ui.SwitchBase(c.Key(key), value).Size(t.SizeSwitchWidth, t.SizeSwitchHeight).Radius(t.RadiusControl).Background(ui.Hex(background)).Padding(t.Space2).Justify(align).AlignItems(ui.Center).Label(label).FocusRing(true).OnChange(changed).Children(func() {
		ui.Box(c).Size(t.SizeSwitchThumb, t.SizeSwitchThumb).Radius(t.SizeSwitchThumb / 2).Background(ui.Hex(thumb))
	})
}

func (a *application) row(c *ui.Context, t tokenValues, label string, right func()) {
	ui.Row(c).FillWidth().Height(t.SizeRowHeight).AlignItems(ui.Center).Gap(t.Space8).Children(func() {
		ui.Text(c, label).FontSize(t.FontSizeBody).Grow(1)
		right()
	})
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	configPath := filepath.Join(home, "Library", "Application Support", "cubbix", "config.toml")
	if qaPath := os.Getenv("CUBBIX_QA_CONFIG"); os.Getenv("CUBBIX_QA") != "" && qaPath != "" {
		configPath = qaPath
	}
	a, err := newApplication(configPath)
	if err != nil {
		log.Fatal(fmt.Errorf("configuration: %w", err))
	}
	mygo.App.SetName("Cubbix")
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mygo.App.WhenReady(a.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

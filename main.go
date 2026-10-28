package main

import (
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

//go:embed design/locales/*.json assets/menubar-template.tiff brand/dist/mark.svg design/assets/lucide-0.468.0/settings-2.svg design/assets/lucide-0.468.0/info.svg design/assets/lucide-0.468.0/refresh-cw.svg design/assets/lucide-0.468.0/triangle-alert.svg design/assets/lucide-0.468.0/chevron-right.svg
var assets embed.FS

const appVersion = "0.0.1"

type application struct {
	config        Config
	path          string
	catalog       *i18n.Catalog
	window        *mygo.Window
	tray          *mygo.Tray
	page          int
	language      int
	updateError   bool
	settingsError string
	quitting      bool
	setLogin      func(bool) error
	setLocale     func(string)
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
	a := &application{config: config, path: path, catalog: catalog}
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
	a.resizeWindow()
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
			a.resizeWindow()
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

func (a *application) resizeWindow() {
	if a.window != nil {
		t := tokens(false, "zh-CN")
		a.window.SetSize(int(t.SizeWindowWidth), settingsHeight(t, a.page, a.settingsError != ""))
	}
}

func (a *application) menu() *mygo.Menu {
	locale := mygo.App.Locale()
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: a.text(locale, "MenuSettings"), Accelerator: "Cmd+,", Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }},
		{Label: a.text(locale, "Quit"), Accelerator: "Cmd+Q", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}

func (a *application) show() { a.window.Show(); a.window.Focus() }

func (a *application) ready() {
	mygo.App.SetLanguages("zh-CN", "en")
	a.setLocale = mygo.App.SetLocale
	a.setLogin = mygo.App.SetOpenAtLogin
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
	t := tokens(false, "zh-CN")
	a.window = mygo.NewWindow(mygo.WindowOptions{Title: "Cubbix", Width: int(t.SizeWindowWidth), Height: int(t.SizeWindowHeight), Hidden: true, Frameless: true, DisableResize: true, DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true, Content: ui.View(a.view)})
	a.window.OnClose(func(event *mygo.CloseEvent) {
		if !a.quitting {
			event.PreventDefault()
			a.window.Hide()
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
	mygo.App.OnLocaleChanged(func(string) { a.tray.SetMenu(a.menu()) })
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.tray.Destroy() })
	if os.Getenv("CUBBIX_QA") != "" {
		if os.Getenv("CUBBIX_QA_PAGE") == "about" {
			a.page = 1
			a.resizeWindow()
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

func version() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && len(setting.Value) >= 7 {
				return appVersion + " (" + setting.Value[:7] + ")"
			}
		}
	}
	return appVersion
}

func (a *application) view(c *ui.Context) {
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
	text := func(key string) string { return a.text(locale, key) }
	c.OnShortcut(ui.Cmd, ui.KeyW, func() {
		if a.window != nil {
			a.window.Hide()
		}
	})
	c.OnShortcut(0, ui.KeyEscape, func() {
		if a.window != nil {
			a.window.Hide()
		}
	})
	c.OnShortcut(ui.Cmd, ui.KeyQ, func() { mygo.App.Quit() })
	ui.Row(c).Fill().Background(ui.Hex(t.ColorWindow)).Children(func() {
		ui.Column(c).Width(t.SizeSidebarWidth).FillHeight().Padding(t.Space12).Gap(t.Space16).Background(ui.Hex(t.ColorSidebar)).DragWindow().Children(func() {
			ui.Row(c).Height(t.SizeRowHeight).AlignItems(ui.Center).PaddingX(t.Space8).Gap(t.Space8).Children(func() {
				ui.Icon(c, brand(t.ColorText)).Size(t.SizeBrandMark, t.SizeBrandMark).TextColor(ui.Hex(t.ColorText))
				ui.Text(c, "Cubbix").FontSize(t.FontSizeBody).Bold()
			})
			ui.Column(c).FillWidth().Gap(t.Space4).Children(func() {
				for index, item := range []struct{ key, glyph string }{{"General", "settings-2"}, {"About", "info"}} {
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
						a.page = index
						a.resizeWindow()
					}).Children(func() {
						ui.Row(c).Fill().AlignItems(ui.Center).PaddingX(t.Space8).Gap(t.Space8).Children(func() {
							ui.Icon(c, icon(item.glyph)).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
							ui.Text(c, text(item.key)).FontSize(t.FontSizeBody)
						})
					})
				}
			})
		})
		ui.Column(c).Grow(1).FillHeight().Padding(t.Space24).Gap(t.Space8).DragWindow().Children(func() {
			if a.page == 1 {
				ui.Text(c, text("UpdateGroup")).Height(t.FontSizeSmall * t.LineHeightBody).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
			}
			ui.Column(c).FillWidth().PaddingX(t.Space16).Radius(t.RadiusCard).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).Background(ui.Hex(t.ColorCard)).Children(func() {
				if a.page == 0 {
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
						ui.ButtonBase(c.Key("check")).Size(t.SizeIconButton, t.SizeIconButton).Label(text("CheckAction")).Tooltip(text("CheckAction")).OnClick(func() { a.updateError = true }).Children(func() {
							ui.Icon(c, icon("refresh-cw")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
						})
						if a.updateError {
							ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).Tooltip(text("UpdateSourceMissing"))
						}
					})
					ui.Divider(c)
					a.row(c, t, text("WhatsNew"), func() {
						ui.ButtonBase(c.Key("notes")).Size(t.SizeIconButton, t.SizeIconButton).Label(text("WhatsNew")).Tooltip(text("UpdateNotesUnavailable")).OnClick(func() { a.updateError = true }).Children(func() {
							ui.Icon(c, icon("chevron-right")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
						})
					})
					ui.Divider(c)
					a.row(c, t, text("AutomaticUpdates"), func() { a.switchControl(c, t, "auto", text("AutomaticUpdates"), &a.config.AutomaticUpdates, a.save) })
				}
			})
			if a.settingsError != "" {
				ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).Tooltip(text(a.settingsError))
			}
		})
	})
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

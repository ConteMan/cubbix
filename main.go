package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

//go:embed design/locales/*.json assets/*.tiff brand/dist/mark.svg design/assets/lucide-0.468.0/*.svg
var assets embed.FS

//go:embed frontend/dist/*
var jsonAssets embed.FS

const appVersion = "0.6.1"

type application struct {
	lidCompletion func(error)
	lidLastCode   string
	lidMessage    string
	lidAuthorize  bool
	lidPolling    bool

	lid                       *lidController
	lidBusy, lidAsync, lidURL bool
	lidNotice                 string
	lidPending                *bool

	jsonDocuments        []*jsonDocument
	jsonNext             int
	jsonReadClipboard    func() string
	createJSONWindow     func(*jsonDocument)
	presentJSONWindow    func(*jsonDocument)
	jsonCopy             func(string) error
	jsonChooseOpen       func(*mygo.Window) (string, error)
	jsonChooseSave       func(*mygo.Window) (string, error)
	jsonShortcutError    string
	shortcutTarget       string
	palette              *paletteState
	paletteShortcutError string
	panelShortcutError   string
	// displays and cursorPoint are the shared screen seams: the palette and
	// every window placement read them, and tests replace them so no case
	// needs a real display or the pointer. paletteWindowPosition and
	// setPaletteWindowPosition are seams too, so a drag needs no native window.
	displays                 func() []mygo.Display
	cursorPoint              func() mygo.Point
	paletteWindowPosition    func() screenPoint
	setPaletteWindowPosition func(screenPoint)
	// settingsFrame is a seam: tests drive show and placement with a fake
	// frame instead of a native settings window.
	settingsFrame func() placementWindow
	// knownDisplays is the last display snapshot the notification handler
	// saw; settingsPlace and the JSON documents keep their own as well.
	knownDisplays   []mygo.Display
	displayRevision uint64 // invalidates display-menu state after a topology change
	settingsPlace   windowPlaceState
	moveSeq         uint64
	// jsonCommitted is the move sequence each display already has in its JSON
	// seat, for this run only: a later close of an older window cannot win.
	jsonCommitted map[int64]uint64
	// createPaletteGuide is a seam too: tests never open the overlay.
	createPaletteGuide func(*paletteGuideState)
	guide              *paletteGuideState
	feedback           *feedbackPresenter
	feedbackMode       int // index into feedbackModes
	feedbackCorner     int // index into feedbackCorners
	panel              *menubarPanelState
	// createPaletteWindow and createMenubarPanel are seams: tests replace
	// them so no case creates a native window.
	createPaletteWindow func(*paletteState)
	createMenubarPanel  func(*menubarPanelState)
	// paletteOpenJSON and registerURLScheme are seams: tests replace them so
	// no case creates a native window or changes the system URL handler.
	paletteOpenJSON   func(string)
	registerURLScheme func(string) error
	urlDispatch       func(func())

	// cli is the command line socket server, nil until ready.
	cli *cliServer
	// translate is the floating window state. The remaining fields are seams:
	// headless tests replace the window factory, the keychain, the main-thread
	// hop, the clipboard and the HTTP functions, so no case touches a real
	// window, keychain, shortcut or network.
	translate               *translateState
	translateShortcutError  string
	translateRowError       string
	translateKeyDraft       string
	translateKeySaved       bool
	translateKeyEditing     bool
	translateKeyFocus       bool
	translateKeyWriting     bool
	translateKeyGeneration  uint64
	translateKeyError       string
	translateModelList      []string
	translateModelListError string
	translateModelFetching  bool
	translateTestRunning    bool
	translateTestKey        string
	translateTestUntil      time.Time
	translateCLIDispatch    func(func())
	translateSecret         translateSecretStore
	translateRunOnMain      func(func())
	translateAsync          bool
	translateStreamFn       translateStreamFunc
	translateModelsFn       translateModelsFunc
	translateTestFn         translateTestFunc
	translateDictionaryFn   func(string) string
	translateCopy           func(string) error
	// speech is the pronunciation seam. ready() installs the real engine;
	// headless tests keep it nil or inject a fake, so no test makes a sound.
	speech                speechEngine
	createTranslateWindow func(*translateState)
	// selection is the AX seam. It stays nil in every headless test; ready()
	// installs the real reader, so tests can neither read the frontmost
	// application nor cause a permission prompt.
	selection                    selectionReader
	selectionReading             atomic.Bool
	selectionBudget              time.Duration
	openAccessibilitySettings    func()
	translateSelectionTrusted    bool
	translateSelectionHintShown  bool
	accessibilitySettingsOpened  bool
	translateSelectionPending    bool
	translateSelectionGeneration uint64
	// quiet silences the hint and the lid notification for one command line
	// request; it is set and reset on the main thread only.
	quiet                  bool
	registerShortcut       func(string, func()) error
	shortcutSuspended      bool
	refreshApplicationMenu func()
	unregisterShortcut     func(string)
	awake                  *awakeController
	dialog                 *awakeDialog
	shortcutRecording      bool
	shortcutDraft          string
	shortcutError          string
	// registeredShortcut records the combination each slot really owns. A
	// close may only release the combination it registered, never one another
	// slot took over in the meantime.
	registeredShortcut map[string]string
	// suspendedShortcut preserves the real owners across recorder suspension.
	// Failed slots may remember the same key, but cannot take it on resume.
	suspendedShortcut map[string]string
	// moduleClosing is true while a keep-awake close runs on a worker. New
	// requests are refused until it settles; the committed state stays on.
	moduleClosing bool
	moduleAsync   bool
	moduleError   string
	// moduleGeneration lets a late completion see it no longer owns the state.
	moduleGeneration uint64
	// focusModuleSwitch asks the next frame to focus the module switch once.
	focusModuleSwitch bool
	stopAwakeServices context.CancelFunc
	notifyBattery     func(string, string)
	policy            mygo.ActivationPolicy
	config            Config
	path              string
	catalog           *i18n.Catalog
	window            *mygo.Window
	tray              *mygo.Tray
	page              int
	language          int
	updates           *updateManager
	notesPage         bool
	relaunch          func()
	settingsError     string
	quitting          bool
	quitGuard         *quitWatchdog
	setLogin          func(bool) error
	setLocale         func(string)
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
	a := &application{config: config, path: path, catalog: catalog, awake: newAwakeController(), moduleAsync: true, notifyBattery: newBatteryNotifier().send, registerShortcut: mygo.GlobalShortcut.Register, unregisterShortcut: mygo.GlobalShortcut.Unregister, registerURLScheme: mygo.App.RegisterURLScheme, urlDispatch: mygo.RunOnMain}
	a.createPaletteWindow = a.buildPaletteWindow
	a.createPaletteGuide = a.buildPaletteGuideWindow
	a.displays = mygo.Screen.Displays
	a.cursorPoint = mygo.Screen.CursorScreenPoint
	a.paletteWindowPosition = func() screenPoint {
		if a.palette == nil || a.palette.window == nil {
			return screenPoint{}
		}
		x, y := a.palette.window.Position()
		return screenPoint{X: x, Y: y}
	}
	a.setPaletteWindowPosition = func(p screenPoint) {
		if a.palette == nil || a.palette.window == nil {
			return
		}
		a.palette.window.SetPosition(p.X, p.Y)
	}
	a.createMenubarPanel = a.buildMenubarPanel
	a.createTranslateWindow = a.buildTranslateWindow
	a.translateSecret = mygo.Secrets
	a.translateRunOnMain = mygo.RunOnMain
	a.translateAsync = true
	a.translateDictionaryFn = dictionaryDefinition
	a.translateCopy = defaultTranslateCopy
	a.selectionBudget = selectionTimeout
	a.createJSONWindow = a.buildJSONWindow
	a.presentJSONWindow = a.focusJSONWindow
	a.feedbackMode = feedbackIndex(feedbackModes, config.FeedbackMode)
	a.feedbackCorner = feedbackIndex(feedbackCorners, config.FeedbackCorner)
	a.feedback = newFeedbackPresenter()
	a.feedback.mode = a.effectiveFeedbackMode
	a.feedback.corner = a.feedbackCornerName
	a.feedback.create = a.buildFeedbackWindow
	a.feedback.changed = a.invalidateWindow
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
	if !visible {
		if a.window != nil && a.window.IsVisible() {
			visible = true
		}
		for _, d := range a.jsonDocuments {
			if d.window != nil && d.window.IsVisible() {
				visible = true
				break
			}
		}
	}
	if visible {
		policy = mygo.ActivationPolicyRegular
	}
	// MyGo activates the app when the policy becomes regular. Repeating an
	// unchanged policy would pull the user back out of whatever app or Space
	// they moved to, so only a real change reaches AppKit.
	if a.policy == policy {
		return
	}
	a.policy = policy
	mygo.App.SetActivationPolicy(policy)
}

func (a *application) hide() {
	a.cancelShortcutRecording()
	w := a.settingsPlacementWindow()
	if w == nil {
		return
	}
	a.rememberWindowSize()
	// The place is read while the window is still alive: after Hide, or after
	// a change of the display layout, the frame no longer means anything.
	if a.rememberSettingsPlace() {
		a.save()
	}
	w.Hide()
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

// contextMenu is the tray's right-click menu: the system actions that stay
// outside the card panel. Left clicks open the panel instead.
func (a *application) contextMenu(locale string) *mygo.Menu {
	items := []*mygo.MenuItem{
		{ID: "settings", Label: a.text(locale, "MenuSettings"), Accelerator: "Cmd+,", Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }},
	}
	if a.updates.snapshot().Phase == updateReady {
		items = append(items, &mygo.MenuItem{ID: "restart-update", Label: a.text(locale, "MenuRestartUpdate"), Click: func(*mygo.MenuItem, *mygo.Window) { a.restartUpdate() }})
	}
	items = append(items, &mygo.MenuItem{ID: "quit", Label: a.text(locale, "Quit"), Accelerator: "Cmd+Q", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }})
	return mygo.NewMenu(items)
}

// showContextMenu opens the right-click menu from the tray icon. The menu is
// bound for the popup only, so left clicks keep reaching OnClick.
func (a *application) showContextMenu() {
	if a.tray == nil {
		return
	}
	a.hideMenubarPanel()
	a.tray.SetMenu(a.contextMenu(mygo.App.Locale()))
	defer a.tray.SetMenu(nil)
	a.tray.PopUpMenu()
}

// show opens the settings window. An already visible window is only shown and
// focused again: it never re-places itself, so a second shortcut or menu item
// cannot move the window under the user's pointer.
func (a *application) show() {
	w := a.settingsPlacementWindow()
	if w == nil {
		return
	}
	if w.IsMinimized() {
		w.Restore()
		// Coming back from the Dock repairs a changed display layout, but does
		// not follow the pointer.
		a.reconcileSettingsDisplayIfNeeded()
	} else if !w.IsVisible() {
		a.placeSettingsWindow()
	}
	a.visibility(true)
	w.Show()
	w.Focus()
}

func (a *application) applicationMenu(locale string) *mygo.Menu {
	text := func(key string) string { return a.text(locale, key) }
	if a.shortcutRecording {
		return mygo.NewMenu([]*mygo.MenuItem{{Label: "Cubbix", Submenu: []*mygo.MenuItem{{Label: text("R05Recording"), Disabled: true}}}})
	}
	items := []*mygo.MenuItem{
		{Label: "Cubbix", Submenu: []*mygo.MenuItem{
			{Label: text("MenuAbout"), Click: func(*mygo.MenuItem, *mygo.Window) { a.page = 1; a.notesPage = false; a.invalidateWindow(); a.show() }},
			{Label: text("MenuSettings"), Accelerator: "Cmd+,", Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }},
			mygo.Separator(),
			{Label: text("Quit"), Role: mygo.RoleQuit, Accelerator: "Cmd+Q"},
		}},
		{Label: text("JSONFileMenu"), Submenu: []*mygo.MenuItem{
			{Label: text("R07NewWindow"), Disabled: !a.config.JSONEnabled, Click: func(*mygo.MenuItem, *mygo.Window) { a.newJSONWindow() }},
			{Label: text("R07Open"), Accelerator: "Cmd+O", Click: func(_ *mygo.MenuItem, w *mygo.Window) {
				if d := a.jsonForWindow(w); d != nil {
					a.openJSONFile(d)
				}
			}},
			{Label: text("JSONSave"), Accelerator: "Cmd+S", Click: func(_ *mygo.MenuItem, w *mygo.Window) { a.jsonPageAction(w, "save(false)") }},
			{Label: text("JSONSaveAs"), Accelerator: "Cmd+Shift+S", Click: func(_ *mygo.MenuItem, w *mygo.Window) { a.jsonPageAction(w, "save(true)") }},
			{Label: text("JSONClose"), Role: mygo.RoleClose, Accelerator: "Cmd+W"},
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
	}
	if !a.config.JSONEnabled {
		items = append(items[:1], items[2:]...)
	}
	return mygo.NewMenu(items)
}

func (a *application) createSettingsWindow() {
	t := tokens(false, "zh-CN")
	width, height := settingsSize(a.config.WindowWidth, a.config.WindowHeight)
	minWidth, minHeight := settingsMinimumSize()
	// Close, minimize and zoom keep their system symbols; full screen stays off.
	a.window = mygo.NewWindow(mygo.WindowOptions{Title: "Cubbix", Width: width, Height: height, MinWidth: minWidth, MinHeight: minHeight, Hidden: true, TitleBarStyle: mygo.TitleBarHidden, TrafficLightPosition: &mygo.Point{X: int(t.Space12), Y: int(t.Space12)}, DisableFullScreen: true, Content: ui.View(a.view)})
	a.window.OnShow(func() { a.visibility(true) })
	a.window.OnHide(func() { a.cancelShortcutRecording(); a.visibility(false) })
	a.window.OnClosed(func() { a.visibility(false) })
	a.window.OnResize(a.rememberWindowSize)
	a.window.OnMove(a.observeSettingsMove)
	a.window.OnRestore(a.reconcileSettingsDisplayIfNeeded)
	a.window.OnFocus(a.onSettingsFocus)

	a.window.OnClose(func(event *mygo.CloseEvent) {
		if !a.quitting {
			event.PreventDefault()
			a.hide()
		}
	})
}

func (a *application) ready() {
	mygo.App.SetLanguages("zh-CN", "en")
	// The real AX reader, the System Settings opener and the speech engine
	// exist only in the running app. Headless tests keep these seams nil.
	a.selection = systemSelectionReader{}
	a.openAccessibilitySettings = openAccessibilitySettingsPane
	a.speech = newSystemSpeech()
	if a.selectionBudget <= 0 {
		a.selectionBudget = selectionTimeout
	}

	a.setLocale = mygo.App.SetLocale
	a.setLogin = mygo.App.SetOpenAtLogin
	a.installQuitWatchdog()
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
	// The first display snapshot: frames draw from it, and only window
	// placement and display changes take a new one.
	a.knownDisplays = a.displaysNow()
	a.createSettingsWindow()
	mygo.App.SetMenu(a.applicationMenu(mygo.App.Locale()))
	mygo.App.OnActivate(func(visible bool) {
		// A visible palette is a tool window, not a settings request.
		if a.paletteVisible() {
			a.hidePalette()
			return
		}
		if !visible {
			a.show()
		}
	})
	icon, err := assets.ReadFile("assets/menubar-template.tiff")
	if err != nil {
		log.Fatal(err)
	}
	a.tray, err = mygo.NewTray(mygo.TrayOptions{ID: "com.conteman.cubbix.tray", Icon: icon, IconIsTemplate: true})
	if err != nil {
		log.Fatal(err)
	}
	// No menu is bound: left clicks open the card panel, and the right-click
	// menu is built for its popup only.
	a.tray.OnClick(a.toggleMenubarPanel)
	a.tray.OnRightClick(a.showContextMenu)
	mygo.App.OnLocaleChanged(func(locale string) {
		a.refreshAwake()
		a.refreshJSONTitles(locale)
		mygo.App.SetMenu(a.applicationMenu(locale))
		a.window.Invalidate()
	})
	a.updates.changed = func() {
		mygo.RunOnMain(func() {
			// The check time is recorded on a worker; only the main thread
			// writes configuration, and only when the stored value differs.
			a.syncUpdateCheck()
			if a.quitting {
				return
			}
			a.invalidateWindow()
		})
	}
	a.updates.restoreCheck(a.config.UpdateLastCheck, time.Now())
	a.prepareLid()
	a.startAwakeServices()
	a.registerJSONShortcut()
	a.registerPaletteShortcut()
	a.registerPanelShortcut()
	a.registerTranslateShortcut()
	a.refreshTranslateKey()
	a.feedback.watch()
	a.startCLIServer()
	if err := a.registerURLScheme("cubbix"); err != nil {
		// A missing declaration only disables deep links; it must not block launch.
		log.Print(err)
	}
	mygo.Screen.OnDisplaysChanged(func() {
		mygo.RunOnMain(a.displaysChanged)
	})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnBeforeQuit(a.beforeQuit)
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

// beforeQuit releases services without guarding tool window contents.
func (a *application) beforeQuit(*mygo.QuitEvent) {
	// Arm before cleanup: even a blocked main thread must have a deadline.
	if a.quitGuard != nil {
		a.quitGuard.arm(false)
	}
	// JSON windows are tools: quitting never asks about unsaved text.
	a.quitting = true
	// One save for every window that moved: the settings seat and the newest
	// JSON seat of each display. Windows are still alive here.
	a.saveWindowPlacesOnQuit()
	a.cli.stop()
	a.cancelTranslate()
	a.hidePalette()
	a.hidePaletteGuides()
	a.hideMenubarPanel()
	a.feedback.close()
	if a.stopAwakeServices != nil {
		a.stopAwakeServices()
	}
	// Do not wait past the quit budget if the helper is unreachable.
	if a.lid != nil {
		a.lid.stopForQuit()
	}
	a.awake.setBeforeStop(nil)
	a.awake.stop()
	mygo.GlobalShortcut.UnregisterAll()
	// A check that finished just before quitting is still worth remembering.
	a.syncUpdateCheck()
	a.updates.close()
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
	theme.ScrollbarWidth = t.SizeScrollbar
	theme.Font = t.FontFamily
	c.SetTheme(&theme)
	return locale, t
}

func (a *application) view(c *ui.Context) {
	locale, t := applyTheme(c)
	if !a.shortcutRecording {
		c.OnShortcut(ui.Cmd, ui.KeyW, a.hide)
	}
	c.OnShortcut(0, ui.KeyEscape, a.escape)
	if !a.shortcutRecording {
		c.OnShortcut(ui.Cmd, ui.KeyQ, func() { mygo.App.Quit() })
	}
	ui.Row(c).Fill().Background(ui.Hex(t.ColorWindow)).Children(func() {
		a.sidebar(c, t, locale)
		ui.Column(c).Grow(1).FillHeight().Padding(t.Space24).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Column(c).FillWidth().MaxWidth(t.SizeContentMaxWidth).FillHeight().Children(func() {
				if a.notesPage {
					a.notesView(c)
					return
				}
				a.settingsView(c, t, locale)
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

// switchControl draws one switch. The caller binds a value for this frame only:
// a module switch never writes the config before its transaction commits. It
// returns its element so a caller can ask for the focus once.
func (a *application) switchControl(c *ui.Context, t tokenValues, key, label string, value *bool, changed func(), tip string, disabled bool) ui.Element {
	background, thumb, align := t.ColorControl, t.ColorTextSecondary, ui.Start
	if *value {
		background, thumb, align = t.ColorAccent, t.ColorThumb, ui.End
	}
	base := ui.SwitchBase(c.Key(key), value).Size(t.SizeSwitchWidth, t.SizeSwitchHeight).Radius(t.RadiusControl).Background(ui.Hex(background)).Padding(t.Space2).Justify(align).AlignItems(ui.Center).Label(label).FocusRing(true)
	if tip != "" {
		// The tooltip is also the accessible description: a truncated level-2
		// line must never lose its full text.
		base = base.Tooltip(tip).Description(tip)
	}
	if disabled {
		base = base.Disabled(true).Opacity(t.OpacityDisabled)
	} else {
		base = base.OnChange(changed)
	}
	return base.Children(func() {
		ui.Box(c).Size(t.SizeSwitchThumb, t.SizeSwitchThumb).Radius(t.SizeSwitchThumb / 2).Background(ui.Hex(thumb))
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
	a.setupJSON()
	// Register before Run so the URL that launched the app also arrives.
	mygo.App.OnOpenURL(a.handleURL)
	mygo.App.WhenReady(a.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

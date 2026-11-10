package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// fakeTranslateSecrets keeps API keys in memory only. No test reads the real
// login keychain.
type fakeTranslateSecrets struct {
	value   []byte
	getErr  error
	setErr  error
	sets    int
	deletes int
}

func newFakeTranslateSecrets() *fakeTranslateSecrets { return &fakeTranslateSecrets{} }

func (f *fakeTranslateSecrets) Get(string) ([]byte, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.value == nil {
		return nil, mygo.ErrSecretNotFound
	}
	return f.value, nil
}

func (f *fakeTranslateSecrets) Set(_ string, value []byte) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.value = value
	f.sets++
	return nil
}

func (f *fakeTranslateSecrets) Delete(string) error {
	f.value = nil
	f.deletes++
	return nil
}

func translateTestApp(t *testing.T) *application {
	t.Helper()
	a := awakeTestApp(t)
	a.translateSecret = newFakeTranslateSecrets()
	return a
}

func TestTranslateShortcutRecording(t *testing.T) {
	a := translateTestApp(t)
	beforeAwake := a.config.AwakeShortcut
	a.beginTileShortcutRecording("translate")
	if !a.shortcutRecording || a.shortcutTarget != "translate" {
		t.Fatal("translate recorder did not start")
	}
	a.recordShortcut(ui.Ctrl|ui.Alt|ui.Cmd, ui.KeyT)
	if a.config.TranslateShortcut != "Ctrl+Alt+Cmd+T" {
		t.Fatalf("translate shortcut = %q", a.config.TranslateShortcut)
	}
	if a.config.AwakeShortcut != beforeAwake {
		t.Fatalf("recording translate overwrote the keep-awake shortcut: %q", a.config.AwakeShortcut)
	}
	if a.shortcutRecording || a.shortcutTarget != "" {
		t.Fatal("recorder stayed open")
	}
	// An unknown target writes nothing at all.
	before := []string{a.config.AwakeShortcut, a.config.JSONShortcut, a.config.PaletteShortcut, a.config.PanelShortcut, a.config.TranslateShortcut}
	a.shortcutTarget, a.shortcutRecording = "bogus", true
	a.recordShortcut(ui.Ctrl|ui.Alt|ui.Cmd, ui.KeyY)
	after := []string{a.config.AwakeShortcut, a.config.JSONShortcut, a.config.PaletteShortcut, a.config.PanelShortcut, a.config.TranslateShortcut}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("unknown target wrote shortcut %d: %q -> %q", i, before[i], after[i])
		}
	}
}

func TestTranslateShortcutUnavailable(t *testing.T) {
	a := translateTestApp(t)
	a.registerShortcut = func(string, func()) error { return errors.New("busy") }
	a.registerTranslateShortcut()
	if got := a.slotError("translate"); got != "ShortcutUnavailable" {
		t.Fatalf("slot error = %q", got)
	}
	a.page = settingsTranslate
	tt := ui.NewTester(a.view, 720, settingsWindowHeight)
	tt.SetLocale("en")
	tt.Frame()
	if _, ok := tt.Find(a.text("en", "ShortcutUnavailable")); !ok {
		t.Fatalf("missing registration failure hint: %v", tt.Texts())
	}
}

func TestTranslateSlotDisabledWhenModuleOff(t *testing.T) {
	a := translateTestApp(t)
	a.config.TranslateEnabled = false
	if a.shortcutSlotEnabled("translate") {
		t.Fatal("released slot still enabled")
	}
	a.page = settingsTranslate
	tt := ui.NewTester(a.view, 720, settingsWindowHeight)
	tt.SetLocale("en")
	tt.Frame()
	reason := fmt.Sprintf(a.text("en", "ModuleShortcutOff"), a.text("en", "Translate"))
	if _, ok := tt.Find(reason); !ok {
		t.Fatalf("disabled row has no release reason: %v", tt.Texts())
	}
	if _, ok := tt.Find(a.text("en", "TranslateBaseURL")); ok {
		t.Fatal("settings group was drawn for a closed module")
	}
}

func TestTranslateModuleSwitch(t *testing.T) {
	a := translateTestApp(t)
	a.registerTranslateShortcut()
	if a.ownedShortcut("translate") == "" {
		t.Fatal("shortcut was not registered")
	}
	a.changeTranslateEnabled(false)
	if a.config.TranslateEnabled || a.ownedShortcut("translate") != "" {
		t.Fatalf("module off kept its registration: %+v", a.registeredShortcut)
	}
	a.toggleTranslate()
	if a.translateVisible() {
		t.Fatal("a closed module opened the panel")
	}
	a.changeTranslateEnabled(true)
	if !a.config.TranslateEnabled || a.ownedShortcut("translate") == "" {
		t.Fatal("module on did not restore the shortcut")
	}
	if a.config.TranslateShortcut != defaultTranslateKey {
		t.Fatalf("module switch changed the shortcut: %q", a.config.TranslateShortcut)
	}
}

func TestTranslateNotConfiguredAndStreaming(t *testing.T) {
	a := translateTestApp(t)
	called := false
	a.translateStreamFn = func(context.Context, translateConfig, string, string, string, func(string)) (string, error) {
		called = true
		return "", nil
	}
	s := a.ensureTranslateState()
	s.input = "hello"
	a.submitTranslate()
	if s.phase != translateNotConfigured || called {
		t.Fatalf("unconfigured submit started a request: %+v", s)
	}

	secrets := a.translateSecret.(*fakeTranslateSecrets)
	secrets.value = []byte("k")
	a.config.TranslateBaseURL = "https://h/v1"
	a.config.TranslateModel = "group/model"
	streamedPhase := translateEmpty
	a.translateStreamFn = func(_ context.Context, cfg translateConfig, text, source, target string, onDelta func(string)) (string, error) {
		if cfg.Key != "k" || cfg.Model != "group/model" || text != "hello" {
			t.Fatalf("request = %+q %q %q %q", cfg, text, source, target)
		}
		onDelta("你")
		streamedPhase = a.translate.phase
		onDelta("好")
		return "你好", nil
	}
	s.input = "hello"
	a.submitTranslate()
	if streamedPhase != translateStreaming || s.phase != translateDone || s.output != "你好" {
		t.Fatalf("stream = phase %v, output %q", s.phase, s.output)
	}
	a.translateStreamFn = func(context.Context, translateConfig, string, string, string, func(string)) (string, error) {
		return "", &translateHTTPError{status: 401}
	}
	s.input = "hello"
	a.submitTranslate()
	if s.phase != translateFailed || s.errorKey != "TranslateAuthError" {
		t.Fatalf("failure = %+v", s)
	}
}

func TestTranslateStateGeneration(t *testing.T) {
	a := translateTestApp(t)
	secrets := a.translateSecret.(*fakeTranslateSecrets)
	secrets.value = []byte("k")
	a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
	var first func(string)
	a.translateStreamFn = func(_ context.Context, _ translateConfig, _, _, _ string, onDelta func(string)) (string, error) {
		first = onDelta
		return "old", nil
	}
	s := a.ensureTranslateState()
	s.input = "one"
	a.submitTranslate()
	a.translateStreamFn = func(_ context.Context, _ translateConfig, _, _, _ string, onDelta func(string)) (string, error) {
		onDelta("new")
		return "new", nil
	}
	s.input = "two"
	a.submitTranslate()
	// A delta from the superseded request must not change the result.
	first("late")
	if s.output != "new" || s.phase != translateDone {
		t.Fatalf("late delta changed state: %+v", s)
	}
	a.cancelTranslate()
	first("after cancel")
	if s.output != "new" {
		t.Fatal("canceled request still appended")
	}
}

func TestTranslateCloseShortcuts(t *testing.T) {
	for _, command := range []bool{false, true} {
		a := translateTestApp(t)
		s := a.ensureTranslateState()
		s.visible, s.input, s.output = true, "hello", "你好"
		tester := ui.NewTester(a.translateView, 480, 360)
		tester.Frame()
		if command {
			tester.Key(ui.Cmd, ui.KeyW)
		} else {
			tester.Key(0, ui.KeyEscape)
		}
		if a.translate != nil || s.visible {
			t.Fatal("close shortcut retained the tool window")
		}
	}
}

func TestTranslateDirectionSwap(t *testing.T) {
	a := translateTestApp(t)
	s := a.ensureTranslateState()
	s.input = "hello"
	if _, target := a.translateDirection(s); target != "zh-Hans" {
		t.Fatalf("default target = %q", target)
	}
	a.swapTranslateDirection()
	if _, target := a.translateDirection(s); target != "en" || !s.directionSwapped {
		t.Fatal("direction did not swap")
	}
	a.swapTranslateDirection()
	if _, target := a.translateDirection(s); target != "zh-Hans" || s.directionSwapped {
		t.Fatal("direction did not swap back")
	}
	if a.config.TranslateAutoSpeak != true {
		t.Fatal("swapping wrote a preference")
	}
}

func TestTranslateHeight(t *testing.T) {
	a := translateTestApp(t)
	tk := tokens(false, "zh-CN")
	for _, phase := range []translatePhase{translateEmpty, translateLoading, translateStreaming, translateDone, translateFailed, translateNotConfigured} {
		state := &translateState{phase: phase, input: "hello"}
		if height := a.translateHeight(tk, state); height < tk.SizeTranslateMinHeight || height > tk.SizeTranslateMaxHeight {
			t.Fatalf("phase %v content height = %v", phase, height)
		}
	}
	state := &translateState{phase: translateStreaming, input: "a\nb\nc\nd", output: strings.Repeat("long output ", 1000), dictionary: dictionaryTestDefinition, dictionaryExpanded: true}
	if height := a.translateHeight(tk, state); height != tk.SizeTranslateMaxHeight {
		t.Fatalf("long content height = %v", height)
	}
	if dictionaryBlockHeight(tk, true)-dictionaryBlockHeight(tk, false) != dictionaryBodyHeight(tk) {
		t.Fatal("folding did not release the preview height")
	}
}

func TestSettingsTranslatePageHeight(t *testing.T) {
	a := translateTestApp(t)
	tk := tokens(false, "zh-CN")
	if got := translatePageHeight(tk, true); got != 444 {
		t.Fatalf("translate page content = %v; want 444", got)
	}
	if got := translatePageHeight(tk, false); got != 144 {
		t.Fatalf("closed translate page = %v; want 144", got)
	}
	if got := pageHeight(tk, settingsTranslate, true, false, false, false, false); got != 444 {
		t.Fatalf("pageHeight = %v", got)
	}
	if w, h := settingsMinimumSize(); w != 720 || h != settingsWindowHeight {
		t.Fatalf("minimum window = %dx%d; want 720x%d", w, h, settingsWindowHeight)
	}
	if got := sidebarNaturalHeight(tk); got != 409 {
		t.Fatalf("sidebar = %v; want 409", got)
	}
	if settingsNav[2].page != settingsJSON || settingsNav[3].page != settingsTranslate ||
		settingsNav[4].page != settingsQR || settingsNav[5].page != settingsShortcuts {
		t.Fatalf("nav order = %+v", settingsNav)
	}
	if settingsGeneral != 0 || settingsAbout != 1 || settingsAwake != 2 || settingsShortcuts != 3 || settingsJSON != 4 {
		t.Fatal("existing page numbers moved")
	}
	if got := settingsPageName(settingsTranslate); got != "translate" {
		t.Fatalf("page name = %q", got)
	}
	if got, ok := settingsPageIndex("translate", a); !ok || got != settingsTranslate {
		t.Fatalf("CLI page index = %d %v", got, ok)
	}
	l := newSettingsLayout(a, settingsTranslate)
	if l.height(tk) != translatePageHeight(tk, a.config.TranslateEnabled) {
		t.Fatal("layout height disagrees with the view")
	}
	// The other pages keep their natural heights.
	if got := settingsNaturalHeight(tk, settingsShortcuts, true, false, false, false, false); got != 380 {
		t.Fatalf("shortcuts natural height = %d", got)
	}
	if got := settingsNaturalHeight(tk, settingsGeneral, true, true, true, true, false); got != 388 {
		t.Fatalf("general five rows natural height = %d", got)
	}
}

func TestTranslateSecretStore(t *testing.T) {
	a := translateTestApp(t)
	secrets := a.translateSecret.(*fakeTranslateSecrets)
	a.translateKeyDraft = "  secret  "
	a.setTranslateAPIKey(a.translateKeyDraft)
	if string(secrets.value) != "secret" || secrets.sets != 1 || !a.translateKeySaved || a.translateKeyDraft != "" {
		t.Fatalf("key state = %q %+v", secrets.value, a)
	}
	if key, err := a.translateAPIKey(); err != nil || key != "secret" {
		t.Fatalf("read key = %q, %v", key, err)
	}
	// The saved key never reaches the config file.
	a.save()
	data, err := os.ReadFile(a.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), "translate_api_key") {
		t.Fatalf("key leaked into config: %s", data)
	}
	a.clearTranslateKeyAndDraft()
	if secrets.value != nil || a.translateKeySaved || secrets.deletes != 1 {
		t.Fatal("clear did not remove the key")
	}
	secrets.getErr = errors.New("keychain unavailable")
	if _, err := a.translateAPIKey(); err == nil {
		t.Fatal("keychain failure was swallowed")
	}
}

func TestTranslateStatesRender(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			a := translateTestApp(t)
			s := a.ensureTranslateState()
			checks := []struct {
				phase translatePhase
				setup func()
				keys  []string
			}{
				{translateEmpty, func() {}, []string{"TranslateEmpty", "TranslateToChinese"}},
				{translateLoading, func() {}, []string{"TranslateBusy"}},
				{translateStreaming, func() { s.output = "你好" }, []string{"TranslateCopy"}},
				{translateDone, func() { s.output = "你好，世界" }, []string{"TranslateCopy"}},
				{translateFailed, func() { s.errorKey = "TranslateNetworkError" }, []string{"TranslateNetworkError", "TranslateRetry"}},
				{translateNotConfigured, func() {}, []string{"TranslateNotConfigured", "TranslateOpenSettings"}},
			}
			for _, check := range checks {
				s.phase = check.phase
				s.input = "hello"
				s.output = ""
				s.errorKey = ""
				check.setup()
				tk := tokens(dark, locale)
				height := int(a.translateHeight(tk, s))
				tt := ui.NewTester(a.translateView, int(tk.SizeTranslateWidth), height)
				tt.SetLocale(locale)
				tt.SetDark(dark)
				tt.Frame()
				for _, key := range check.keys {
					if _, ok := tt.Find(a.text(locale, key)); !ok {
						t.Fatalf("%s dark=%v phase=%v: missing %q: %v", locale, dark, check.phase, key, tt.Texts())
					}
				}
				if _, ok := tt.Find(a.text(locale, "R07Pin")); !ok {
					t.Fatalf("missing pin button: %v", tt.Texts())
				}
			}
		}
	}
}

func TestTranslateCopyAndRetry(t *testing.T) {
	a := translateTestApp(t)
	locale := "en"
	s := a.ensureTranslateState()
	s.input, s.output, s.phase = "hello", "你好，世界", translateDone
	copied := ""
	a.translateCopy = func(value string) error { copied = value; return nil }
	tk := tokens(false, locale)
	tt := ui.NewTester(a.translateView, int(tk.SizeTranslateWidth), int(a.translateHeight(tk, s)))
	tt.SetLocale(locale)
	tt.Frame()
	if err := tt.Click(a.text(locale, "TranslateCopy")); err != nil {
		t.Fatalf("copy button: %v", err)
	}
	if copied != "你好，世界" || s.copiedUntil.IsZero() {
		t.Fatalf("copy did not run: %q", copied)
	}
	secrets := a.translateSecret.(*fakeTranslateSecrets)
	secrets.value = []byte("k")
	a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
	a.translateStreamFn = func(context.Context, translateConfig, string, string, string, func(string)) (string, error) {
		return "retried", nil
	}
	s.phase, s.errorKey, s.output = translateFailed, "TranslateNetworkError", ""
	tt.Frame()
	if err := tt.Click(a.text(locale, "TranslateRetry")); err != nil {
		t.Fatalf("retry button: %v", err)
	}
	if s.phase != translateDone || s.output != "retried" {
		t.Fatalf("retry did not resubmit: %+v", s)
	}
}

func TestTranslateSettingsRowsRender(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			a := translateTestApp(t)
			a.page = settingsTranslate
			tt := ui.NewTester(a.view, 720, settingsWindowHeight)
			tt.SetLocale(locale)
			tt.SetDark(dark)
			tt.Frame()
			for _, key := range []string{"Translate", "TranslatePageDesc", "TranslateBaseURL", "TranslateEndpointLocalHint",
				"TranslateAPIKey", "TranslateKeyHint", "TranslateModel", "TranslateAutoSpeak", "TranslateAutoSpeakHint",
				"TranslateShortcutLabel"} {
				if !tt.HasText(a.text(locale, key)) {
					t.Fatalf("%s dark=%v: missing %q: %v", locale, dark, key, tt.Texts())
				}
			}
		}
	}
}

func TestTranslateEndpointHTTPRejectedInline(t *testing.T) {
	a := translateTestApp(t)
	a.page = settingsTranslate
	// The field edits a draft, so a case that types an address sets the draft.
	a.translateBaseURLDraft, a.translateBaseURLDirty = "http://example.com/v1", true
	a.commitTranslateBaseURL()
	if a.translateRowError != "TranslateEndpointHTTPRejected" {
		t.Fatalf("save did not report the rejection: %q", a.translateRowError)
	}
	tt := ui.NewTester(a.view, 720, settingsWindowHeight)
	tt.SetLocale("en")
	tt.Frame()
	if _, ok := tt.Find(a.text("en", "TranslateEndpointHTTPRejected")); !ok {
		t.Fatalf("no inline error: %v", tt.Texts())
	}
	// A valid address clears it.
	a.translateBaseURLDraft, a.translateBaseURLDirty = "https://h/v1", true
	a.commitTranslateBaseURL()
	if a.translateRowError != "" {
		t.Fatalf("valid address kept the error: %q", a.translateRowError)
	}
}

func TestTranslateModelListAndProbe(t *testing.T) {
	a := translateTestApp(t)
	secrets := a.translateSecret.(*fakeTranslateSecrets)
	secrets.value = []byte("k")
	a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "manual"
	a.translateModelsFn = func(_ context.Context, cfg translateConfig) ([]string, error) {
		if cfg.Key != "k" {
			t.Fatalf("model list lost the key: %+v", cfg)
		}
		return []string{"group/one", "provider/two"}, nil
	}
	a.translateTestFn = func(context.Context, translateConfig) (string, error) { return "", nil }
	a.fetchTranslateModelList()
	if len(a.translateModelList) != 2 || a.translateModelListError != "" || a.translateModelFetching {
		t.Fatalf("model list = %+v", a)
	}
	a.setTranslateModel("provider/two")
	if a.config.TranslateModel != "provider/two" {
		t.Fatal("model with a slash was rewritten")
	}
	a.testTranslateEndpoint()
	if a.translateTestKey != "TranslateTestOK" || a.translateTestRunning || time.Now().After(a.translateTestUntil) {
		t.Fatalf("probe = %+v", a)
	}
	a.translateModelsFn = func(context.Context, translateConfig) ([]string, error) {
		return nil, errors.New("unavailable")
	}
	a.fetchTranslateModelList()
	if a.translateModelListError != "TranslateModelFetchFailed" || a.config.TranslateModel != "provider/two" {
		t.Fatalf("failed list cleared manual model: %+v", a)
	}
	// A rejected address reports the same key from the connection test.
	a.config.TranslateBaseURL = "http://example.com/v1"
	a.testTranslateEndpoint()
	if a.translateTestKey != "TranslateEndpointHTTPRejected" || a.translateRowError != "TranslateEndpointHTTPRejected" {
		t.Fatalf("rejected address probe = %q / %q", a.translateTestKey, a.translateRowError)
	}
}

func TestTranslateAutoSpeakPreference(t *testing.T) {
	a := translateTestApp(t)
	if !a.config.TranslateAutoSpeak {
		t.Fatal("default is off")
	}
	a.changeTranslateAutoSpeak(false)
	if a.config.TranslateAutoSpeak {
		t.Fatal("explicit off was not stored")
	}
	data, err := os.ReadFile(a.path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "translate_auto_speak = false") {
		t.Fatalf("preference not persisted: %s", data)
	}
}

func TestTranslateModelListDropdown(t *testing.T) {
	a := translateTestApp(t)
	secrets := a.translateSecret.(*fakeTranslateSecrets)
	secrets.value = []byte("k")
	a.page = settingsTranslate
	a.config.TranslateBaseURL = "https://h/v1"
	a.config.TranslateModel = "manual"
	a.translateModelsFn = func(context.Context, translateConfig) ([]string, error) {
		return []string{"group/one", "provider/two"}, nil
	}
	tt := ui.NewTester(a.view, 720, settingsWindowHeight)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click(a.text("en", "TranslateModelFetch")); err != nil {
		t.Fatalf("model list trigger: %v", err)
	}
	if len(a.translateModelList) != 2 {
		t.Fatalf("list was not fetched: %+v", a.translateModelList)
	}
	tt.Frame()
	if err := tt.Click("provider/two"); err != nil {
		t.Fatalf("model item: %v; texts %v", err, tt.Texts())
	}
	if a.config.TranslateModel != "provider/two" {
		t.Fatalf("selected model = %q", a.config.TranslateModel)
	}
	// Manual typing stays available.
	a.translateModelDraft, a.translateModelDirty = "hand/typed", true
	a.commitTranslateModel()
	if a.config.TranslateModel != "hand/typed" {
		t.Fatal("manual model was rewritten")
	}
}

package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// translateCommitApp is a headless app on the Translate settings page, with a
// fake keychain and no network: every case here is about when a draft is
// written, never about what the endpoint answers.
func translateCommitApp(t *testing.T) *application {
	t.Helper()
	a := translateTestApp(t)
	a.page = settingsTranslate
	return a
}

// commitSettings frames the real settings view and returns the tester.
func commitSettings(a *application, locale string) *ui.Tester {
	tt := ui.NewTester(a.view, 720, settingsWindowHeight)
	tt.SetLocale(locale)
	tt.Frame()
	return tt
}

// savedConfig reads the file back, so a case can prove a value really reached
// the disk rather than only the in-memory config.
func savedConfig(t *testing.T, a *application) Config {
	t.Helper()
	config, err := loadConfig(a.path)
	if err != nil {
		t.Fatal(err)
	}
	return config
}

// focusTranslateField tabs through the page until the named input has the
// keyboard. Tab order is the page's own and does not depend on the locale's
// text widths, unlike a click at a guessed coordinate.
func focusTranslateField(t *testing.T, tt *ui.Tester, a *application, field string) {
	t.Helper()
	for step := 0; step < 24; step++ {
		if a.translateFieldFocused[field] {
			return
		}
		tt.Key(0, ui.KeyTab)
		tt.Frame()
	}
	t.Fatalf("%s never took the focus: %v", field, a.translateFieldFocused)
}

// blurTranslateFields clicks a row title, which is not a control, so whatever
// had the keyboard loses it.
func blurTranslateFields(t *testing.T, tt *ui.Tester, a *application, locale string) {
	t.Helper()
	title, ok := tt.Find(a.text(locale, "TranslateModel"))
	if !ok {
		t.Fatalf("missing the model row: %v", tt.Texts())
	}
	tt.ClickAt(title.X+10, title.Y+title.H/2)
	tt.Frame()
}

// TestTranslateAddressCommitsOnBlur is the maintainer's regression: typing an
// address and clicking away must save it without Return.
func TestTranslateAddressCommitsOnBlur(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a := translateCommitApp(t)
			tt := commitSettings(a, locale)
			focusTranslateField(t, tt, a, translateFieldBaseURL)
			tt.Type("https://example.test/v1")
			tt.Frame()
			blurTranslateFields(t, tt, a, locale)
			if got := savedConfig(t, a).TranslateBaseURL; got != "https://example.test/v1" {
				t.Fatalf("blur did not save the address: %q", got)
			}
			if a.translateRowError != "" {
				t.Fatalf("a valid address reported %q", a.translateRowError)
			}
		})
	}
}

// TestTranslateKeyCommitsOnBlur covers the reported bug directly: a pasted key
// must be saved when the field loses the focus, not only on Return.
func TestTranslateKeyCommitsOnBlur(t *testing.T) {
	a := translateCommitApp(t)
	// The endpoint and model are already configured; only the key is missing,
	// which is the state the report describes.
	a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
	a.translateBaseURLDraft, a.translateModelDraft = "https://h/v1", "m"
	if !a.translateNotConfigured() {
		t.Fatal("the fixture should start unconfigured")
	}
	secrets := a.translateSecret.(*fakeTranslateSecrets)
	tt := commitSettings(a, "en")
	focusTranslateField(t, tt, a, translateFieldKey)
	tt.Type("pasted-key")
	tt.Frame()
	blurTranslateFields(t, tt, a, "en")
	if string(secrets.value) != "pasted-key" || secrets.sets != 1 {
		t.Fatalf("blur did not save the key: %q, %d writes", secrets.value, secrets.sets)
	}
	if !a.translateKeySaved || a.translateKeyEditing || a.translateKeyDraft != "" {
		t.Fatalf("the row did not collapse after the blur save: %+v", a)
	}
	if a.translateNotConfigured() {
		t.Fatal("a saved key still read as unconfigured")
	}
}

// TestTranslateKeyEmptyBlurKeepsSavedKey is the rule the order names: only the
// Clear button deletes a key. An empty draft that loses the focus does not.
func TestTranslateKeyEmptyBlurKeepsSavedKey(t *testing.T) {
	a := translateCommitApp(t)
	secrets := a.translateSecret.(*fakeTranslateSecrets)
	secrets.value, a.translateKeySaved = []byte("existing"), true
	tt := commitSettings(a, "en")
	if err := tt.Click(a.text("en", "TranslateKeyReplace")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	blurTranslateFields(t, tt, a, "en")
	if string(secrets.value) != "existing" || secrets.deletes != 0 || secrets.sets != 0 {
		t.Fatalf("an empty blur touched the keychain: %q deletes=%d sets=%d", secrets.value, secrets.deletes, secrets.sets)
	}
	if !a.translateKeySaved {
		t.Fatal("an empty blur cleared the saved state")
	}
}

// TestTranslateModelCommitsOnBlur keeps the third field on the same rule.
func TestTranslateModelCommitsOnBlur(t *testing.T) {
	a := translateCommitApp(t)
	tt := commitSettings(a, "en")
	focusTranslateField(t, tt, a, translateFieldModel)
	tt.Type("group/model")
	tt.Frame()
	blurTranslateFields(t, tt, a, "en")
	if got := savedConfig(t, a).TranslateModel; got != "group/model" {
		t.Fatalf("blur did not save the model: %q", got)
	}
}

// TestTranslateUnchangedBlurWritesNothing keeps a blur from rewriting a value
// the user never changed.
func TestTranslateUnchangedBlurWritesNothing(t *testing.T) {
	a := translateCommitApp(t)
	a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
	a.translateBaseURLDraft, a.translateModelDraft = "https://h/v1", "m"
	empty := "version = 1\nlanguage = \"zh-CN\"\n"
	if err := os.WriteFile(a.path, []byte(empty), 0o600); err != nil {
		t.Fatal(err)
	}
	tt := commitSettings(a, "en")
	focusTranslateField(t, tt, a, translateFieldBaseURL)
	blurTranslateFields(t, tt, a, "en")
	body, err := os.ReadFile(a.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != empty {
		t.Fatalf("an unchanged blur wrote the file: %s", body)
	}
}

// TestTranslateRejectedAddressKeepsDraft proves the rejected address is not
// written, stays on screen, and shows the existing inline error.
func TestTranslateRejectedAddressKeepsDraft(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a := translateCommitApp(t)
			tt := commitSettings(a, locale)
			focusTranslateField(t, tt, a, translateFieldBaseURL)
			tt.Type("http://example.com/v1")
			tt.Frame()
			blurTranslateFields(t, tt, a, locale)
			if a.translateRowError != "TranslateEndpointHTTPRejected" {
				t.Fatalf("blur did not report the rejection: %q", a.translateRowError)
			}
			if got := savedConfig(t, a).TranslateBaseURL; got != "" {
				t.Fatalf("a rejected address reached the file: %q", got)
			}
			if a.translateBaseURLDraft != "http://example.com/v1" {
				t.Fatalf("the rejected draft was replaced under the user: %q", a.translateBaseURLDraft)
			}
			if _, ok := tt.Find(a.text(locale, "TranslateEndpointHTTPRejected")); !ok {
				t.Fatalf("missing inline error: %v", tt.Texts())
			}
		})
	}
}

// TestTranslateDraftsCommitOnEveryTrigger covers the four triggers the order
// lists when no element-level blur exists: the window losing focus, a page
// switch, closing the window, and the connection test.
func TestTranslateDraftsCommitOnEveryTrigger(t *testing.T) {
	triggers := []struct {
		name string
		run  func(*application, *ui.Tester)
	}{
		{"window blur", func(_ *application, tt *ui.Tester) { tt.SetFocused(false) }},
		{"page switch", func(a *application, _ *ui.Tester) { a.selectSettingsPage(settingsGeneral) }},
		{"window close", func(a *application, _ *ui.Tester) { a.hide() }},
		{"connection test", func(a *application, _ *ui.Tester) { a.testTranslateEndpoint() }},
	}
	for _, trigger := range triggers {
		t.Run(trigger.name, func(t *testing.T) {
			a := translateCommitApp(t)
			secrets := a.translateSecret.(*fakeTranslateSecrets)
			tt := commitSettings(a, "en")
			focusTranslateField(t, tt, a, translateFieldBaseURL)
			tt.Type("https://example.test/v1")
			tt.Frame()
			focusTranslateField(t, tt, a, translateFieldKey)
			tt.Type("pasted-key")
			tt.Frame()
			trigger.run(a, tt)
			tt.Frame()
			if got := savedConfig(t, a).TranslateBaseURL; got != "https://example.test/v1" {
				t.Fatalf("address not saved on %s: %q", trigger.name, got)
			}
			if string(secrets.value) != "pasted-key" {
				t.Fatalf("key not saved on %s: %q", trigger.name, secrets.value)
			}
		})
	}
}

// TestTranslatePanelCommitsDraftBeforeConfiguredCheck is item 2: opening the
// floating panel must commit the draft before it decides the module is
// configured, or the user sees the "not configured" prompt about a key they
// already pasted.
func TestTranslatePanelCommitsDraftBeforeConfiguredCheck(t *testing.T) {
	a := translateCommitApp(t)
	a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
	a.translateBaseURLDraft, a.translateModelDraft = "https://h/v1", "m"
	a.translateKeyDraft = "pasted-key"
	var requests int
	a.translateStreamFn = func(context.Context, translateConfig, string, string, string, func(string)) (string, error) {
		requests++
		return "你好", nil
	}
	a.openTranslateText("hello")
	if requests != 1 {
		t.Fatalf("the panel did not translate with the draft key: %d requests", requests)
	}
	if s := a.translate; s.phase != translateDone || s.output != "你好" {
		t.Fatalf("panel = %+v", s)
	}
	if a.translateKeyDraft != "" {
		t.Fatal("the draft stayed in the field after it was committed")
	}
}

// TestTranslateTestButtonIsNamed is item 4: the connection test is a worded
// button, in both languages, with the result still beside it.
func TestTranslateTestButtonIsNamed(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a := translateCommitApp(t)
			a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
			a.translateBaseURLDraft, a.translateModelDraft = "https://h/v1", "m"
			a.translateTestFn = func(context.Context, translateConfig) (string, error) { return "", nil }
			a.translateSecret.(*fakeTranslateSecrets).value = []byte("k")
			tt := commitSettings(a, locale)
			label := a.text(locale, "TranslateTest")
			bounds, ok := tt.Find(label)
			if !ok {
				t.Fatalf("the test button has no word: %v", tt.Texts())
			}
			if bounds.W < 28 {
				t.Fatalf("the test button is not wide enough for its word: %+v", bounds)
			}
			if bounds.X+bounds.W > 696 {
				t.Fatalf("the test button overflows the card: %+v", bounds)
			}
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if a.translateTestKey != "TranslateTestOK" {
				t.Fatalf("the named button did not test the endpoint: %q", a.translateTestKey)
			}
			if !tt.HasText(a.text(locale, "TranslateTest")) {
				t.Fatal("the button lost its word while showing a result")
			}
			tt.Advance(3e9)
			tt.Frame()
			if a.translateTestKey != "" {
				t.Fatalf("the result did not clear: %q", a.translateTestKey)
			}
		})
	}
}

// TestTranslatePageStillFitsTheWindow keeps the shared 444 floor: the named
// button must not grow the translate page.
func TestTranslatePageStillFitsTheWindow(t *testing.T) {
	tk := tokens(false, "zh-CN")
	if got := translatePageHeight(tk, true); got != 444 {
		t.Fatalf("translate page = %v; want 444", got)
	}
	if _, height := settingsMinimumSize(); height != settingsWindowHeight {
		t.Fatalf("window minimum = %d; want %d", height, settingsWindowHeight)
	}
}

// translateNotConfigured reports whether the panel would show the "not
// configured" prompt right now, which is the check the report is about.
func (a *application) translateNotConfigured() bool {
	key, err := a.translateAPIKey()
	if err != nil {
		return true
	}
	cfg := a.currentTranslateConfig()
	return strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Model) == "" || key == ""
}

// TestTranslateEmptyAddressIsNotAnError keeps an emptied field from raising a
// network error: it is simply "not configured".
func TestTranslateEmptyAddressIsNotAnError(t *testing.T) {
	a := translateCommitApp(t)
	a.config.TranslateBaseURL = "https://h/v1"
	a.translateBaseURLDraft, a.translateBaseURLDirty = "", true
	a.commitTranslateBaseURL()
	if a.translateRowError != "" {
		t.Fatalf("an empty address reported %q", a.translateRowError)
	}
	if a.config.TranslateBaseURL != "" {
		t.Fatalf("an emptied address stayed saved: %q", a.config.TranslateBaseURL)
	}
}

// TestTranslateRejectedAddressSurvivesAnotherSave proves the bad draft cannot
// reach the file through a later save of something else.
func TestTranslateRejectedAddressSurvivesAnotherSave(t *testing.T) {
	a := translateCommitApp(t)
	a.translateBaseURLDraft, a.translateBaseURLDirty = "http://example.com/v1", true
	a.commitTranslateBaseURL()
	a.changeTranslateAutoSpeak(false)
	if got := savedConfig(t, a).TranslateBaseURL; got != "" {
		t.Fatalf("a rejected draft reached the file: %q", got)
	}
	if a.translateBaseURLDraft != "http://example.com/v1" {
		t.Fatalf("the rejected draft was lost: %q", a.translateBaseURLDraft)
	}
}

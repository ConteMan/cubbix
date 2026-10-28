package main

import (
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLoginFailureRollsBack(t *testing.T) {
	app, err := newApplication(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var calls []bool
	app.setLogin = func(enabled bool) error {
		calls = append(calls, enabled)
		if enabled {
			return errors.New("approval required")
		}
		return nil
	}
	app.config.OpenAtLogin = true
	app.changeLogin()
	if len(calls) != 2 || !calls[0] || calls[1] || app.config.OpenAtLogin || app.settingsError != "LoginError" {
		t.Fatalf("calls=%v config=%+v error=%q", calls, app.config, app.settingsError)
	}
}

func TestSettingsNavigationAndLocale(t *testing.T) {
	app, err := newApplication(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	tester := ui.NewTester(app.view, 720, 176)
	tester.SetLocale("zh-CN")
	if !tester.HasText("语言") {
		t.Fatalf("missing general page: %v", tester.Texts())
	}
	if err := tester.Click("关于"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("自动更新") {
		t.Fatalf("missing about page: %v", tester.Texts())
	}
	if err := tester.Click("检查更新"); err != nil {
		t.Fatal(err)
	}
	if !app.updateError {
		t.Fatal("update check did not report missing source")
	}
	tester.SetLocale("en")
	if !tester.HasText("Automatic updates") {
		t.Fatalf("missing English page: %v", tester.Texts())
	}
	tester.SetDark(true)
	tester.Frame()
}

func TestLanguageAndPreferencesPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	app, err := newApplication(path)
	if err != nil {
		t.Fatal(err)
	}
	var selectedLocale string
	app.setLocale = func(locale string) { selectedLocale = locale }
	tester := ui.NewTester(app.view, 720, 176)
	tester.SetLocale("zh-CN")
	if err := tester.Click("English"); err != nil {
		t.Fatal(err)
	}
	if selectedLocale != "en" || app.config.Language != "en" {
		t.Fatalf("locale=%q config=%q", selectedLocale, app.config.Language)
	}
	if err := tester.Click("关于"); err != nil {
		t.Fatal(err)
	}
	tester.SetSize(720, settingsHeight(tokens(false, "zh-CN"), 1, false))
	tester.ClickAt(672, 170)
	loaded, err := loadConfig(path)
	if err != nil || loaded.Language != "en" || loaded.AutomaticUpdates {
		t.Fatalf("config=%+v err=%v", loaded, err)
	}
}

func TestSettingsHeight(t *testing.T) {
	tokens := tokens(false, "zh-CN")
	if got := settingsHeight(tokens, 0, false); got != 176 {
		t.Fatalf("general height = %d", got)
	}
	if got := settingsHeight(tokens, 1, false); got != 220 {
		t.Fatalf("about height = %d", got)
	}
	if got := settingsHeight(tokens, 1, true); got != 244 {
		t.Fatalf("about with error height = %d", got)
	}
}

func TestPreview(t *testing.T) {
	path := os.Getenv("CUBBIX_TEST_PREVIEW")
	if path == "" {
		t.Skip("preview not requested")
	}
	app, err := newApplication(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	tester := ui.NewTester(app.view, 720, 176)
	tester.SetLocale("zh-CN")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tester.Image()); err != nil {
		t.Fatal(err)
	}
}

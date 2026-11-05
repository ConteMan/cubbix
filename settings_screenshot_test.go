package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// evidencePaths gives one render its own link and CLI binary, under a temporary
// directory, so no state leaks into the next one or into the real home.
func evidencePaths(t *testing.T) cliPaths {
	t.Helper()
	dir := t.TempDir()
	resources := filepath.Join(dir, "resources")
	if err := os.MkdirAll(filepath.Join(resources, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(resources, "bin", "cubbix")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	paths := cliPaths{
		link:   filepath.Join(dir, "home", ".local", "bin", "cubbix"),
		target: target,
		bundle: filepath.Join(dir, "Cubbix.app"),
	}
	cliPathsFor = func() (cliPaths, bool) { return paths, true }
	t.Cleanup(func() { cliPathsFor = func() (cliPaths, bool) { return cliPaths{}, false } })
	return paths
}

// TestModuleSettingsScreenshots renders every page state of the module
// settings in both languages and both themes. It is an in-memory render of the
// real view: no window is created, nothing is activated, no shortcut is
// registered. CUBBIX_SETTINGS_EVIDENCE names the output directory, which stays
// outside the repository.
func TestModuleSettingsScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_SETTINGS_EVIDENCE")
	if dir == "" {
		t.Skip("settings evidence not requested")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	capture := func(name, locale string, dark bool, prepare func(a *application, paths cliPaths), hover func(a *application) string) {
		t.Helper()
		a := awakeTestApp(t)
		paths := evidencePaths(t)
		if locale == "en" {
			a.language = 2
		} else {
			a.language = 1
		}
		if prepare != nil {
			prepare(a, paths)
		}
		height := settingsHeight(tokens(dark, locale), a.page, a.config.AwakeEnabled, false, false, true, true)
		tt := ui.NewTester(a.view, 720, height)
		tt.SetScale(2)
		tt.SetLocale(locale)
		tt.SetDark(dark)
		tt.Frame()
		// Let the 140 ms navigation colour transition settle before drawing.
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
		if hover != nil {
			if label := hover(a); label != "" {
				if rect, ok := tt.Find(label); ok {
					tt.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
					tt.Advance(700 * time.Millisecond)
					tt.Frame()
				}
			}
		}
		mode := "light"
		if dark {
			mode = "dark"
		}
		file, err := os.Create(filepath.Join(dir, fmt.Sprintf("settings-%s-%s-%s.png", name, locale, mode)))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, tt.Image()); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			// General: the switch in every link state, including the two that
			// carry a red outline alert.
			for _, state := range []string{"missing", "installed", "conflict", "broken", "translocated"} {
				state := state
				capture("general-cli-"+state, locale, dark, func(a *application, paths cliPaths) {
					a.page = settingsGeneral
					switch state {
					case "installed":
						_ = installCLILink(paths)
					case "conflict":
						_ = os.MkdirAll(filepath.Dir(paths.link), 0o755)
						_ = os.WriteFile(paths.link, []byte("x"), 0o600)
					case "broken":
						_ = installCLILink(paths)
						_ = os.Remove(paths.target)
					case "translocated":
						translocated := paths
						translocated.bundle = "/private/var/folders/xy/AppTranslocation/1234/d/Cubbix.app"
						cliPathsFor = func() (cliPaths, bool) { return translocated, true }
					}
				}, nil)
			}
			// Awake module: open, and closed with its released shortcut.
			capture("awake-on", locale, dark, func(a *application, paths cliPaths) {
				a.page = settingsAwake
			}, nil)
			capture("awake-off", locale, dark, func(a *application, paths cliPaths) {
				a.page = settingsAwake
				a.config.AwakeEnabled = false
			}, nil)
			capture("awake-off-reason", locale, dark, func(a *application, paths cliPaths) {
				a.page = settingsAwake
				a.config.AwakeEnabled = false
			}, func(a *application) string {
				return fmt.Sprintf(a.text(locale, "ModuleShortcutOff"), a.text(locale, "Awake"))
			})
			// JSON module: open and closed.
			capture("json-on", locale, dark, func(a *application, paths cliPaths) { a.page = settingsJSON }, nil)
			capture("json-off", locale, dark, func(a *application, paths cliPaths) {
				a.page = settingsJSON
				a.config.JSONEnabled = false
			}, nil)
			// Shortcuts page: both groups, plus a level-3 tooltip.
			capture("shortcuts", locale, dark, func(a *application, paths cliPaths) {
				a.page = settingsShortcuts
			}, nil)
			capture("shortcuts-info", locale, dark, func(a *application, paths cliPaths) {
				a.page = settingsShortcuts
			}, func(a *application) string { return a.text(locale, "FeedbackModeInfo") })
			// About page: current, a failed check, and an update ready.
			capture("about", locale, dark, func(a *application, paths cliPaths) {
				a.page = settingsAbout
			}, nil)
			capture("about-error", locale, dark, func(a *application, paths cliPaths) {
				a.page = settingsAbout
				a.updates.mu.Lock()
				a.updates.state.ErrorKey = "UpdateNetworkError"
				a.updates.mu.Unlock()
			}, nil)
			capture("about-ready", locale, dark, func(a *application, paths cliPaths) {
				a.page = settingsAbout
				a.updates.mu.Lock()
				a.updates.state.Phase = updateReady
				a.updates.mu.Unlock()
			}, nil)
			// The note page keeps its scroll and its back row.
			capture("about-notes", locale, dark, func(a *application, paths cliPaths) {
				a.page, a.notesPage = settingsAbout, true
			}, nil)
		}
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// settingsTestPaths points the row at a temporary home and resources directory:
// the real ~/.local/bin is never touched.
func settingsTestPaths(t *testing.T) (cliPaths, *application) {
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
	original := cliPathsFor
	cliPathsFor = func() (cliPaths, bool) { return paths, true }
	t.Cleanup(func() { cliPathsFor = original })
	return paths, cliTestApp(t)
}

func TestCLILinkInstallAndRemove(t *testing.T) {
	paths, _ := settingsTestPaths(t)
	if state := paths.status(); state != cliMissing {
		t.Fatalf("fresh state = %v", state)
	}
	if err := installCLILink(paths); err != nil {
		t.Fatal(err)
	}
	if state := paths.status(); state != cliInstalled {
		t.Fatalf("installed state = %v", state)
	}
	dest, err := os.Readlink(paths.link)
	if err != nil || dest != paths.target {
		t.Fatalf("link = %q %v", dest, err)
	}
	if info, err := os.Stat(paths.link); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("the link does not resolve to the binary: %v", err)
	}
	if err := installCLILink(paths); err != nil {
		t.Fatalf("installing twice: %v", err)
	}
	if err := removeCLILink(paths); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(paths.link); !os.IsNotExist(err) {
		t.Fatalf("the link is still there: %v", err)
	}
	if err := removeCLILink(paths); err == nil {
		t.Fatal("removing a missing link reported success")
	}
}

func TestCLILinkNeverReplacesForeignFiles(t *testing.T) {
	paths, _ := settingsTestPaths(t)
	if err := os.MkdirAll(filepath.Dir(paths.link), 0o755); err != nil {
		t.Fatal(err)
	}
	// A plain file owns the name.
	if err := os.WriteFile(paths.link, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if state := paths.status(); state != cliConflict {
		t.Fatalf("plain file state = %v", state)
	}
	if err := installCLILink(paths); err == nil || err.Error() != "CLILinkConflict" {
		t.Fatalf("install over a file = %v", err)
	}
	if data, _ := os.ReadFile(paths.link); string(data) != "mine" {
		t.Fatal("the file was replaced")
	}
	if err := removeCLILink(paths); err == nil {
		t.Fatal("remove deleted a file that is not ours")
	}

	// A symlink to something else is refused too.
	if err := os.Remove(paths.link); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other")
	if err := os.WriteFile(other, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, paths.link); err != nil {
		t.Fatal(err)
	}
	if state := paths.status(); state != cliConflict {
		t.Fatalf("foreign link state = %v", state)
	}
	if err := installCLILink(paths); err == nil || err.Error() != "CLILinkConflict" {
		t.Fatalf("install over a foreign link = %v", err)
	}
	if dest, _ := os.Readlink(paths.link); dest != other {
		t.Fatal("the foreign link was replaced")
	}
}

func TestCLILinkRepairsBrokenTarget(t *testing.T) {
	paths, _ := settingsTestPaths(t)
	if err := installCLILink(paths); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(paths.target); err != nil {
		t.Fatal(err)
	}
	if state := paths.status(); state != cliBroken {
		t.Fatalf("broken state = %v", state)
	}
	// Reinstalling the binary makes the same link work again.
	if err := os.WriteFile(paths.target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if state := paths.status(); state != cliInstalled {
		t.Fatalf("repaired state = %v", state)
	}
}

func TestCLILinkAbsentAndTranslocated(t *testing.T) {
	paths, _ := settingsTestPaths(t)
	if err := os.Remove(paths.target); err != nil {
		t.Fatal(err)
	}
	if state := paths.status(); state != cliAbsent {
		t.Fatalf("absent state = %v", state)
	}
	if err := installCLILink(paths); err == nil || err.Error() != "CLIInstallFailed" {
		t.Fatalf("install without a binary = %v", err)
	}

	translocated := paths
	translocated.bundle = "/private/var/folders/xy/AppTranslocation/1234/d/Cubbix.app"
	if err := os.WriteFile(translocated.target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if state := translocated.status(); state != cliTranslocated {
		t.Fatalf("translocated state = %v", state)
	}
	if err := installCLILink(translocated); err == nil || err.Error() != "CLITranslocated" {
		t.Fatalf("install while translocated = %v", err)
	}
}

// cliSwitchX is the centre of the switch column of a settings row. The content
// column is 528 wide with a 16 inset, so the switch sits at its right edge.
func cliSwitchX(t tokenValues) float32 {
	return t.SizeWindowWidth - t.Space24 - t.Space16 - t.SizeSwitchWidth/2
}

// The row draws every state as a switch without opening a window.
func TestCLISettingsRowStates(t *testing.T) {
	states := []string{"missing", "installed", "conflict", "broken", "translocated"}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, state := range states {
			paths, a := settingsTestPaths(t)
			a.page = settingsGeneral
			restore := func() {}
			switch state {
			case "installed":
				if err := installCLILink(paths); err != nil {
					t.Fatal(err)
				}
			case "conflict":
				if err := os.MkdirAll(filepath.Dir(paths.link), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(paths.link, []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "broken":
				if err := installCLILink(paths); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(paths.target); err != nil {
					t.Fatal(err)
				}
			case "translocated":
				original := cliPathsFor
				translocated := paths
				translocated.bundle = "/private/var/folders/xy/AppTranslocation/1234/d/Cubbix.app"
				cliPathsFor = func() (cliPaths, bool) { return translocated, true }
				restore = func() { cliPathsFor = original }
			}
			tt := ui.NewTester(a.view, 720, 388)
			tt.SetLocale(locale)
			if _, found := tt.Find(a.text(locale, "CLIName")); !found {
				t.Fatalf("%s/%s: missing row: %v", state, locale, tt.Texts())
			}
			// Both fixed levels of the row are visible: the title and, at level
			// one, the hint that says what the switch builds.
			if !tt.HasText(a.text(locale, "CLILinkHint")) {
				t.Fatalf("%s/%s: missing hint: %v", state, locale, tt.Texts())
			}
			// Only the failing states carry the reason. A missing or installed
			// link is a normal state, not an error.
			wantAlert := state == "broken" || state == "conflict" || state == "translocated"
			reason := map[string]string{
				"broken":       a.text(locale, "CLILinkBroken"),
				"conflict":     strings.ReplaceAll(a.text(locale, "CLILinkConflict"), "%s", paths.link),
				"translocated": a.text(locale, "CLITranslocated"),
			}[state]
			alert := reason != "" && tt.HasText(reason)
			if alert != wantAlert {
				t.Fatalf("%s/%s: alert shown=%v want=%v: %v", state, locale, alert, wantAlert, tt.Texts())
			}
			// Clicking the switch applies its target. Translocated is the only
			// state whose switch cannot be used.
			title, _ := tt.Find(a.text(locale, "CLIName"))
			tt.ClickAt(cliSwitchX(tokens(false, "zh-CN")), title.Y+title.H/2)
			tt.Frame()
			switch state {
			case "missing":
				if paths.status() != cliInstalled {
					t.Fatalf("%s/%s: the switch did not install", state, locale)
				}
			case "installed":
				if paths.status() != cliMissing {
					t.Fatalf("%s/%s: the switch did not remove", state, locale)
				}
			case "conflict":
				// The foreign file survives and the failure reaches the header.
				if data, _ := os.ReadFile(paths.link); string(data) != "x" {
					t.Fatal("the foreign file was replaced")
				}
				if a.settingsError != "CLILinkConflict" {
					t.Fatalf("%s/%s: missing failure: %q", state, locale, a.settingsError)
				}
			case "broken":
				// Removing our broken link hides the row: the target is gone,
				// so the next state is absent rather than missing.
				if _, err := os.Lstat(paths.link); !os.IsNotExist(err) {
					t.Fatalf("%s/%s: the broken link survived", state, locale)
				}
				tt.Frame()
				if _, found := tt.Find(a.text(locale, "CLIName")); found {
					t.Fatalf("%s/%s: the row outlived its target: %v", state, locale, tt.Texts())
				}
			case "translocated":
				if _, err := os.Lstat(paths.link); !os.IsNotExist(err) {
					t.Fatalf("%s/%s: a translocated bundle accepted the click", state, locale)
				}
				if a.settingsError != "" {
					t.Fatalf("%s/%s: the disabled switch reported %q", state, locale, a.settingsError)
				}
			}
			restore()
		}
	}
}

// A bundle without the CLI draws no row at all.
func TestCLISettingsRowHiddenWithoutBinary(t *testing.T) {
	paths, a := settingsTestPaths(t)
	if err := os.Remove(paths.target); err != nil {
		t.Fatal(err)
	}
	a.page = 0
	tt := ui.NewTester(a.view, 720, 340)
	tt.SetLocale("en")
	if tt.HasText(a.text("en", "CLIName")) {
		t.Fatalf("the row was drawn without the binary: %v", tt.Texts())
	}
}

// The install button works and reports failures through the settings error slot.
// The switch installs and removes the link, and a failure stays in the error
// slot instead of flipping the switch.
func TestCLISettingsInstallClick(t *testing.T) {
	paths, a := settingsTestPaths(t)
	a.page = settingsGeneral
	tt := ui.NewTester(a.view, 720, 388)
	tt.SetLocale("en")
	title, ok := tt.Find(a.text("en", "CLIName"))
	if !ok {
		t.Fatal("missing the command line row")
	}
	x := cliSwitchX(tokens(false, "en"))
	tt.ClickAt(x, title.Y+title.H/2)
	tt.Frame()
	if state := paths.status(); state != cliInstalled {
		t.Fatalf("click did not install: %v", state)
	}
	if a.config.PaletteFavorites != nil {
		t.Fatal("the switch wrote unrelated configuration")
	}
	title, _ = tt.Find(a.text("en", "CLIName"))
	tt.ClickAt(x, title.Y+title.H/2)
	tt.Frame()
	if state := paths.status(); state != cliMissing {
		t.Fatalf("click did not remove: %v", state)
	}
}

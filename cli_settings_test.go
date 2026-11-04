package main

import (
	"os"
	"path/filepath"
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

// The row draws every state without opening a window.
func TestCLISettingsRowStates(t *testing.T) {
	for _, state := range []string{"missing", "installed", "conflict", "broken", "translocated"} {
		paths, a := settingsTestPaths(t)
		a.page = 0
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
			t.Cleanup(func() { cliPathsFor = original })
		}
		for _, locale := range []string{"zh-CN", "en"} {
			tt := ui.NewTester(a.view, 720, 340)
			tt.SetLocale(locale)
			if !tt.HasText(a.text(locale, "CLIName")) {
				t.Fatalf("%s: missing row: %v", state, tt.Texts())
			}
			want := map[string]string{
				"missing": "CLIInstall", "installed": "CLIRemove", "conflict": "CLIRepair",
				"broken": "CLIRepair", "translocated": "CLITranslocated",
			}[state]
			if state == "translocated" {
				// The hint sits on the alert's tooltip, which a tester cannot see;
				// the disabled button is the visible part.
				_ = tt.Click(a.text(locale, "CLIInstall"))
				if _, err := os.Lstat(paths.link); !os.IsNotExist(err) {
					t.Fatal("a translocated bundle accepted the install click")
				}
				continue
			}
			if !tt.HasText(a.text(locale, want)) {
				t.Fatalf("%s/%s: missing %s: %v", state, locale, want, tt.Texts())
			}
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
func TestCLISettingsInstallClick(t *testing.T) {
	paths, a := settingsTestPaths(t)
	a.page = 0
	tt := ui.NewTester(a.view, 720, 340)
	tt.SetLocale("en")
	if err := tt.Click(a.text("en", "CLIInstall")); err != nil {
		t.Fatal(err)
	}
	if state := paths.status(); state != cliInstalled {
		t.Fatalf("click did not install: %v", state)
	}
	tt.Frame()
	if err := tt.Click(a.text("en", "CLIRemove")); err != nil {
		t.Fatal(err)
	}
	if state := paths.status(); state != cliMissing {
		t.Fatalf("click did not remove: %v", state)
	}
}

package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestCLIScreenshots renders the command line row of the general page in every
// state, in both languages and themes. Evidence stays outside the repository:
// point CUBBIX_CLI_UI_EVIDENCE at a directory outside it.
func TestCLIScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_CLI_UI_EVIDENCE")
	if dir == "" {
		t.Skip("CLI UI evidence not requested")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			mode := "light"
			if dark {
				mode = "dark"
			}
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
					defer func() { cliPathsFor = original }()
				}
				tt := ui.NewTester(a.view, 720, 340)
				tt.SetScale(2)
				tt.SetLocale(locale)
				tt.SetDark(dark)
				tt.Frame()
				file, err := os.Create(filepath.Join(dir, fmt.Sprintf("cli-%s-%s-%s.png", state, locale, mode)))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, tt.Image())
				file.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

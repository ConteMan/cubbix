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

// These are in-memory renders of the real card panel. No flyout is created,
// no screen is touched and nothing is activated.
func TestMenubarPanelScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_PANEL_UI_EVIDENCE")
	if dir == "" {
		t.Skip("panel UI evidence not requested")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	addWindows := func(a *application, count int) {
		names := []string{"orders.json", "settings.json", "response.json", "items-1.json", "items-2.json", "draft.json", "payload.json"}
		for index := range count {
			path := ""
			if index < len(names) {
				path = names[index]
			}
			d := a.addJSONDocument("", path, true)
			d.pinned = index == 0
		}
	}
	scenarios := []struct {
		name  string
		setup func(a *application)
	}{
		{"panel-off", func(a *application) {}},
		{"panel-30m", func(a *application) {
			if err := a.setDuration(30); err != nil {
				t.Fatal(err)
			}
		}},
		{"panel-1h", func(a *application) {
			if err := a.setDuration(60); err != nil {
				t.Fatal(err)
			}
		}},
		{"panel-unlimited", func(a *application) {
			if err := a.setDuration(0); err != nil {
				t.Fatal(err)
			}
		}},
		{"panel-until", func(a *application) {
			a.config.AwakeUntil = "18:30"
			if err := a.startAwake(awakeSession{Mode: "screen", End: time.Now().Add(time.Hour), Until: "18:30"}); err != nil {
				t.Fatal(err)
			}
		}},
		{"panel-follow", func(a *application) {
			a.config.AwakeMode = "tasks"
			if err := a.startAwake(awakeSession{Mode: "tasks", PID: 4242, Identity: "sample", Name: "codex"}); err != nil {
				t.Fatal(err)
			}
		}},
		{"panel-error", func(a *application) {
			a.awake.mu.Lock()
			a.awake.state = awakeState{Error: "ProcessEnded"}
			a.awake.mu.Unlock()
		}},
		{"panel-json-0", func(a *application) {
			a.config.AwakeEnabled = false
		}},
		{"panel-json-3", func(a *application) {
			a.config.AwakeEnabled = false
			addWindows(a, 3)
		}},
		{"panel-json-7", func(a *application) {
			a.config.AwakeEnabled = false
			addWindows(a, 7)
		}},
		{"panel-json-7-open", func(a *application) {
			a.config.AwakeEnabled = false
			addWindows(a, 7)
		}},
		{"panel-empty", func(a *application) {
			a.config.AwakeEnabled = false
			a.config.JSONEnabled = false
		}},
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			mode := "light"
			if dark {
				mode = "dark"
			}
			for _, scenario := range scenarios {
				a := panelTestApp(t)
				a.config.AwakeMinutes = 60
				scenario.setup(a)
				a.showMenubarPanel()
				if scenario.name == "panel-json-7-open" {
					a.panel.expanded = true
				}
				token := tokens(dark, locale)
				tt := ui.NewTester(a.menubarPanelView, int(token.SizeMenubarPanelWidth), a.menubarPanelHeight(token))
				tt.SetScale(2)
				tt.SetLocale(locale)
				tt.SetDark(dark)
				tt.Frame()
				file, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%s-%s.png", scenario.name, locale, mode)))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(file, tt.Image()); err != nil {
					file.Close()
					t.Fatal(err)
				}
				file.Close()
			}
		}
	}
}

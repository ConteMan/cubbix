package main

import (
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestAwakeScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_AWAKE_UI_EVIDENCE")
	if dir == "" {
		t.Skip("UI evidence not requested")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	a := awakeTestApp(t)
	icons := map[int]*ui.Bitmap{}
	// Use the system's real graphical app icons in otherwise synthetic process data.
	for pid, src := range map[int]string{936: "/System/Applications/Utilities/Terminal.app/Contents/Resources/Terminal.icns", 724: "/Applications/Safari.app/Contents/Resources/AppIconUpdated.icns"} {
		dst := filepath.Join(t.TempDir(), "icon.png")
		if _, err := exec.Command("/usr/bin/sips", "-s", "format", "png", src, "--out", dst).CombinedOutput(); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(dst)
		if err != nil {
			t.Fatal(err)
		}
		img, _, err := image.Decode(file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		icons[pid] = ui.NewBitmap(img)
	}

	capture := func(name, locale string, dark bool, view func(*ui.Context), w, h int) {
		t.Helper()
		tt := ui.NewTester(view, w, h)
		tt.SetScale(2)
		tt.SetLocale(locale)
		tt.SetDark(dark)
		if strings.HasPrefix(name, "conflict") {
			if rect, ok := tt.Find(a.text(locale, a.shortcutError)); ok {
				tt.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
				tt.Advance(700 * time.Millisecond)
				tt.Frame()
			}
		}
		file, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			mode := "light"
			if dark {
				mode = "dark"
			}
			suffix := "-" + locale + "-" + mode
			for _, page := range []int{2, 3} {
				a.page = page
				name := "tiles"
				if page == 3 {
					name = "shortcuts"
				}
				capture(name+suffix, locale, dark, a.view, 720, 340)
			}
			a.page = 3
			a.shortcutRecording = true
			capture("recording"+suffix, locale, dark, a.view, 720, 340)
			a.shortcutDraft = "Cmd+,"
			a.shortcutError = "R05Conflict"
			capture("conflict"+suffix, locale, dark, a.view, 720, 340)
			a.shortcutRecording = false
			a.shortcutDraft = ""
			a.shortcutError = ""
			for _, kind := range []string{"custom", "until", "follow"} {
				a.dialog = &awakeDialog{kind: kind, hours: "1", minutes: "30", until: "02:30", selected: 1842, icons: icons, processes: []runningProcess{{PID: 1842, Name: "codex", Identity: "sample"}, {PID: 2087, Name: "claude", Identity: "sample"}, {PID: 936, Name: "Terminal", Identity: "sample"}, {PID: 724, Name: "Safari", Identity: "sample"}}}
				height := 108
				if kind == "follow" {
					height = 280
				}
				capture(kind+suffix, locale, dark, a.awakeDialogView, 440, height)
			}
			a.awake.mu.Lock()
			a.awake.state = awakeState{}
			a.awake.mu.Unlock()
			capture("menu-off"+suffix, locale, dark, a.menuEvidenceView, 352, 292)
			a.awake.mu.Lock()
			a.awake.state = awakeState{Active: true, Session: awakeSession{Mode: "screen", End: time.Now().Add(84 * time.Minute)}}
			a.awake.mu.Unlock()
			capture("menu-on"+suffix, locale, dark, a.menuEvidenceView, 352, 292)
			a.awake.mu.Lock()
			a.awake.state = awakeState{}
			a.awake.mu.Unlock()
		}
	}
}

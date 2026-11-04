package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLidScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_LID_UI_EVIDENCE")
	if dir == "" {
		t.Skip("lid UI evidence not requested")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			mode := "light"
			if dark {
				mode = "dark"
			}
			for _, state := range []string{"unregistered", "available", "enabling", "on", "battery", "low", "foreign", "restore-failed", "approval", "denied", "unsupported", "stale", "settings", "palette"} {
				a, s, p, svc, conn := lidTestApp(t)
				a.panel = &menubarPanelState{}
				switch state {
				case "unregistered":
					svc.status = svcNotRegistered
					a.lid.snapshotValue.service = svcNotRegistered
				case "available":
					a.lid.snapshotValue.service = svcEnabled
				case "enabling":
					a.lidBusy = true
					a.lid.snapshotValue.state = lidEnabling
				case "on", "battery":
					if state == "battery" {
						p.kind = "battery"
					}
					session := a.selectedSession()
					session.Lid = true
					if err := a.startAwake(session); err != nil {
						t.Fatal(err)
					}
				case "low":
					p.kind = "battery"
					p.percent = 20
					a.lid.snapshotValue.reason = "LidBatteryTooLow"
					a.lid.snapshotValue.state = lidBlocked
				case "foreign":
					s.value = 1
					a.lid.Inspect()
				case "restore-failed":
					session := a.selectedSession()
					session.Lid = true
					a.startAwake(session)
					s.failZero = true
					a.lid.stop()
				case "approval":
					a.lid.snapshotValue.service = svcRequiresApproval
					a.lid.snapshotValue.reason = "LidAuthNeeded"
				case "denied":
					a.lid.snapshotValue.service = svcDenied
					a.lid.snapshotValue.reason = "LidDenied"
				case "unsupported":
					a.lid.supported = false
				case "stale":
					s.value = 1
					conn.gone = true
					a.lid.writeMarker("stale")
					a.lid.Inspect()
				}
				token := tokens(dark, locale)
				width, height := int(token.SizeMenubarPanelWidth), a.menubarPanelHeight(token)
				view := a.menubarPanelView
				if state == "settings" {
					a.page = 2
					width, height = 720, 340
					view = a.view
				}
				if state == "palette" {
					a.showPalette()
					a.palette.query = "常亮 2h 合盖"
					if locale == "en" {
						a.palette.query = "awake 2h lid"
					}
					a.refreshPalette(a.palette, locale)
					a.togglePaletteActions()
					width, height = paletteWindowWidth(), paletteWindowHeight()
					view = a.paletteView
				}
				tt := ui.NewTester(view, width, height)
				tt.SetScale(2)
				tt.SetLocale(locale)
				tt.SetDark(dark)
				tt.Frame()
				if state == "restore-failed" || state == "stale" || state == "approval" {
					r, ok := tt.Find(a.text(locale, a.lidReason()))
					if !ok {
						t.Fatal("missing alert", state)
					}
					tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
					tt.Advance(time.Second)
					tt.Frame()
					if (state == "restore-failed" || state == "stale") && !tt.HasText(a.text(locale, "LidForceRestore")) {
						t.Fatal("missing force button", tt.Texts())
					}
				}
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%s-%s.png", state, locale, mode)))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, tt.Image())
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
func TestLidUIInteraction(t *testing.T) {
	a, _, _, _, _ := lidTestApp(t)
	a.panel = &menubarPanelState{}
	tt := panelView(a, "en")
	if err := tt.Click(a.text("en", "R05Lid")); err != nil {
		t.Fatal(err)
	}
	if !a.awake.snapshot().Session.Lid {
		t.Fatal("switch did not enable")
	}
	tt.Frame()
	if !tt.HasText(a.text("en", "R05Lid")) {
		t.Fatal("missing status")
	}
	a.page = 2
	settings := ui.NewTester(a.view, 720, 340)
	settings.SetLocale("en")
	if err := settings.Click("30%"); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(a.path)
	if err != nil || loaded.AwakeLidBatteryMin != 30 {
		t.Fatal("battery setting not saved")
	}
}

func TestLidRecoveryPopoverCanClick(t *testing.T) {
	a, s, _, _, conn := lidTestApp(t)
	s.value = 1
	conn.gone = true
	a.lid.writeMarker("stale")
	a.lid.Inspect()
	a.panel = &menubarPanelState{}
	tt := panelView(a, "en")
	if err := tt.Click(a.text("en", "LidStaleSetting")); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(a.text("en", "LidForceRestore")) {
		t.Fatal("recovery did not open")
	}
	if err := tt.Click(a.text("en", "LidForceRestore")); err != nil {
		t.Fatal(err)
	}
	if s.value != 0 || a.lid.markerExists() {
		t.Fatal("recovery button was not interactive")
	}
}

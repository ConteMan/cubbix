package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestFeedbackSettingsRows renders the shortcuts page in memory and checks
// that both feedback rows and their labels are there in each language.
func TestFeedbackSettingsRows(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		a := awakeTestApp(t)
		a.page, a.notesPage = 3, false
		tt := ui.NewTester(a.view, 720, 340)
		tt.SetLocale(locale)
		for _, key := range []string{"FeedbackMode", "FeedbackHint", "FeedbackNotification", "FeedbackOff", "FeedbackPosition"} {
			if !tt.HasText(a.text(locale, key)) {
				t.Fatalf("%s: missing %s: %v", locale, key, tt.Texts())
			}
		}
	}
}

// Exercise the actual segmented control, including disabled event propagation.
func TestFeedbackPositionControlInteraction(t *testing.T) {
	a := awakeTestApp(t)
	h := testFeedback(a)
	a.page = 3
	a.feedbackMode = feedbackIndex(feedbackModes, "notification")
	tt := ui.NewTester(a.view, 720, 340)
	tt.SetLocale("en")
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("Bottom left")
	if a.feedbackCornerName() != "top-right" {
		t.Fatal("disabled position accepted a pointer event")
	}
	h.presenter.denied = true
	tt.Frame()
	click("Bottom left")
	if a.feedbackCornerName() != "bottom-left" {
		t.Fatal("degraded notification did not unlock position")
	}
	loaded, err := loadConfig(a.path)
	if err != nil || loaded.FeedbackCorner != "bottom-left" {
		t.Fatalf("position not saved: %+v %v", loaded, err)
	}
	click("Off")
	click("Top left")
	if a.feedbackCornerName() != "bottom-left" || a.feedbackModeName() != "off" {
		t.Fatal("off did not lock position")
	}
	click("On-screen hint")
	click("Center")
	if a.feedbackCornerName() != "center" {
		t.Fatal("hint mode did not unlock position")
	}
}

func TestFeedbackViewAnimationAndTheme(t *testing.T) {
	a := awakeTestApp(t)
	h := testFeedback(a)
	tt := ui.NewTester(a.feedbackView, 320, 36)
	tt.SetLocale("en")
	pixels := func() []byte { return bytes.Clone(tt.Image().Pix) }
	empty := pixels()
	h.presenter.message = a.awakeFeedbackMessage("en", awakeSession{Minutes: 60})
	h.presenter.showing = true
	tt.Frame()
	tt.Advance(75 * time.Millisecond)
	entering := pixels()
	tt.Advance(100 * time.Millisecond)
	resting := pixels()
	if bytes.Equal(empty, entering) || bytes.Equal(entering, resting) {
		t.Fatal("capsule did not animate after the empty root frame")
	}
	h.presenter.message = feedbackMessage{Icon: "power", Text: "Keep awake off", Muted: true}
	tt.Frame()
	tt.Advance(50 * time.Millisecond)
	changing := pixels()
	tt.Advance(500 * time.Millisecond)
	if bytes.Equal(changing, pixels()) {
		t.Fatal("content replacement did not animate")
	}
	light := pixels()
	tt.SetDark(true)
	tt.Advance(500 * time.Millisecond)
	dark := pixels()
	if bytes.Equal(light, dark) || !tt.HasText("Keep awake off") {
		t.Fatal("visible hint did not follow theme changes")
	}
	h.presenter.showing = false
	tt.Frame()
	tt.Advance(75 * time.Millisecond)
	if bytes.Equal(empty, pixels()) {
		t.Fatal("capsule disappeared before its exit finished")
	}
	tt.Advance(100 * time.Millisecond)
	if !bytes.Equal(empty, pixels()) {
		t.Fatal("exit left content in the transparent slot")
	}
}

// TestFeedbackScreenshots renders the real hint view and the real settings
// page with ui.Tester. It never creates a native window or shows a hint.
func TestFeedbackScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_FEEDBACK_UI_EVIDENCE")
	if dir == "" {
		t.Skip("feedback UI evidence not requested")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	a := awakeTestApp(t)
	a.feedback = newFeedbackPresenter()
	a.feedback.mode = a.effectiveFeedbackMode
	a.feedback.corner = a.feedbackCornerName
	capture := func(name string, view func(*ui.Context), width, height int, locale string, dark bool) {
		t.Helper()
		tt := ui.NewTester(view, width, height)
		tt.SetScale(2)
		tt.SetLocale(locale)
		tt.SetDark(dark)
		// Let the theme rebuild's color and entrance transitions settle, so the
		// evidence shows the resting look rather than an in-flight frame.
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
		file, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	until := "02:30"
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			mode := "light"
			if dark {
				mode = "dark"
			}
			suffix := "-" + locale + "-" + mode
			states := []struct {
				name    string
				message feedbackMessage
			}{
				{"on-hour", a.awakeFeedbackMessage(locale, awakeSession{Minutes: 60})},
				{"on-custom", a.awakeFeedbackMessage(locale, awakeSession{Minutes: 45})},
				{"on-unlimited", a.awakeFeedbackMessage(locale, awakeSession{})},
				{"on-until", a.awakeFeedbackMessage(locale, awakeSession{Until: until})},
				{"on-follow", a.awakeFeedbackMessage(locale, awakeSession{PID: 421, Name: "Google Chrome"})},
				{"off", feedbackMessage{Icon: "power", Title: a.text(locale, "Awake"), Text: a.text(locale, "ToastAwakeOff"), Muted: true}},
				{"error", feedbackMessage{Icon: "triangle-alert", Title: a.text(locale, "Awake"), Text: a.text(locale, "ToastAwakeError"), Danger: true}},
			}
			for _, state := range states {
				for _, corner := range []string{"top-right", "bottom-left"} {
					a.feedback.showing = true
					a.feedback.message = state.message
					a.feedback.corner = func() string { return corner }
					capture("feedback-"+state.name+"-"+corner+suffix, a.feedbackView, 320, 36, locale, dark)
				}
			}
			// Settings: default hint mode (editable position), a working
			// notification (disabled position), and a denied notification
			// (editable again with the alert).
			a.feedback.corner = func() string { return "top-right" }
			for _, state := range []struct {
				name   string
				mode   string
				denied bool
			}{
				{"hint", "hint", false},
				{"notification", "notification", false},
				{"denied", "notification", true},
			} {
				a.feedbackMode = feedbackIndex(feedbackModes, state.mode)
				a.feedback.denied = state.denied
				a.page, a.notesPage = 3, false
				capture("feedback-settings-"+state.name+suffix, a.view, 720, 340, locale, dark)
			}
			a.feedback.denied = false
		}
	}
}

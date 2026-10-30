package main

import (
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestURLMalformedQueriesNeverPartiallyExecute(t *testing.T) {
	a := awakeTestApp(t)
	opens := 0
	a.paletteOpenJSON = func(string) { opens++ }
	a.jsonReadClipboard = func() string { t.Fatal("URL read clipboard"); return "" }
	before := a.config
	for _, raw := range []string{
		"cubbix://awake?minutes=%ZZ", "cubbix://awake?minutes=60&mode=%GG",
		"cubbix://awake?minutes=60&extra=%", "cubbix://awake?minutes=60;mode=tasks",
		"cubbix://json/new?text=%GG", "cubbix://json/new?text=%FF",
		"cubbix://json/new?blank=true", "cubbix://json/new?blank=0", "cubbix://json/new?blank=",
		"cubbix://user:password@awake?minutes=1", "cubbix://awake:80?minutes=1",
		"cubbix://awake#other", "cubbix:awake", "cubbix:///awake",
		"cubbix://json/new?file=/etc/passwd", // Unknown fields cannot become paths.
	} {
		req, ok := parsePaletteURL(raw, true, true, jsonFileLimit)
		if raw == "cubbix://json/new?file=/etc/passwd" {
			if !ok || req.Text != "" {
				t.Fatalf("unknown file parameter = %+v %v", req, ok)
			}
			continue
		}
		if ok {
			t.Fatalf("malformed URL accepted: %q -> %+v", raw, req)
		}
	}
	if opens != 0 || a.awake.snapshot().Active || !reflect.DeepEqual(a.config, before) {
		t.Fatal("malformed URL changed state")
	}
	if _, ok := parsePaletteURL("cubbix://json/new?text="+strings.Repeat("x", 4100), true, true, 4); ok {
		t.Fatal("raw URL budget was not enforced")
	}
	req, ok := parsePaletteURL("cubbix://json/new?text=&blank=1", true, true, 4)
	if !ok || req.Blank {
		t.Fatal("an explicitly empty text must take precedence over blank")
	}
}

func TestURLJSONTreatsExecutableAndFileTextAsBody(t *testing.T) {
	a := awakeTestApp(t)
	a.jsonReadClipboard = func() string { t.Fatal("URL read clipboard"); return "" }
	var bodies []string
	a.paletteOpenJSON = func(text string) { bodies = append(bodies, text) }
	payloads := []string{"", `{"a":1}`, `/etc/passwd`, `this.constructor.constructor("return 7")()`, `<script>throw Error("executed")</script>`}
	for _, body := range payloads {
		req, ok := parsePaletteURL("CUBBIX://JSON//NEW///?text="+url.QueryEscape(body)+"&blank=1", true, true, jsonFileLimit)
		if !ok {
			t.Fatalf("body rejected: %q", body)
		}
		if err := a.runURL(req); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(bodies, payloads) {
		t.Fatalf("URL did not preserve literal bodies: %q", bodies)
	}
}

func TestURLDeliveryIsAsynchronousAndRechecksAvailability(t *testing.T) {
	for _, disable := range []bool{false, true} {
		a := awakeTestApp(t)
		delivery := make(chan func(), 1)
		a.urlDispatch = func(fn func()) { delivery <- fn }
		var got string
		a.paletteOpenJSON = func(body string) { got = body }
		// Parsing cannot wait for the main-thread dispatcher to run its callback.
		a.handleURL("cubbix://json/new?text=body")
		select {
		case fn := <-delivery:
			a.config.JSONEnabled = !disable
			fn()
		case <-time.After(2 * time.Second):
			t.Fatal("URL never reached the dispatcher")
		}
		if !disable && got != "body" || disable && got != "" {
			t.Fatalf("delivery after disable=%v: %q", disable, got)
		}
	}
	a := awakeTestApp(t)
	delivery := make(chan func(), 1)
	a.urlDispatch = func(fn func()) { delivery <- fn }
	a.paletteOpenJSON = func(string) { t.Fatal("delivered URL after quitting") }
	a.handleURL("cubbix://json/new")
	select {
	case fn := <-delivery:
		a.quitting = true
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("missing delivery")
	}
}

func TestURLAwakeErrorsAndToggle(t *testing.T) {
	a := awakeTestApp(t)
	if err := a.runURL(paletteURLRequest{Kind: "awake.toggle"}); err != nil || !a.awake.snapshot().Active {
		t.Fatalf("toggle start: %v", err)
	}
	if err := a.runURL(paletteURLRequest{Kind: "awake.toggle"}); err != nil || a.awake.snapshot().Active {
		t.Fatalf("toggle stop: %v", err)
	}
	a.awake.launch = func([]string) (awakeChild, error) { return nil, errors.New("launch failed") }
	if err := a.runURL(paletteURLRequest{Kind: "awake"}); err == nil {
		t.Fatal("URL swallowed start failure")
	}
	if a.settingsError != "" {
		t.Fatal("URL failure leaked into Settings")
	}
}

func TestPaletteLaunchErrorSurvivesFramesAndRetry(t *testing.T) {
	a := paletteTestApp(t)
	fail := true
	launches := 0
	a.awake.launch = func([]string) (awakeChild, error) {
		launches++
		if fail {
			return nil, errors.New("launch failed")
		}
		return &fakeAwakeChild{done: make(chan error, 1)}, nil
	}
	tt := ui.NewTester(a.paletteView, 640, 400)
	tt.Type("常亮 30m")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Frame()
	if a.palette.errorKey != "AwakeStartError" || a.palette.query != "常亮 30m" || len(a.config.PaletteRecents) != 0 {
		t.Fatalf("failure was lost across frames: %+v", a.palette)
	}
	if a.config.AwakeMinutes != 60 {
		t.Fatal("failed command changed remembered duration")
	}
	fail = false
	tt.Type(" ")
	tt.Key(0, ui.KeyEnter)
	if launches != 2 || !a.awake.snapshot().Active || a.palette.errorKey != "" {
		t.Fatalf("retry failed: launches=%d error=%q", launches, a.palette.errorKey)
	}
}

func TestPaletteActionsRecordOnlySuccessfulExecutions(t *testing.T) {
	a := paletteTestApp(t)
	tt := ui.NewTester(a.paletteView, 640, 400)
	tt.Type("常亮 45m")
	tt.Key(ui.Cmd, ui.KeyK)
	a.awake.launch = func([]string) (awakeChild, error) { return nil, errors.New("launch failed") }
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.palette.errorKey != "AwakeStartError" || len(a.config.PaletteRecents) != 0 {
		t.Fatal("failed action was recorded or error lost")
	}
	a.awake.launch = func([]string) (awakeChild, error) { return &fakeAwakeChild{done: make(chan error, 1)}, nil }
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	if !a.awake.snapshot().Active || a.awake.snapshot().Session.Mode != "tasks" || !reflect.DeepEqual(a.config.PaletteRecents, []string{"awake.start"}) {
		t.Fatalf("action result/recents = %+v / %v", a.awake.snapshot(), a.config.PaletteRecents)
	}
}

func TestAwakeMinutesRejectOverflowAndSplitDigits(t *testing.T) {
	for _, arg := range []string{"307445734561825861h17m", "9223372036854775807h", "999999999999999999999h", "1 2m", "1 h o u r", "1m30h"} {
		if got := parseAwakeArg(arg, time.Now()); got.Error != "DurationError" {
			t.Fatalf("unsafe duration %q = %+v", arg, got)
		}
	}
	a := awakeTestApp(t)
	before := a.config
	for _, value := range []int{-1, awakeMinutesMaximum + 1} {
		if err := a.setDuration(value); err == nil {
			t.Fatal("invalid duration accepted")
		}
		if !reflect.DeepEqual(before, a.config) || a.awake.snapshot().Active {
			t.Fatal("invalid duration mutated state")
		}
	}
	a.config.AwakeEnabled = false
	a.dialog = &awakeDialog{kind: "custom", hours: "0", minutes: "30"}
	a.confirmAwakeDialog()
	if a.dialog == nil || a.dialog.errorKey != "AwakeStartError" || a.awake.snapshot().Active {
		t.Fatal("dialog bypassed tile availability")
	}
}

func TestShortcutRecordingOwnsAllThreeSlots(t *testing.T) {
	a := awakeTestApp(t)
	registered := map[string]func(){}
	a.registerShortcut = func(acc string, fn func()) error {
		if _, exists := registered[acc]; exists {
			t.Fatalf("shortcut registered twice: %s", acc)
		}
		registered[acc] = fn
		return nil
	}
	a.unregisterShortcut = func(acc string) { delete(registered, acc) }
	a.registerAwakeShortcut()
	a.registerJSONShortcut()
	a.registerPaletteShortcut()
	a.beginTileShortcutRecording("palette")
	if len(registered) != 0 {
		t.Fatalf("shortcuts not suspended: %v", registered)
	}
	a.togglePalette()
	if a.palette != nil {
		t.Fatal("recording opened palette")
	}
	a.recordShortcut(ui.Ctrl|ui.Alt|ui.Cmd, ui.KeyK)
	if !a.shortcutRecording || a.shortcutError == "" || len(registered) != 0 {
		t.Fatal("conflicting shortcut committed")
	}
	a.recordShortcut(ui.Alt, ui.KeyComma)
	if a.shortcutRecording || a.config.PaletteShortcut != "Alt+," || len(registered) != 3 {
		t.Fatal("new shortcut not immediately restored")
	}
	a.beginTileShortcutRecording("palette")
	a.cancelShortcutRecording()
	if len(registered) != 3 {
		t.Fatal("cancel did not restore all slots")
	}
	config, err := loadConfig(a.path)
	if err != nil || config.PaletteShortcut != "Alt+," {
		t.Fatal("shortcut not persisted")
	}
	for _, slot := range a.shortcutSlots() {
		a.unregisterShortcut(slot.Acc)
	}
	if len(registered) != 0 {
		t.Fatal("shortcut leak")
	}
	for _, pair := range [][2]string{{"Alt+.", "option+."}, {"Ctrl+Cmd+K", " command + control + k "}, {"Cmd+Q", "CommandOrControl+q"}, {"Cmd++", "Meta+Plus"}} {
		if !sameShortcut(pair[0], pair[1]) {
			t.Fatalf("shortcut alias missed: %q", pair)
		}
	}
}

func TestSearchDefaultUsesShorterTitleOnScoreTie(t *testing.T) {
	a := commandTestApp(t)
	commands := []*command{
		{ID: "long", Title: func(*application, string) string { return "Long title" }, Keywords: []string{"match"}},
		{ID: "short", Title: func(*application, string) string { return "Short" }, Keywords: []string{"match"}},
	}
	// Non-exact keyword query: score ties must follow the sorted display order.
	got, _ := bestCommand(a, "en", "mat", commands)
	if got == nil || got.ID != "short" {
		t.Fatal("default selection disagrees with sorted results")
	}
}

func TestFailedAwakeMenuActionsReportErrors(t *testing.T) {
	a := awakeTestApp(t)
	a.awake.launch = func([]string) (awakeChild, error) { return nil, errors.New("launch failed") }
	for _, item := range a.awakeMenu("zh-CN") {
		if item.ID == "awake-start" {
			item.Click(nil, nil)
		}
		for _, child := range item.Submenu {
			if child.Label == a.text("zh-CN", "R05Half") {
				child.Click(nil, nil)
			}
		}
	}
	if a.settingsError != "AwakeStartError" {
		t.Fatal("native menu did not report launch failure")
	}
	if a.config.AwakeMinutes != 60 {
		t.Fatal("failed menu changed duration")
	}
}

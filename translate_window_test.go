package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type fakeTranslateWindow struct {
	fakeFrame
	destroyed bool
	pins      []bool
	events    *[]string
}

func (f *fakeTranslateWindow) SetContentSize(width, height int) {
	f.bounds.Width, f.bounds.Height = width, height
}

func (f *fakeTranslateWindow) SetAlwaysOnTop(pinned bool) { f.pins = append(f.pins, pinned) }
func (f *fakeTranslateWindow) Destroy()                   { f.destroyed, f.visible = true, false }
func (f *fakeTranslateWindow) Show() {
	if f.events != nil {
		*f.events = append(*f.events, "show")
	}
	f.fakeFrame.Show()
}
func (f *fakeTranslateWindow) Focus() {
	if f.events != nil {
		*f.events = append(*f.events, "focus")
	}
	f.fakeFrame.Focus()
}

func translateWindowTestApp(t *testing.T) (*application, *[]*fakeTranslateWindow) {
	t.Helper()
	a := translateTestApp(t)
	windows := new([]*fakeTranslateWindow)
	a.createTranslateWindow = func(state *translateState) {
		window := &fakeTranslateWindow{}
		state.testFrame = window
		*windows = append(*windows, window)
	}
	return a, windows
}

func TestTranslateToolWindowOptions(t *testing.T) {
	a := translateTestApp(t)
	for _, dark := range []bool{false, true} {
		for _, locale := range []string{"zh-CN", "en"} {
			tokens := tokens(dark, locale)
			got := a.translateWindowOptions(tokens, locale)
			json := a.jsonWindowOptions(&jsonDocument{}, tokens, locale)
			if got.Panel || got.Frameless || got.Transparent || got.DisableResize || got.DisableMove || got.DisableClose || got.AlwaysOnTop {
				t.Fatalf("unexpected tool window flags: %+v", got)
			}
			if got.TitleBarStyle != json.TitleBarStyle || !reflect.DeepEqual(got.TrafficLightPosition, json.TrafficLightPosition) || got.DisableFullScreen != json.DisableFullScreen || got.BackgroundColor != json.BackgroundColor || got.DisableMinimize != json.DisableMinimize || got.DisableMaximize != json.DisableMaximize || got.Hidden != json.Hidden || got.Page.DevTools != json.Page.DevTools {
				t.Fatal("translation and JSON window chrome differ")
			}
			if got.Content == nil || got.Width != 480 || got.Height != 360 || got.MinWidth != 480 || got.MinHeight != 260 {
				t.Fatalf("unexpected content or geometry: %+v", got)
			}
		}
	}
}

func TestTranslateWindowGeometryRoundTrip(t *testing.T) {
	a, windows := translateWindowTestApp(t)
	screens := paletteTestScreens(a)
	screens.cursor = mygo.Point{X: -600, Y: 300}
	a.openTranslateText("")
	first := (*windows)[0]
	if first.bounds.X >= 0 || a.translate.place.display.ID != 2 {
		t.Fatalf("first window missed pointer display: %+v", first.bounds)
	}
	first.drag(-1500, 250)
	first.SetContentSize(620, 460)
	a.closeTranslate()
	if !first.destroyed || a.translate != nil {
		t.Fatal("closing did not destroy the window")
	}
	saved, err := loadConfig(a.path)
	if err != nil || saved.TranslateWindows["2"] != (translateGeometry{X: 420, Y: 225, Width: 620, Height: 460}) {
		t.Fatalf("geometry did not persist: %+v, %v", saved.TranslateWindows, err)
	}
	a.config = saved
	a.openTranslateText("")
	second := (*windows)[1]
	if second.bounds != first.bounds {
		t.Fatalf("reopened at %+v, want %+v", second.bounds, first.bounds)
	}
	a.closeTranslate()
	screens.cursor = mygo.Point{X: 700, Y: 300}
	a.openTranslateText("")
	third := (*windows)[2]
	if third.bounds.Width != 480 || third.bounds.Height != 360 || third.bounds.X < 0 {
		t.Fatalf("another display inherited geometry: %+v", third.bounds)
	}
	third.drag(200, 200)
	third.SetContentSize(500, 400)
	a.closeTranslate()
	if len(a.config.TranslateWindows) != 2 || a.config.TranslateWindows["2"] != saved.TranslateWindows["2"] {
		t.Fatal("per-display geometry was overwritten")
	}
}

func TestTranslateWindowRepairsWithoutSavingOrFocusing(t *testing.T) {
	for _, minimized := range []bool{false, true} {
		a, windows := translateWindowTestApp(t)
		screens := paletteTestScreens(a)
		a.config.TranslateWindows = map[string]translateGeometry{"1": {X: 850, Y: 400, Width: 560, Height: 400}}
		a.openTranslateText("")
		window := (*windows)[0]
		window.minimized = minimized
		screens.displays[0].WorkArea = mygo.Rectangle{X: 0, Y: 25, Width: 800, Height: 600}
		before := window.bounds
		a.repairWindowPlaces(screens.displays)
		if minimized {
			if window.bounds != before || !a.translate.place.needsRepair {
				t.Fatal("minimized window moved before restoration")
			}
			a.showTranslate()
		} else if window.shows != 1 || window.focuses != 1 {
			t.Fatal("display repair showed or focused the window")
		}
		if window.bounds.X+window.bounds.Width > 800 || window.bounds.Y+window.bounds.Height > 625 {
			t.Fatalf("window escaped changed work area: %+v", window.bounds)
		}
		a.closeTranslate()
		if a.config.TranslateWindows["1"].X != 850 || a.config.TranslateWindows["1"].Y != 400 {
			t.Fatal("automatic repair overwrote the remembered seat")
		}
	}
	a, windows := translateWindowTestApp(t)
	screens := paletteTestScreens(a)
	screens.cursor = mygo.Point{X: -600, Y: 300}
	a.openTranslateText("")
	screens.displays = screens.displays[:1]
	screens.cursor = mygo.Point{X: 500, Y: 300}
	a.repairWindowPlaces(screens.displays)
	if (*windows)[0].bounds.X < 0 || a.translate.place.display.ID != 1 {
		t.Fatal("disconnected display left the window unreachable")
	}
}

func TestTranslateWindowReusesAndReplacesSelection(t *testing.T) {
	a, windows := translateWindowTestApp(t)
	a.config.TranslateBaseURL, a.config.TranslateModel = "https://h/v1", "m"
	a.translateSecret.(*fakeTranslateSecrets).value = []byte("k")
	a.translateStreamFn = func(_ context.Context, _ translateConfig, text, _, _ string, _ func(string)) (string, error) {
		return text + " translated", nil
	}
	a.openTranslateText("first sentence")
	state := a.translate
	window := (*windows)[0]
	position := window.bounds
	window.minimized = true
	state.visible = false
	setTestCursor(a, mygo.Point{X: -500, Y: 300})
	a.openTranslateText("")
	if a.translate != state || len(*windows) != 1 || state.output != "first sentence translated" || window.restores != 1 || window.bounds != position || !state.focus {
		t.Fatal("empty selection did not restore the existing result and frame")
	}
	a.openTranslateText("next sentence")
	if a.translate != state || len(*windows) != 1 || state.input != "next sentence" || state.output != "next sentence translated" || window.bounds != position {
		t.Fatal("new selection did not replace text in the existing window")
	}
}

func TestTranslateWindowCloseCancelsWork(t *testing.T) {
	for _, module := range []bool{false, true} {
		a, windows := translateWindowTestApp(t)
		a.openTranslateText("")
		state := a.translate
		ctx, cancel := context.WithCancel(context.Background())
		state.cancel = cancel
		generation := state.request
		if module {
			a.changeTranslateEnabled(false)
		} else {
			a.closeTranslate()
		}
		if ctx.Err() == nil || state.request <= generation || a.translate != nil || !(*windows)[0].destroyed {
			t.Fatal("close retained request or window")
		}
	}
}

type orderedSelectionReader struct{ events *[]string }

func (r orderedSelectionReader) Trusted() bool       { return true }
func (r orderedSelectionReader) FrontmostPID() int32 { return otherSelectionPID() }
func (r orderedSelectionReader) SelectedText(context.Context, int32) (string, error) {
	*r.events = append(*r.events, "selection")
	return "selected sentence", nil
}

func TestTranslateSelectionPrecedesActivation(t *testing.T) {
	a, windows := translateWindowTestApp(t)
	var events []string
	a.selection = orderedSelectionReader{events: &events}
	a.policy = mygo.ActivationPolicyAccessory
	a.setActivationPolicy = func(mygo.ActivationPolicy) { events = append(events, "activation") }
	create := a.createTranslateWindow
	a.createTranslateWindow = func(state *translateState) {
		create(state)
		(*windows)[0].events = &events
	}
	a.toggleTranslate()
	if !reflect.DeepEqual(events, []string{"selection", "activation", "show", "focus"}) {
		t.Fatalf("selection/activation ordering: %v", events)
	}
	a.visibility(false)
	if a.policy != mygo.ActivationPolicyRegular {
		t.Fatal("a visible translation window lost regular activation policy")
	}
	a.closeTranslate()
	if a.policy != mygo.ActivationPolicyAccessory {
		t.Fatal("the last closed tool window retained regular activation policy")
	}
}

func TestTranslateWindowPinControls(t *testing.T) {
	a, windows := translateWindowTestApp(t)
	a.openTranslateText("")
	tester := ui.NewTester(a.translateView, 480, 360)
	tester.SetLocale("en")
	tester.Frame()
	_, ok := tester.Find(a.text("en", "R07Pin"))
	if !ok {
		t.Fatal("pin toolbar button missing")
	}
	tester.Click(a.text("en", "R07Pin"))
	if !a.translate.pinned {
		t.Fatal("toolbar button did not pin")
	}
	tester.Key(ui.Cmd|ui.Alt, ui.KeyP)
	if a.translate.pinned || !reflect.DeepEqual((*windows)[0].pins, []bool{true, false}) {
		t.Fatal("pin shortcut did not update native window level")
	}
}

func TestTranslateWindowInputGeometry(t *testing.T) {
	for _, dimensions := range [][2]int{{480, 260}, {480, 360}, {720, 500}} {
		a := translateTestApp(t)
		a.openTranslateText("")
		tester := ui.NewTester(a.translateView, dimensions[0], dimensions[1])
		tester.SetLocale("en")
		tester.Frame()
		input, found := tester.Find(a.text("en", "TranslateInputPlaceholder"))
		if !found || input.H != 36 {
			t.Fatalf("source input minimum height escaped its row: %+v", input)
		}
		direction, found := tester.Find(a.text("en", "TranslateSwapDirection"))
		if !found || direction.Y < input.Y+input.H {
			t.Fatalf("direction overlaps source input: %+v / %+v", input, direction)
		}
	}
}

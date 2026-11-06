package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// fakeScreens are the displays, pointer and window position the palette sees
// in tests. The palette composes screen positions from them, so no case needs
// an app, a real screen or a native window.
type fakeScreens struct {
	displays []mygo.Display
	cursor   mygo.Point
	window   screenPoint
}

// paletteTestScreens replaces the palette's screen, pointer and window seams
// with fakes. The main display starts at the menu bar, the way a mac does, so
// the guide lines have to subtract the work area's origin.
func paletteTestScreens(a *application) *fakeScreens {
	f := &fakeScreens{
		displays: []mygo.Display{
			{ID: 1, Label: "main", Bounds: mygo.Rectangle{X: 0, Y: 0, Width: 1440, Height: 900}, WorkArea: mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}},
			{ID: 2, Label: "left", Bounds: mygo.Rectangle{X: -1920, Y: 0, Width: 1920, Height: 1080}, WorkArea: mygo.Rectangle{X: -1920, Y: 25, Width: 1920, Height: 1055}},
		},
		cursor: mygo.Point{X: 700, Y: 300},
	}
	a.displays = func() []mygo.Display { return f.displays }
	a.cursorPoint = func() mygo.Point { return f.cursor }
	a.paletteWindowPosition = func() screenPoint { return f.window }
	a.setPaletteWindowPosition = func(p screenPoint) { f.window = p }
	a.createPaletteGuide = func(*paletteGuideState) {}
	return f
}

func TestSnapPalette(t *testing.T) {
	size := screenSize{Width: 640, Height: 400}
	guides := paletteGuides{CenterX: 720, TopY: 121, MiddleY: 462}
	cases := []struct {
		name      string
		candidate screenPoint
		want      screenPoint
		snap      paletteSnap
	}{
		{"free", screenPoint{X: 100, Y: 300}, screenPoint{X: 100, Y: 300}, paletteSnap{}},
		{"vertical centre", screenPoint{X: 404, Y: 300}, screenPoint{X: 400, Y: 300}, paletteSnap{X: true}},
		{"vertical centre at the threshold", screenPoint{X: 408, Y: 300}, screenPoint{X: 400, Y: 300}, paletteSnap{X: true}},
		{"past the threshold", screenPoint{X: 409, Y: 300}, screenPoint{X: 409, Y: 300}, paletteSnap{}},
		{"default height", screenPoint{X: 100, Y: 117}, screenPoint{X: 100, Y: 121}, paletteSnap{Y: paletteSnapTop}},
		{"work area centre", screenPoint{X: 100, Y: 265}, screenPoint{X: 100, Y: 262}, paletteSnap{Y: paletteSnapMiddle}},
		// Both horizontal lines equally near: the default height wins, it is
		// where the panel opens.
		{"tie takes the default height", screenPoint{X: 100, Y: 118}, screenPoint{X: 100, Y: 121}, paletteSnap{Y: paletteSnapTop}},
		{"both lines nearest wins", screenPoint{X: 100, Y: 255}, screenPoint{X: 100, Y: 262}, paletteSnap{Y: paletteSnapMiddle}},
	}
	for _, c := range cases {
		got, snap := snapPalette(c.candidate, size, guides, 8)
		if got != c.want || snap != c.snap {
			t.Errorf("%s: got %+v %+v; want %+v %+v", c.name, got, snap, c.want, c.snap)
		}
	}
	// A tie needs guides half a panel apart; check that rule directly.
	tie := paletteGuides{CenterX: 720, TopY: 121, MiddleY: 321}
	if _, snap := snapPalette(screenPoint{X: 0, Y: 118}, size, tie, 8); snap.Y != paletteSnapTop {
		t.Errorf("tie snapped to %d; want the default height", snap.Y)
	}
}

func TestPaletteClamp(t *testing.T) {
	size := screenSize{Width: 640, Height: 400}
	main := mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}
	cases := []struct {
		name string
		in   screenPoint
		work mygo.Rectangle
		want screenPoint
	}{
		{"inside", screenPoint{X: 400, Y: 121}, main, screenPoint{X: 400, Y: 121}},
		{"past the right and bottom", screenPoint{X: 1000, Y: 700}, main, screenPoint{X: 800, Y: 500}},
		{"past the left and top", screenPoint{X: -100, Y: -100}, main, screenPoint{X: 0, Y: 25}},
		// A work area narrower than the panel leaves X alone: opening centres
		// it across, and a drag keeps the pointer's choice.
		{"narrow work area keeps x", screenPoint{X: -20, Y: 121}, mygo.Rectangle{X: 0, Y: 0, Width: 600, Height: 900}, screenPoint{X: -20, Y: 121}},
		// A work area shorter than the panel leaves Y alone for the same
		// reason; the caller's fallback pins it against the top.
		{"short work area keeps y", screenPoint{X: 100, Y: 20}, mygo.Rectangle{X: 0, Y: 0, Width: 1440, Height: 320}, screenPoint{X: 100, Y: 20}},
		{"display left of the main one", screenPoint{X: -1000, Y: 0}, mygo.Rectangle{X: -1920, Y: 25, Width: 1920, Height: 1055}, screenPoint{X: -1000, Y: 25}},
	}
	for _, c := range cases {
		if got := clampToWorkArea(c.in, size, c.work); got != c.want {
			t.Errorf("%s: got %+v; want %+v", c.name, got, c.want)
		}
	}
}

func TestPalettePointerScreen(t *testing.T) {
	box := ui.Rect{X: 32, Y: 32}
	window := screenPoint{X: 1000, Y: 500}
	press := palettePointerScreen(box, window, ui.InputEvent{X: 5, Y: 7})
	if press != (screenPoint{X: 1037, Y: 539}) {
		t.Fatalf("press = %+v", press)
	}
	// The window moved by the drag: the same screen point composes the same
	// way from the new window and the negative element coordinates that come
	// with it. Subtracting element-local positions would give the residual.
	moved := screenPoint{X: 1050, Y: 520}
	again := palettePointerScreen(box, moved, ui.InputEvent{X: -45, Y: -13})
	if again != press {
		t.Fatalf("screen point moved from %+v to %+v", press, again)
	}
	// Sub-pixel pointer positions round to the DIP they are nearest.
	if got := palettePointerScreen(ui.Rect{}, screenPoint{}, ui.InputEvent{X: 4.6, Y: 4.4}); got != (screenPoint{X: 5, Y: 4}) {
		t.Fatalf("rounding = %+v", got)
	}
}

func TestPaletteGuides(t *testing.T) {
	main := mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}
	if got := paletteGuidesFor(main, 96); got != (paletteGuides{CenterX: 720, TopY: 121, MiddleY: 462}) {
		t.Fatalf("main guides = %+v", got)
	}
	// Odd widths put the centre line on the left half of the middle pixel.
	odd := mygo.Rectangle{X: 0, Y: 25, Width: 1441, Height: 875}
	if got := paletteGuidesFor(odd, 96); got.CenterX != 720 {
		t.Fatalf("odd centre = %d", got.CenterX)
	}
	left := mygo.Rectangle{X: -1920, Y: 25, Width: 1920, Height: 1055}
	if got := paletteGuidesFor(left, 96); got != (paletteGuides{CenterX: -960, TopY: 121, MiddleY: 552}) {
		t.Fatalf("left display guides = %+v", got)
	}
}

func TestPaletteGuideLocal(t *testing.T) {
	// The overlay window sits at the work area's origin, its lines are placed
	// relative to it, and the guides are screen DIPs: the origin comes off.
	main := mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}
	guides := paletteGuidesFor(main, 96)
	if got := paletteGuideLocal(guides, main); got != (paletteGuides{CenterX: 720, TopY: 96, MiddleY: 437}) {
		t.Fatalf("main local guides = %+v", got)
	}
	left := mygo.Rectangle{X: -1920, Y: 25, Width: 1920, Height: 1055}
	if got := paletteGuideLocal(paletteGuidesFor(left, 96), left); got != (paletteGuides{CenterX: 960, TopY: 96, MiddleY: 527}) {
		t.Fatalf("left display local guides = %+v", got)
	}
}

func TestPaletteSeat(t *testing.T) {
	main := mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}
	panel := screenPoint{X: 400, Y: 121}
	stored := seatOffset(panel, main)
	if stored != (seat{X: 400, Y: 96}) {
		t.Fatalf("seat = %+v", stored)
	}
	if got := seatPoint(stored, main); got != panel {
		t.Fatalf("panel = %+v", got)
	}
	if got := seatKey(int64(-42)); got != "-42" {
		t.Fatalf("negative key = %q", got)
	}
	// An older config has no palette_positions table at all: the map is nil
	// and writing has to build it instead of panicking.
	config := Config{}
	config.putPaletteSeat("1", seat{X: 12, Y: 34})
	if !reflect.DeepEqual(config.PalettePositions, map[string]seat{"1": {X: 12, Y: 34}}) {
		t.Fatalf("seats = %+v", config.PalettePositions)
	}
}

func TestPaletteConfigDefaultsAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("version=1\nlanguage='zh-CN'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.PanelShortcut != defaultPanelKey {
		t.Fatalf("panel shortcut = %q; want the default", config.PanelShortcut)
	}
	if config.PalettePositions != nil {
		t.Fatalf("seats = %+v; want none", config.PalettePositions)
	}
	config.putPaletteSeat("732561", seat{X: 40, Y: 96})
	config.putPaletteSeat("-42", seat{X: -8, Y: 0})
	config.PanelShortcut = "Ctrl+Alt+Cmd+M"
	if err := saveConfig(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.PalettePositions, config.PalettePositions) || loaded.PanelShortcut != config.PanelShortcut {
		t.Fatalf("round trip = %+v", loaded.PalettePositions)
	}
	delete(loaded.PalettePositions, "-42")
	if len(loaded.PalettePositions) != 1 {
		t.Fatalf("deleting one seat kept %+v", loaded.PalettePositions)
	}
}

// TestPaletteSeatsInAnOldConfig covers a config written before this feature
// and one with a key that is not a display number: both load, and only the
// display's own offset is used.
func TestPaletteSeatsInAnOldConfig(t *testing.T) {
	a := paletteTestApp(t)
	a.createPaletteWindow = func(*paletteState) {}
	f := paletteTestScreens(a)
	content := "version = 1\nlanguage = 'zh-CN'\n[palette_positions]\n[palette_positions.abc]\nx = 5\ny = 6\n[palette_positions.1]\nx = 40\ny = 96\n"
	if err := os.WriteFile(a.path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig(a.path)
	if err != nil {
		t.Fatal(err)
	}
	a.config = config
	a.placePalette()
	margin := a.paletteMargin()
	if got := (screenPoint{X: f.window.X + margin, Y: f.window.Y + margin}); got != (screenPoint{X: 40, Y: 121}) {
		t.Fatalf("panel = %+v; want the remembered offset on the main display", got)
	}
}

func TestPaletteDisplays(t *testing.T) {
	f := &fakeScreens{displays: []mygo.Display{
		{ID: 1, Bounds: mygo.Rectangle{X: 0, Y: 0, Width: 1440, Height: 900}},
		{ID: 2, Bounds: mygo.Rectangle{X: -1920, Y: 0, Width: 1920, Height: 1080}},
	}}
	// The display a rectangle overlaps most, so a panel straddling two
	// displays follows the one it is mostly on.
	if got := displayMatching(f.displays, mygo.Rectangle{X: -100, Y: 100, Width: 640, Height: 400}); got.ID != 1 {
		t.Fatalf("matching display = %d; want the main one", got.ID)
	}
	if got := displayMatching(f.displays, mygo.Rectangle{X: -500, Y: 100, Width: 640, Height: 400}); got.ID != 2 {
		t.Fatalf("matching display = %d; want the left one", got.ID)
	}
	if got := displayNearest(f.displays, mygo.Point{X: -400, Y: 300}); got.ID != 2 {
		t.Fatalf("nearest display = %d; want the left one", got.ID)
	}
	if got := displayNearest(f.displays, mygo.Point{X: 900, Y: 300}); got.ID != 1 {
		t.Fatalf("nearest display = %d; want the main one", got.ID)
	}
}

// TestPalettePlaceUsesTheRememberedSeat covers opening: the display under the
// pointer, its remembered offset, and the clamp into its work area.
func TestPalettePlaceUsesTheRememberedSeat(t *testing.T) {
	a := paletteTestApp(t)
	a.createPaletteWindow = func(*paletteState) {}
	f := paletteTestScreens(a)
	a.placePalette()
	margin := a.paletteMargin()
	if want := (screenPoint{X: f.window.X + margin, Y: f.window.Y + margin}); want != (screenPoint{X: 400, Y: 121}) {
		t.Fatalf("default position = %+v", want)
	}
	// The second display remembers a place: the panel opens there.
	f.cursor = mygo.Point{X: -400, Y: 300}
	a.config.putPaletteSeat("2", seat{X: 100, Y: 50})
	a.placePalette()
	if got := (screenPoint{X: f.window.X + margin, Y: f.window.Y + margin}); got != (screenPoint{X: -1820, Y: 75}) {
		t.Fatalf("remembered position = %+v", got)
	}
	// A remembered place that no longer fits is pulled back inside.
	a.config.putPaletteSeat("2", seat{X: 5000, Y: 5000})
	a.placePalette()
	if got := (screenPoint{X: f.window.X + margin, Y: f.window.Y + margin}); got != (screenPoint{X: -640, Y: 680}) {
		t.Fatalf("clamped position = %+v", got)
	}
}

func TestPaletteResetCommand(t *testing.T) {
	a := paletteTestApp(t)
	a.createPaletteWindow = func(*paletteState) {}
	f := paletteTestScreens(a)
	command := findCommand(a.commands(), "palette.position.reset")
	if command == nil || command.Available != nil {
		t.Fatalf("reset command = %+v", command)
	}
	a.config.putPaletteSeat("1", seat{X: 10, Y: 20})
	f.window = screenPoint{X: 700 - a.paletteMargin(), Y: 500 - a.paletteMargin()}
	if err := command.Run(a, request{Command: command}); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.config.PalettePositions["1"]; ok {
		t.Fatalf("reset kept the seat: %+v", a.config.PalettePositions)
	}
	// Back at the default position for the display the panel was on.
	panel := screenPoint{X: f.window.X + a.paletteMargin(), Y: f.window.Y + a.paletteMargin()}
	if panel != (screenPoint{X: 400, Y: 121}) {
		t.Fatalf("position after reset = %+v", panel)
	}
}

func TestPaletteRememberedPositionOnSmallerWorkArea(t *testing.T) {
	for _, work := range []mygo.Rectangle{
		{X: -500, Y: 25, Width: 600, Height: 320},
		{X: -500, Y: 25, Width: 600, Height: 900},
		{X: -500, Y: 25, Width: 1000, Height: 320},
	} {
		a := paletteTestApp(t)
		f := paletteTestScreens(a)
		f.displays = []mygo.Display{{ID: 1, Bounds: work, WorkArea: work}}
		stored := seat{X: 5000, Y: 5000}
		a.config.putPaletteSeat("1", stored)
		a.placePalette()
		got := panelPointOf(a, f)
		want := screenPoint{X: work.X + max(0, work.Width-640), Y: work.Y + max(0, work.Height-400)}
		if work.Width < 640 {
			want.X = work.X + (work.Width-640)/2
		}
		if got != want || a.config.PalettePositions["1"] != stored {
			t.Fatalf("work=%+v: panel=%+v want=%+v seat=%+v", work, got, want, a.config.PalettePositions["1"])
		}
	}
}

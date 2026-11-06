package main

import (
	"os"
	"reflect"
	"testing"

	"github.com/egoist/mygo"
)

func TestPlacementSaveSkipsEmptyDisplays(t *testing.T) {
	for _, action := range []string{"hide", "close", "quit"} {
		t.Run(action, func(t *testing.T) {
			a, f := placementTestApp(t)
			a.show()
			f.drag(300, 300)
			a.observeSettingsMove()
			d, jf := addJSONFrame(a, 700, 460)
			a.placeJSONWindow(d)
			jf.drag(100, 200)
			a.observeJSONMove(d)
			settings, json := a.settingsPlace, d.place
			a.displays = func() []mygo.Display { return nil }
			switch action {
			case "hide":
				a.hide()
			case "close":
				if a.rememberJSONPlace(d) {
					t.Fatal("empty snapshot committed a JSON seat")
				}
			case "quit":
				a.saveWindowPlacesOnQuit()
			}
			if a.settingsPlace != settings || d.place != json || len(a.config.WindowPositions)+len(a.config.JSONPositions) != 0 {
				t.Fatal("empty snapshot consumed or committed pending seats")
			}
			if _, err := os.Stat(a.path); !os.IsNotExist(err) {
				t.Fatalf("empty snapshot wrote configuration: %v", err)
			}
		})
	}
}

func TestPlacementSaveIgnoresInactiveUnobservedMoves(t *testing.T) {
	for _, minimized := range []bool{false, true} {
		t.Run(map[bool]string{false: "hidden", true: "minimized"}[minimized], func(t *testing.T) {
			a, f := placementTestApp(t)
			a.show()
			d, jf := addJSONFrame(a, 700, 460)
			a.placeJSONWindow(d)
			for _, frame := range []*fakeFrame{f, jf} {
				frame.visible, frame.minimized = minimized, minimized
				frame.drag(123, 234) // No OnMove notification before quit.
			}
			a.saveWindowPlacesOnQuit()
			if len(a.config.WindowPositions)+len(a.config.JSONPositions) != 0 {
				t.Fatalf("inactive system move was persisted: %+v %+v", a.config.WindowPositions, a.config.JSONPositions)
			}
		})
	}
}

func TestOpeningWithEmptyDisplaysDoesNotUseStaleSnapshot(t *testing.T) {
	a, f := placementTestApp(t)
	a.displays = func() []mygo.Display { return nil }
	a.show()
	if len(f.moves) != 0 || len(a.displaySnapshot()) != 0 {
		t.Fatalf("empty snapshot reused disconnected displays: %+v %+v", f.moves, a.displaySnapshot())
	}
}

func TestTopologyRepairSurvivesAnEarlierSettingsSnapshot(t *testing.T) {
	a, _ := placementTestApp(t)
	d, f := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(d)
	f.drag(300, 300)
	a.observeJSONMove(d)
	next := placementScreens()
	next[0].WorkArea.Y += 50
	next[0].WorkArea.Height -= 50
	a.displays = func() []mygo.Display { return next }
	// Opening settings refreshes the view before the display notification.
	a.placeSettingsWindow()
	a.displaysChanged()
	if len(f.moves) != 2 || f.moves[1] != (screenPoint{X: 300, Y: 350}) || d.place.pending != nil {
		t.Fatalf("refresh swallowed JSON repair: moves=%+v state=%+v", f.moves, d.place)
	}
}

func TestMinimizedTopologyMovesWaitForRestore(t *testing.T) {
	for _, moveFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "display-notification-first", true: "move-notification-first"}[moveFirst], func(t *testing.T) {
			a, f := placementTestApp(t)
			a.show()
			d, jf := addJSONFrame(a, 700, 460)
			a.placeJSONWindow(d)
			f.drag(300, 300)
			jf.drag(400, 300)
			a.observeSettingsMove()
			a.observeJSONMove(d)
			f.minimized, jf.minimized = true, true
			next := placementScreens()
			next[0].WorkArea.Y += 50
			next[0].WorkArea.Height -= 50
			a.displays = func() []mygo.Display { return next }
			// AppKit has already changed the native frames. Keep the old origins.
			f.drag(0, 0)
			jf.drag(0, 0)
			if !moveFirst {
				a.displaysChanged()
			}
			a.observeSettingsMove()
			a.observeJSONMove(d)
			a.displaysChanged()
			if len(f.moves) != 1 || len(jf.moves) != 1 || !a.settingsPlace.needsRepair || !d.place.needsRepair {
				t.Fatalf("minimized window moved early: settings=%+v JSON=%+v", f.moves, jf.moves)
			}
			if a.settingsPlace.pending != nil || d.place.pending != nil {
				t.Fatal("topology change retained a pending seat")
			}
			a.show()
			a.focusJSONWindow(d)
			if f.restores != 1 || jf.restores != 1 || len(f.moves) != 2 || len(jf.moves) != 2 ||
				f.moves[1] != (screenPoint{X: 300, Y: 350}) || jf.moves[1] != (screenPoint{X: 400, Y: 350}) {
				t.Fatalf("restore lost old work-area offsets: settings=%+v JSON=%+v", f.moves, jf.moves)
			}
			a.saveWindowPlacesOnQuit()
			if len(a.config.WindowPositions)+len(a.config.JSONPositions) != 0 {
				t.Fatal("restoration wrote automatic positions")
			}
		})
	}
}

func TestJSONOversizedStaggerKeepsDefaultAxes(t *testing.T) {
	for _, size := range []screenSize{{Width: 1600, Height: 460}, {Width: 700, Height: 1000}, {Width: 1600, Height: 1000}} {
		a, _ := placementTestApp(t)
		display := a.displaysNow()[0]
		base := screenPlacement(display, nil, size)
		for i := 0; i < 6; i++ {
			d, f := addJSONFrame(a, size.Width, size.Height)
			a.placeJSONWindow(d)
			point := f.moves[0]
			if size.Width > display.WorkArea.Width && point.X != base.X || size.Height > display.WorkArea.Height && point.Y != base.Y {
				t.Fatalf("size=%+v window=%d point=%+v default=%+v", size, i, point, base)
			}
		}
	}
}

func TestFinalMoveCaptureAndNewestJSONCloseOrder(t *testing.T) {
	a, f := placementTestApp(t)
	a.show()
	f.drag(300, 300) // No OnMove: hide must capture the final visible frame.
	a.hide()
	if a.config.WindowPositions["1"] != (seat{X: 300, Y: 275}) {
		t.Fatal("hide lost the final move")
	}
	older, oldFrame := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(older)
	oldFrame.drag(100, 200)
	a.observeJSONMove(older)
	newer, newFrame := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(newer)
	newFrame.drag(200, 250) // Close must capture this final move too.
	a.destroyJSON(newer)
	want := map[string]seat{"1": {X: 200, Y: 225}}
	if a.rememberJSONPlace(older) || !reflect.DeepEqual(a.config.JSONPositions, want) {
		t.Fatalf("older close replaced newer seat: %+v", a.config.JSONPositions)
	}
	// No notification after automatic staggering must ever make it persistent.
	unmoved, _ := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(unmoved)
	a.destroyJSON(unmoved)
	if !reflect.DeepEqual(a.config.JSONPositions, want) {
		t.Fatal("automatic stagger became the saved seat")
	}
}

func TestJSONCloseReleasesAnUnsaturatedStaggerSlot(t *testing.T) {
	a, _ := placementTestApp(t)
	var documents []*jsonDocument
	var points []screenPoint
	for i := 0; i < 3; i++ {
		d, f := addJSONFrame(a, 700, 460)
		a.placeJSONWindow(d)
		documents, points = append(documents, d), append(points, f.moves[0])
	}
	a.destroyJSON(documents[1])
	d, f := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(d)
	if f.moves[0] != points[1] || len(a.config.JSONPositions) != 0 {
		t.Fatalf("close did not release its slot without saving: %+v seats=%+v", f.moves, a.config.JSONPositions)
	}
}

func TestPrimaryScreenChangeKeepsDIPOffsetsAboveTheOrigin(t *testing.T) {
	a, f := placementTestApp(t)
	lower := fakeDisplay(1, "Lower", 0, 0, 1440, 900)
	lower.Primary, lower.ScaleFactor = true, 1
	lower.WorkArea.Y, lower.WorkArea.Height = 25, 875
	upper := fakeDisplay(2, "Upper", 0, -1080, 1920, 1080)
	upper.ScaleFactor = 2
	upper.WorkArea.Y, upper.WorkArea.Height = -1055, 1055
	a.displays = func() []mygo.Display { return []mygo.Display{lower, upper} }
	setTestCursor(a, mygo.Point{X: 500, Y: -500})
	a.show()
	f.drag(200, -1000)
	a.observeSettingsMove()
	setTestCursor(a, mygo.Point{X: 500, Y: 500})
	d, jf := addJSONFrame(a, 700, 460)
	a.placeJSONWindow(d)
	jf.drag(300, 300)
	a.observeJSONMove(d)
	// Making the upper screen primary translates the global Y origin. Work
	// areas come from the new snapshot, with independent menu-bar insets.
	upper.Primary, lower.Primary = true, false
	upper.Bounds.Y, upper.WorkArea.Y, upper.WorkArea.Height = 0, 40, 1040
	upper.ScaleFactor = 1
	lower.Bounds.Y, lower.WorkArea.Y = 1080, 1105
	a.displaysChanged()
	if f.bounds.X != 200 || f.bounds.Y != 95 || jf.bounds.X != 300 || jf.bounds.Y != 1380 {
		t.Fatalf("primary change lost DIP offsets: settings=%+v JSON=%+v", f.bounds, jf.bounds)
	}
	a.saveWindowPlacesOnQuit()
	if len(a.config.WindowPositions)+len(a.config.JSONPositions) != 0 {
		t.Fatal("primary-screen translation saved automatic seats")
	}
}

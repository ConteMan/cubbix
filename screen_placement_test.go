package main

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo"
)

// fakeDisplay builds a display with one frame: work area and bounds are the
// same unless a case needs a menu bar.
func fakeDisplay(id int64, label string, x, y, width, height int) mygo.Display {
	return mygo.Display{
		ID:       id,
		Label:    label,
		Bounds:   mygo.Rectangle{X: x, Y: y, Width: width, Height: height},
		WorkArea: mygo.Rectangle{X: x, Y: y, Width: width, Height: height},
	}
}

func TestScreenPlacementDefaultsAndClamping(t *testing.T) {
	size := screenSize{Width: 400, Height: 300}
	main := mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}
	left := mygo.Rectangle{X: -1920, Y: 25, Width: 1920, Height: 1055}
	cases := []struct {
		name       string
		work       mygo.Rectangle
		remembered *seat
		want       screenPoint
	}{
		{"no record centres both ways", main, nil, screenPoint{X: 520, Y: 312}},
		// An odd remainder truncates towards zero, so the window sits half a
		// DIP up and left of the exact middle.
		{"odd centring truncates", mygo.Rectangle{X: 0, Y: 0, Width: 1441, Height: 901}, nil, screenPoint{X: 520, Y: 300}},
		{"a record that fits is restored", main, &seat{X: 40, Y: 96}, screenPoint{X: 40, Y: 121}},
		{"a record past the edge is clamped", main, &seat{X: 5000, Y: 5000}, screenPoint{X: 1040, Y: 600}},
		{"negative offsets survive", main, &seat{X: -20, Y: -60}, screenPoint{X: 0, Y: 25}},
		{"a display left of the main one", left, &seat{X: 100, Y: 50}, screenPoint{X: -1820, Y: 75}},
		// A work area narrower than the window ignores the remembered X and
		// centres across; it does not clamp that axis.
		{"too narrow ignores x and centres", mygo.Rectangle{X: -500, Y: 25, Width: 300, Height: 900}, &seat{X: 5000, Y: 40}, screenPoint{X: -550, Y: 65}},
		// A work area shorter than the window keeps the top edge reachable
		// instead of centring, which would put the title bar off screen.
		{"too short pins the top", mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 200}, &seat{X: 40, Y: 5000}, screenPoint{X: 40, Y: 25}},
		{"both axes too small", mygo.Rectangle{X: -500, Y: 40, Width: 300, Height: 200}, &seat{X: 10, Y: 10}, screenPoint{X: -550, Y: 40}},
	}
	for _, c := range cases {
		display := mygo.Display{ID: 1, Bounds: c.work, WorkArea: c.work}
		if got := screenPlacement(display, c.remembered, size); got != c.want {
			t.Errorf("%s: got %+v; want %+v", c.name, got, c.want)
		}
	}
	// The scale factor is a separate field: DIPs stay DIPs on Retina.
	retina := mygo.Display{ID: 1, Bounds: main, WorkArea: main, ScaleFactor: 2}
	if got, plain := screenPlacement(retina, &seat{X: 40, Y: 96}, size), screenPlacement(mygo.Display{ID: 1, Bounds: main, WorkArea: main, ScaleFactor: 1}, &seat{X: 40, Y: 96}, size); got != plain {
		t.Fatalf("scale factor changed the DIP result: %+v vs %+v", got, plain)
	}
}

func TestSeatConversion(t *testing.T) {
	work := mygo.Rectangle{X: -1920, Y: 25, Width: 1920, Height: 1055}
	p := screenPoint{X: -1800, Y: 90}
	stored := seatOffset(p, work)
	if stored != (seat{X: 120, Y: 65}) {
		t.Fatalf("seat = %+v", stored)
	}
	if got := seatPoint(stored, work); got != p {
		t.Fatalf("point = %+v", got)
	}
	if got := seatKey(int64(-42)); got != "-42" {
		t.Fatalf("negative key = %q", got)
	}
	if got := screenRect(p, screenSize{Width: 10, Height: 20}); got != (mygo.Rectangle{X: -1800, Y: 90, Width: 10, Height: 20}) {
		t.Fatalf("rect = %+v", got)
	}
	rect := mygo.Rectangle{X: 0, Y: 0, Width: 100, Height: 100}
	if !screenRectContains(rect, screenPoint{X: 99, Y: 0}) || screenRectContains(rect, screenPoint{X: 100, Y: 0}) {
		t.Fatal("screenRectContains is not half-open")
	}
}

func TestDisplayMatchingContracts(t *testing.T) {
	displays := []mygo.Display{
		fakeDisplay(1, "main", 0, 0, 1440, 900),
		fakeDisplay(2, "left", -1920, 0, 1920, 1080),
	}
	// Empty list: zero value, never a panic.
	if got := displayMatching(nil, mygo.Rectangle{Width: 10, Height: 10}); got.ID != 0 {
		t.Fatalf("empty list returned %d", got.ID)
	}
	// No overlap at all still returns the first display: 005 relies on that
	// when the panel is dragged off screen.
	if got := displayMatching(displays, mygo.Rectangle{X: 5000, Y: 5000, Width: 10, Height: 10}); got.ID != 1 {
		t.Fatalf("no overlap returned %d; want the first display", got.ID)
	}
	if got := displayMatching(displays, mygo.Rectangle{X: -100, Y: 100, Width: 640, Height: 400}); got.ID != 1 {
		t.Fatalf("straddling returned %d", got.ID)
	}
	// An exact tie keeps the earlier display.
	tie := []mygo.Display{fakeDisplay(3, "a", 0, 0, 100, 100), fakeDisplay(4, "b", 100, 0, 100, 100)}
	if got := displayMatching(tie, mygo.Rectangle{X: 90, Y: 0, Width: 20, Height: 10}); got.ID != 3 {
		t.Fatalf("tie returned %d", got.ID)
	}
	// The window-only variant refuses a rectangle with no positive overlap.
	if got, ok := windowDisplayMatching(displays, mygo.Rectangle{X: 5000, Y: 5000, Width: 10, Height: 10}); ok || got.ID != 0 {
		t.Fatalf("off-screen window matched %d, %v", got.ID, ok)
	}
	// Touching an edge is not an overlap.
	if _, ok := windowDisplayMatching(displays, mygo.Rectangle{X: 1440, Y: 0, Width: 10, Height: 10}); ok {
		t.Fatal("an edge touch counted as an overlap")
	}
	if got, ok := windowDisplayMatching(displays, mygo.Rectangle{X: 100, Y: 100, Width: 100, Height: 100}); !ok || got.ID != 1 {
		t.Fatalf("window match = %d, %v", got.ID, ok)
	}
}

func TestDisplayNearest(t *testing.T) {
	displays := []mygo.Display{
		fakeDisplay(1, "main", 0, 0, 100, 100),
		fakeDisplay(2, "left", -200, 0, 100, 100),
	}
	// Inside, in the gap and on an edge all resolve by bounds.
	for _, c := range []struct {
		name  string
		point mygo.Point
		want  int64
	}{
		{"inside", mygo.Point{X: 50, Y: 50}, 1},
		{"in the gap, nearer the left one", mygo.Point{X: -150, Y: 50}, 2},
		{"in the gap, nearer the main one", mygo.Point{X: -50, Y: 50}, 1},
		{"on the main one's left edge", mygo.Point{X: 0, Y: 50}, 1},
		{"above the main one", mygo.Point{X: 50, Y: -80}, 1},
		{"negative space below the left one", mygo.Point{X: -150, Y: 400}, 2},
	} {
		if got := displayNearest(displays, c.point); got.ID != c.want {
			t.Errorf("%s: got %d; want %d", c.name, got.ID, c.want)
		}
	}
	// Equal distance keeps the earlier display.
	tie := []mygo.Display{fakeDisplay(3, "a", 0, 0, 100, 100), fakeDisplay(4, "b", 200, 0, 100, 100)}
	if got := displayNearest(tie, mygo.Point{X: 150, Y: 50}); got.ID != 3 {
		t.Fatalf("distance tie returned %d", got.ID)
	}
	if got := displayNearest(nil, mygo.Point{}); got.ID != 0 {
		t.Fatalf("empty list returned %d", got.ID)
	}
}

func TestSeatTable(t *testing.T) {
	var nilTable seatTable
	if _, ok := nilTable.lookup(fakeDisplay(1, "main", 0, 0, 10, 10)); ok {
		t.Fatal("a nil table returned a seat")
	}
	if nilTable.store(fakeDisplay(1, "main", 0, 0, 10, 10), screenPoint{}) {
		t.Fatal("a nil table reported a change")
	}
	if nilTable.forget(fakeDisplay(1, "main", 0, 0, 10, 10)) {
		t.Fatal("a nil table forgot something")
	}

	stored := map[string]seat{}
	table := seatTable{seats: &stored}
	display := mygo.Display{ID: 7, Bounds: mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 900}, WorkArea: mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}}
	if !table.store(display, screenPoint{X: 40, Y: 121}) {
		t.Fatal("the first store did not report a change")
	}
	if got, ok := table.lookup(display); !ok || got != (seat{X: 40, Y: 96}) {
		t.Fatalf("lookup = %+v, %v", got, ok)
	}
	// The same place again changes nothing, so the caller can skip the save.
	if table.store(display, screenPoint{X: 40, Y: 121}) {
		t.Fatal("storing the same place reported a change")
	}
	// A pending seat is committed without reading the current work area.
	if !table.storeSeat(7, seat{X: -8, Y: 0}) {
		t.Fatal("storeSeat did not report a change")
	}
	if got := stored["7"]; got != (seat{X: -8, Y: 0}) {
		t.Fatalf("storeSeat stored %+v", got)
	}
	// Historical keys, including the old zero and negative ones, still read
	// and write; the settings and JSON callers skip ID 0 instead.
	if !table.storeSeat(0, seat{X: 1, Y: 2}) || !table.storeSeat(-42, seat{X: 3, Y: 4}) {
		t.Fatal("zero or negative display numbers were dropped")
	}
	if stored["0"] != (seat{X: 1, Y: 2}) || stored["-42"] != (seat{X: 3, Y: 4}) {
		t.Fatalf("old keys = %+v", stored)
	}
	if !table.forget(display) || table.forget(display) {
		t.Fatal("forget did not report one change")
	}
	// Two tables never write into each other.
	other := map[string]seat{}
	seatTable{seats: &other}.storeSeat(7, seat{X: 9, Y: 9})
	if other["7"] == stored["7"] || len(stored) != 2 {
		t.Fatalf("tables leaked: %+v %+v", stored, other)
	}
	// A nil map behind a non-nil pointer is allocated on the first store.
	var fresh map[string]seat
	if !(seatTable{seats: &fresh}).storeSeat(1, seat{X: 1, Y: 1}) {
		t.Fatal("a nil map was not allocated")
	}
	if fresh["1"] != (seat{X: 1, Y: 1}) {
		t.Fatalf("fresh map = %+v", fresh)
	}
}

func TestSeatTableMatchesTheConfig(t *testing.T) {
	config := Config{}
	config.putPaletteSeat("1", seat{X: 12, Y: 34})
	if !reflect.DeepEqual(config.PalettePositions, map[string]seat{"1": {X: 12, Y: 34}}) {
		t.Fatalf("seats = %+v", config.PalettePositions)
	}
	if got := (seatTable{seats: &config.PalettePositions}); len(*got.seats) != 1 {
		t.Fatalf("wrapper wrote %+v", *got.seats)
	}
}

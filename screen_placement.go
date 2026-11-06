package main

import (
	"math"
	"strconv"

	"github.com/egoist/mygo"
)

// screenPoint is a position in screen DIPs: the display space whose origin is
// the main display's top-left corner. Windows, displays and the pointer all
// share it, so a display left of or above the main one has negative
// coordinates.
type screenPoint struct{ X, Y int }

// screenSize describes the object being placed: a palette panel, which is
// smaller than its window, or a window's outer frame.
type screenSize struct{ Width, Height int }

// seat is where an object sits on one display: the offset of its top-left
// corner from that display's work area origin. Offsets survive a display
// moving, an absolute position would not.
type seat struct {
	X int `toml:"x"`
	Y int `toml:"y"`
}

// seatKey is the config key of a display: its runtime display number, never a
// serial number or a name.
func seatKey(id int64) string { return strconv.FormatInt(id, 10) }

// seatOffset turns a screen position into the offset stored for a display.
func seatOffset(p screenPoint, work mygo.Rectangle) seat {
	return seat{X: p.X - work.X, Y: p.Y - work.Y}
}

// seatPoint turns a stored offset back into a screen position.
func seatPoint(s seat, work mygo.Rectangle) screenPoint {
	return screenPoint{X: work.X + s.X, Y: work.Y + s.Y}
}

// screenRect is an object's rectangle for a position.
func screenRect(p screenPoint, size screenSize) mygo.Rectangle {
	return mygo.Rectangle{X: p.X, Y: p.Y, Width: size.Width, Height: size.Height}
}

// screenRectContains reports whether p is inside r.
func screenRectContains(r mygo.Rectangle, p screenPoint) bool {
	return p.X >= r.X && p.Y >= r.Y && p.X < r.X+r.Width && p.Y < r.Y+r.Height
}

// clampToWorkArea keeps an object inside the work area. An axis the object
// cannot fit on is left where the caller put it: the caller's own fallback
// then decides, and a drag keeps the pointer's choice.
func clampToWorkArea(p screenPoint, size screenSize, work mygo.Rectangle) screenPoint {
	if work.Width >= size.Width {
		p.X = clampInt(p.X, work.X, work.X+work.Width-size.Width)
	}
	if work.Height >= size.Height {
		p.Y = clampInt(p.Y, work.Y, work.Y+work.Height-size.Height)
	}
	return p
}

// centeredPoint is the default position on a display: centred both ways, with
// Go's truncating integer division deciding odd remainders.
func centeredPoint(display mygo.Display, size screenSize) screenPoint {
	work := display.WorkArea
	return screenPoint{
		X: work.X + (work.Width-size.Width)/2,
		Y: work.Y + (work.Height-size.Height)/2,
	}
}

// displayMatching returns the display overlapping r the most, the way
// Screen.DisplayMatching does, from a list the caller supplies so tests need
// no app and no real screen. A list with no overlap at all still returns its
// first display: palette placement relies on that.
func displayMatching(displays []mygo.Display, r mygo.Rectangle) mygo.Display {
	best, bestArea := mygo.Display{}, -1
	for _, d := range displays {
		w := min(d.Bounds.X+d.Bounds.Width, r.X+r.Width) - max(d.Bounds.X, r.X)
		h := min(d.Bounds.Y+d.Bounds.Height, r.Y+r.Height) - max(d.Bounds.Y, r.Y)
		if area := max(w, 0) * max(h, 0); area > bestArea {
			best, bestArea = d, area
		}
	}
	return best
}

// windowDisplayMatching is the window-only variant: a rectangle with no
// positive overlap belongs to no display, and the caller decides what to do
// with that. Changing displayMatching instead would change 005's palette
// behavior.
func windowDisplayMatching(displays []mygo.Display, r mygo.Rectangle) (mygo.Display, bool) {
	best, bestArea := mygo.Display{}, 0
	for _, d := range displays {
		w := min(d.Bounds.X+d.Bounds.Width, r.X+r.Width) - max(d.Bounds.X, r.X)
		h := min(d.Bounds.Y+d.Bounds.Height, r.Y+r.Height) - max(d.Bounds.Y, r.Y)
		if area := max(w, 0) * max(h, 0); area > bestArea {
			best, bestArea = d, area
		}
	}
	return best, bestArea > 0
}

// displayNearest returns the display closest to p, as
// Screen.DisplayNearestPoint does: by bounds, not by work area, and the
// earlier display wins a tie.
func displayNearest(displays []mygo.Display, p mygo.Point) mygo.Display {
	best, bestDistance := mygo.Display{}, math.Inf(1)
	for _, d := range displays {
		b := d.Bounds
		dx := math.Max(math.Max(float64(b.X-p.X), 0), float64(p.X-(b.X+b.Width)))
		dy := math.Max(math.Max(float64(b.Y-p.Y), 0), float64(p.Y-(b.Y+b.Height)))
		if distance := dx*dx + dy*dy; distance < bestDistance {
			best, bestDistance = d, distance
		}
	}
	return best
}

// screenPlacement is where a window opens on a display: the remembered offset
// when it has one and the display can hold that axis, centred otherwise. An
// axis the display is too small for keeps the window's top-left reachable
// instead of centring it, so the user can still drag it.
//
// 005's palette does not use this: its default is the top of the work area,
// not the middle.
func screenPlacement(display mygo.Display, remembered *seat, size screenSize) screenPoint {
	p := centeredPoint(display, size)
	if display.WorkArea.Height < size.Height {
		p.Y = display.WorkArea.Y
	}
	if remembered != nil {
		at := seatPoint(*remembered, display.WorkArea)
		if display.WorkArea.Width >= size.Width {
			p.X = at.X
		}
		if display.WorkArea.Height >= size.Height {
			p.Y = at.Y
		}
	}
	return clampToWorkArea(p, size, display.WorkArea)
}

// displaysNow is one fresh snapshot of the connected displays. Placement reads
// it when a window opens or closes; the view must not, because a frame cannot
// wait for the main thread.
func (a *application) displaysNow() []mygo.Display {
	if a.displays == nil {
		return nil
	}
	return a.displays()
}

// cursorNow is the pointer's screen position.
func (a *application) cursorNow() mygo.Point {
	if a.cursorPoint == nil {
		return mygo.Point{}
	}
	return a.cursorPoint()
}

// seatTable reads and writes one per-display position table. It never saves:
// the caller decides when the configuration reaches disk, so a callback that
// fires per frame cannot write the file.
type seatTable struct{ seats *map[string]seat }

// lookup reads a display's seat. A nil table holds nothing and allocates
// nothing.
func (t seatTable) lookup(display mygo.Display) (seat, bool) {
	if t.seats == nil || *t.seats == nil {
		return seat{}, false
	}
	value, ok := (*t.seats)[seatKey(display.ID)]
	return value, ok
}

// store remembers a screen position. It reports whether the table changed.
func (t seatTable) store(display mygo.Display, p screenPoint) bool {
	return t.storeSeat(display.ID, seatOffset(p, display.WorkArea))
}

// storeSeat commits a seat captured earlier, without reading the display's
// current work area again: a pending seat describes the layout at the moment
// the user moved the window.
func (t seatTable) storeSeat(id int64, value seat) bool {
	if t.seats == nil {
		return false
	}
	if *t.seats == nil {
		*t.seats = map[string]seat{}
	}
	key := seatKey(id)
	if current, ok := (*t.seats)[key]; ok && current == value {
		return false
	}
	(*t.seats)[key] = value
	return true
}

// forget drops a display's seat. It reports whether the table changed.
func (t seatTable) forget(display mygo.Display) bool {
	if t.seats == nil || *t.seats == nil {
		return false
	}
	key := seatKey(display.ID)
	if _, ok := (*t.seats)[key]; !ok {
		return false
	}
	delete(*t.seats, key)
	return true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func clampInt(v, low, high int) int {
	return min(max(v, low), high)
}

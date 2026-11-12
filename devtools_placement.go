package main

// devtoolsGeometry is one display's seat for the devtools window: the offset
// from the work area's top-left corner plus the size the user gave it.
type devtoolsGeometry struct {
	X      int `toml:"x"`
	Y      int `toml:"y"`
	Width  int `toml:"width"`
	Height int `toml:"height"`
}

// devtoolsWindow is what the placement code needs of the frame. Tests replace
// it with a fake, so no case creates a native window.
type devtoolsWindow interface {
	placementWindow
	SetContentSize(int, int)
	SetAlwaysOnTop(bool)
	Destroy()
}

func (s *devtoolsState) frame() devtoolsWindow {
	if s.testFrame != nil {
		return s.testFrame
	}
	if s.window != nil {
		return s.window
	}
	return nil
}

func (a *application) devtoolsSeats() seatTable {
	seats := make(map[string]seat, len(a.config.DevtoolsWindows))
	for key, geometry := range a.config.DevtoolsWindows {
		seats[key] = seat{X: geometry.X, Y: geometry.Y}
	}
	return seatTable{seats: &seats}
}

// placeDevtools puts the hidden window where the display under the pointer
// remembers it, before it is shown.
func (a *application) placeDevtools() {
	s := a.devtools
	if s == nil || s.frame() == nil {
		return
	}
	displays := a.displaysNow()
	display, found := a.jsonReselect(displays)
	if !found || display.ID == 0 {
		return
	}
	window := s.frame()
	t := tokens(false, "zh-CN")
	options := a.devtoolsWindowOptions(t, "zh-CN")
	width, height := options.Width, options.Height
	if geometry, ok := a.config.DevtoolsWindows[seatKey(display.ID)]; ok {
		width, height = max(options.MinWidth, geometry.Width), max(options.MinHeight, geometry.Height)
	}
	window.SetContentSize(width, height)
	s.initialSize = windowSize(window)
	a.moveWindowTo(&s.place, window, seatPointForWindow(window, display, a.devtoolsSeats()))
	s.place.display = display
}

func (a *application) reconcileDevtoolsDisplay() {
	if s := a.devtools; s != nil && s.frame() != nil {
		a.reconcilePlacementIfNeeded(&s.place, s.frame(), a.displaysNow(), a.jsonReselect, a.devtoolsSeats())
	}
}

func (a *application) observeDevtoolsMove() {
	if s := a.devtools; s != nil && s.frame() != nil {
		a.observeMove(&s.place, s.frame(), a.displaysNow(), a.jsonReselect, a.devtoolsSeats())
	}
}

// rememberDevtoolsPlace stores the window's seat on the display it is on. It is
// read while the frame is still alive, and an automatic placement never writes.
func (a *application) rememberDevtoolsPlace() bool {
	s := a.devtools
	if s == nil || s.frame() == nil || !s.visible || s.place.needsRepair {
		return false
	}
	displays := a.displaysNow()
	if len(displays) == 0 || a.topologyChanged(&s.place, displays) {
		return false
	}
	a.observeDevtoolsMove()
	bounds := s.frame().Bounds()
	display, found := windowDisplayMatching(displays, bounds)
	if !found || display.ID == 0 {
		return false
	}
	key := seatKey(display.ID)
	previous, exists := a.config.DevtoolsWindows[key]
	_, pending, _, moved := takePending(&s.place)
	resized := !s.frame().IsMinimized() && windowSize(s.frame()) != s.initialSize
	if !moved && !resized {
		return false
	}
	offset := seatOffset(screenPoint{X: bounds.X, Y: bounds.Y}, display.WorkArea)
	if moved {
		offset = pending
	} else if exists {
		offset = seat{X: previous.X, Y: previous.Y}
	}
	geometry := devtoolsGeometry{X: offset.X, Y: offset.Y, Width: bounds.Width, Height: bounds.Height}
	if exists && previous == geometry {
		return false
	}
	if a.config.DevtoolsWindows == nil {
		a.config.DevtoolsWindows = map[string]devtoolsGeometry{}
	}
	a.config.DevtoolsWindows[key] = geometry
	cancelPending(&s.place)
	return true
}

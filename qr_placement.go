package main

type qrGeometry struct {
	X      int `toml:"x"`
	Y      int `toml:"y"`
	Width  int `toml:"width"`
	Height int `toml:"height"`
}

type qrWindow interface {
	placementWindow
	SetContentSize(int, int)
	SetAlwaysOnTop(bool)
	Destroy()
}

func (s *qrState) frame() qrWindow {
	if s.testFrame != nil {
		return s.testFrame
	}
	if s.window != nil {
		return s.window
	}
	return nil
}

func (a *application) qrSeats() seatTable {
	seats := make(map[string]seat, len(a.config.QRWindows))
	for key, geometry := range a.config.QRWindows {
		seats[key] = seat{X: geometry.X, Y: geometry.Y}
	}
	return seatTable{seats: &seats}
}

// placeQR puts the hidden window where the display under the pointer
// remembers it, before it is shown.
func (a *application) placeQR() {
	s := a.qr
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
	options := a.qrWindowOptions(t, "zh-CN")
	width, height := options.Width, options.Height
	if geometry, ok := a.config.QRWindows[seatKey(display.ID)]; ok {
		width, height = max(options.MinWidth, geometry.Width), max(options.MinHeight, geometry.Height)
	}
	window.SetContentSize(width, height)
	s.initialSize = windowSize(window)
	a.moveWindowTo(&s.place, window, seatPointForWindow(window, display, a.qrSeats()))
	s.place.display = display
}

func (a *application) reconcileQRDisplay() {
	if s := a.qr; s != nil && s.frame() != nil {
		a.reconcilePlacementIfNeeded(&s.place, s.frame(), a.displaysNow(), a.jsonReselect, a.qrSeats())
	}
}

func (a *application) observeQRMove() {
	if s := a.qr; s != nil && s.frame() != nil {
		a.observeMove(&s.place, s.frame(), a.displaysNow(), a.jsonReselect, a.qrSeats())
	}
}

// rememberQRPlace stores the window's seat on the display it is on. It is read
// while the frame is still alive, and an automatic placement never writes.
func (a *application) rememberQRPlace() bool {
	s := a.qr
	if s == nil || s.frame() == nil || !s.visible || s.place.needsRepair {
		return false
	}
	displays := a.displaysNow()
	if len(displays) == 0 || a.topologyChanged(&s.place, displays) {
		return false
	}
	a.observeQRMove()
	bounds := s.frame().Bounds()
	display, found := windowDisplayMatching(displays, bounds)
	if !found || display.ID == 0 {
		return false
	}
	key := seatKey(display.ID)
	previous, exists := a.config.QRWindows[key]
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
	geometry := qrGeometry{X: offset.X, Y: offset.Y, Width: bounds.Width, Height: bounds.Height}
	if exists && previous == geometry {
		return false
	}
	if a.config.QRWindows == nil {
		a.config.QRWindows = map[string]qrGeometry{}
	}
	a.config.QRWindows[key] = geometry
	cancelPending(&s.place)
	return true
}

// qrDisplayScaleFactor is the scale factor of the display the window is on, so
// the bitmap can be built from whole device pixels. A window that is not on any
// display yet uses the one it is about to open on.
func (a *application) qrDisplayScaleFactor() float64 {
	s := a.qr
	if s == nil {
		return 1
	}
	displays := a.displaysNow()
	if len(displays) == 0 {
		return 1
	}
	if s.frame() != nil {
		if display, ok := windowDisplayMatching(displays, s.frame().Bounds()); ok && display.ScaleFactor > 0 {
			return display.ScaleFactor
		}
	}
	if s.place.display.ID != 0 {
		if display, ok := displayByID(displays, s.place.display.ID); ok && display.ScaleFactor > 0 {
			return display.ScaleFactor
		}
	}
	if display := displayNearest(displays, a.cursorNow()); display.ScaleFactor > 0 {
		return display.ScaleFactor
	}
	return 1
}

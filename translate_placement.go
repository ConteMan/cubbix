package main

type translateGeometry struct {
	X      int `toml:"x"`
	Y      int `toml:"y"`
	Width  int `toml:"width"`
	Height int `toml:"height"`
}

type translateWindow interface {
	placementWindow
	SetContentSize(int, int)
	SetAlwaysOnTop(bool)
	Destroy()
}

func (s *translateState) frame() translateWindow {
	if s.testFrame != nil {
		return s.testFrame
	}
	if s.window != nil {
		return s.window
	}
	return nil
}

func (a *application) translateSeats() seatTable {
	seats := make(map[string]seat, len(a.config.TranslateWindows))
	for key, geometry := range a.config.TranslateWindows {
		seats[key] = seat{X: geometry.X, Y: geometry.Y}
	}
	return seatTable{seats: &seats}
}

func (a *application) placeTranslate() {
	s := a.translate
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
	options := a.translateWindowOptions(t, "zh-CN")
	width, height := options.Width, options.Height
	if geometry, ok := a.config.TranslateWindows[seatKey(display.ID)]; ok {
		width, height = max(options.MinWidth, geometry.Width), max(options.MinHeight, geometry.Height)
	}
	window.SetContentSize(width, height)
	s.initialSize = windowSize(window)
	a.moveWindowTo(&s.place, window, seatPointForWindow(window, display, a.translateSeats()))
	s.place.display = display
}

func (a *application) reconcileTranslateDisplay() {
	if s := a.translate; s != nil && s.frame() != nil {
		a.reconcilePlacementIfNeeded(&s.place, s.frame(), a.displaysNow(), a.jsonReselect, a.translateSeats())
	}
}

func (a *application) observeTranslateMove() {
	if s := a.translate; s != nil && s.frame() != nil {
		a.observeMove(&s.place, s.frame(), a.displaysNow(), a.jsonReselect, a.translateSeats())
	}
}

func (a *application) rememberTranslatePlace() bool {
	s := a.translate
	if s == nil || s.frame() == nil || !s.visible || s.place.needsRepair {
		return false
	}
	displays := a.displaysNow()
	if len(displays) == 0 || a.topologyChanged(&s.place, displays) {
		return false
	}
	a.observeTranslateMove()
	bounds := s.frame().Bounds()
	display, found := windowDisplayMatching(displays, bounds)
	if !found || display.ID == 0 {
		return false
	}
	key := seatKey(display.ID)
	previous, exists := a.config.TranslateWindows[key]
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
	geometry := translateGeometry{X: offset.X, Y: offset.Y, Width: bounds.Width, Height: bounds.Height}
	if exists && previous == geometry {
		return false
	}
	if a.config.TranslateWindows == nil {
		a.config.TranslateWindows = map[string]translateGeometry{}
	}
	a.config.TranslateWindows[key] = geometry
	cancelPending(&s.place)
	return true
}

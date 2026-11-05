package main

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// polishPalette frames the palette once, runs setup, and frames again, so the
// checks read the geometry of a settled frame.
func polishPalette(t *testing.T, locale string, dark bool, setup func(*application, *ui.Tester)) (*application, *ui.Tester) {
	t.Helper()
	a := paletteTestApp(t)
	tt := ui.NewTester(a.paletteView, paletteWindowWidth(), paletteWindowHeight())
	tt.SetScale(2)
	tt.SetLocale(locale)
	tt.SetDark(dark)
	tt.Frame()
	if setup != nil {
		setup(a, tt)
	}
	tt.Frame()
	return a, tt
}

// pixelOf returns the colour of a DIP of the last frame.
func pixelOf(tt *ui.Tester, x, y float32) color.RGBA {
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / float32(paletteWindowWidth())
	r, g, b, a := img.At(int(x*scale), int(y*scale)).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

// inkBefore returns the rightmost DIP of x0 up to x1 at y whose colour is not
// background, and whether any was found.
func inkBefore(tt *ui.Tester, x0, x1, y float32, background color.RGBA) (float32, bool) {
	last, found := x0, false
	for x := x0; x < x1; x += 0.5 {
		if pixelOf(tt, x, y) != background {
			last, found = x, true
		}
	}
	return last, found
}

// inkStart returns the first DIP of x0 up to x1 at y whose colour is not
// background, or -1.
func inkStart(tt *ui.Tester, x0, x1, y float32, background color.RGBA) float32 {
	for x := x0; x < x1; x += 0.5 {
		if pixelOf(tt, x, y) != background {
			return x
		}
	}
	return -1
}

// inkEnd returns the last DIP of x0 up to x1 at y whose colour is not
// background, or -1.
func inkEnd(tt *ui.Tester, x0, x1, y float32, background color.RGBA) float32 {
	for x := x1; x > x0; x -= 0.5 {
		if pixelOf(tt, x, y) != background {
			return x
		}
	}
	return -1
}

// luminance is the WCAG relative luminance of a colour.
func luminance(c color.RGBA) float64 {
	channel := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(c.R) + 0.7152*channel(c.G) + 0.0722*channel(c.B)
}

// contrastRatio is the WCAG ratio between two colours.
func contrastRatio(a, b color.RGBA) float64 {
	first, second := luminance(a), luminance(b)
	if first < second {
		first, second = second, first
	}
	return (first + 0.05) / (second + 0.05)
}

// rowsRight is where the rows end: the list fills the panel's content box, so
// it is one panel padding short of the panel's right edge.
func rowsRight(a *application) float32 {
	list := a.palette.listBounds
	return list.X + list.W
}

// firstOf returns the first DIP of x0 up to x1 at y whose colour is want, or
// -1.
func firstOf(tt *ui.Tester, x0, x1, y float32, want color.RGBA) float32 {
	for x := x0; x < x1; x += 0.5 {
		if pixelOf(tt, x, y) == want {
			return x
		}
	}
	return -1
}

// lastOf returns the last DIP of x0 up to x1 at y whose colour is want, or -1.
func lastOf(tt *ui.Tester, x0, x1, y float32, want color.RGBA) float32 {
	for x := x1; x > x0; x -= 0.5 {
		if pixelOf(tt, x, y) == want {
			return x
		}
	}
	return -1
}

// inkBottom returns the lowest DIP between y0 and y1 that shows ink at some x
// between x0 and x1, or y0-1.
func inkBottom(tt *ui.Tester, x0, x1, y0, y1 float32, background color.RGBA) float32 {
	lowest := y0 - 1
	for y := y0; y < y1; y += 0.5 {
		if inkStart(tt, x0, x1, y, background) >= 0 {
			lowest = y
		}
	}
	return lowest
}

// testPaletteHighlightIsSymmetric checks the selected row's highlight with the
// scroll bar hidden: the same inset from the panel on both sides, and the row's
// icon and hint the same distance inside it.
func testPaletteHighlightIsSymmetric(t *testing.T, locale string, dark bool) {
	tk := tokens(dark, locale)
	a, tt := polishPalette(t, locale, dark, nil)
	panel, list := a.palette.panelBounds, a.palette.listBounds
	row, ok := tt.Find(a.text(locale, "R05Start"))
	if !ok {
		t.Fatalf("the selected row is missing: %v", tt.Texts())
	}
	y := row.Y + row.H/2
	highlight := pixelOf(tt, list.X+tk.Space2, y)
	left := firstOf(tt, panel.X+tk.Space2, list.X+tk.Space8, y, highlight)
	right := lastOf(tt, list.X, panel.X+panel.W, y, highlight)
	if left < 0 || right < 0 {
		t.Fatalf("no highlight at %.1f: %v", y, tt.Texts())
	}
	leftInset, rightInset := left-panel.X, panel.X+panel.W-right
	if leftInset < tk.Space8 || rightInset < tk.Space8 {
		t.Fatalf("the highlight runs into the panel's padding: left %.1f, right %.1f", leftInset, rightInset)
	}
	if math.Abs(float64(leftInset-rightInset)) > 1 {
		t.Fatalf("the highlight is lopsided: %.1f in from the left, %.1f from the right", leftInset, rightInset)
	}
	// Nothing is reserved on the right for the scroll bar: at rest the panel's
	// own surface shows beside the rows.
	barLeft := rowsRight(a) + tk.SizePaletteScrollbarOverhang - tk.SizeScrollbar - 3
	if got, surface := pixelOf(tt, barLeft+tk.SizeScrollbar/2, y), pixelOf(tt, panel.X+tk.Space2, y); got != surface {
		t.Fatalf("a scroll bar shows beside the rows at rest: %v, panel %v", got, surface)
	}
	// The row's own insets match: the icon starts and the hint ends one
	// Space8 inside the highlight.
	icon := inkStart(tt, left+0.5, left+tk.Space8+tk.SizeIcon+2, y, highlight)
	if icon < 0 {
		t.Fatalf("the row shows no icon ink at %.1f", y)
	}
	entry := a.palette.rows[a.palette.selected]
	hint, ok := tt.Find(entry.Hint)
	if entry.Hint == "" || !ok {
		t.Fatalf("the selected row shows no hint %q: %v", entry.Hint, tt.Texts())
	}
	inner, outer := icon-left, right-(hint.X+hint.W)
	if math.Abs(float64(inner-outer)) > 1.5 {
		t.Fatalf("the row's insets are %.1f inside the icon, %.1f after the hint", inner, outer)
	}
}

// testPaletteScrollbarStaysClearOfRows checks the bar while the list is hovered
// and scrolled: it sits in the panel's margin, past the highlight and past the
// hint, and the rows keep their full width.
func testPaletteScrollbarStaysClearOfRows(t *testing.T, locale string, dark bool) {
	tk := tokens(dark, locale)
	a, tt := polishPalette(t, locale, dark, func(a *application, tt *ui.Tester) {
		// Walk down the long list: the choice scrolls into view, the list
		// overflows, and the pointer on it shows the bar.
		list := a.palette.listBounds
		tt.Move(list.X+list.W/2, list.Y+list.H/2)
		for i := 0; i < 12; i++ {
			tt.Key(0, ui.KeyDown)
		}
	})
	panel, list := a.palette.panelBounds, a.palette.listBounds
	right := rowsRight(a)
	barLeft := right + tk.SizePaletteScrollbarOverhang - tk.SizeScrollbar - 3
	barRight := barLeft + tk.SizeScrollbar
	if barLeft < right-0.5 {
		t.Fatalf("the bar starts at %.1f, inside the rows ending at %.1f", barLeft, right)
	}
	if barRight > panel.X+panel.W-1 {
		t.Fatalf("the bar ends at %.1f, past the panel's %.1f", barRight, panel.X+panel.W)
	}
	entry := a.palette.rows[a.palette.selected]
	if entry.Kind != paletteRowCommand {
		t.Fatalf("the choice landed on a %v row", entry.Kind)
	}
	row, ok := tt.Find(entry.Title)
	if !ok || row.Y < list.Y || row.Y+row.H > list.Y+list.H {
		t.Fatalf("the chosen row %q is not in view: %+v, list %+v", entry.Title, row, list)
	}
	y := row.Y + row.H/2
	surface := pixelOf(tt, panel.X+tk.Space2, y)
	highlight := pixelOf(tt, list.X+tk.Space2, y)
	// The rows and their highlight end where the panel's padding starts, and
	// the bar begins there rather than over them.
	last := lastOf(tt, list.X, panel.X+panel.W, y, highlight)
	if last > barLeft {
		t.Fatalf("the highlight reaches %.1f, under the bar at %.1f", last, barLeft)
	}
	if last < right-1 {
		t.Fatalf("the highlight ends at %.1f, short of the rows' edge %.1f", last, right)
	}
	// The bar is there, in the margin: the thumb's column shows its colour
	// somewhere beside the rows.
	bar := float32(-1)
	for probe := list.Y + 2; probe < list.Y+list.H-2; probe += 1 {
		if pixelOf(tt, barLeft+tk.SizeScrollbar/2, probe) != surface {
			bar = probe
			break
		}
	}
	if bar < 0 {
		t.Fatalf("no scroll bar in the margin beside the rows: %v", tt.Texts())
	}
	// A hint ends a whole Space8 before the rows' edge, so the bar cannot
	// cover it.
	if entry.Hint != "" {
		hint, ok := tt.Find(entry.Hint)
		if !ok {
			t.Fatalf("the chosen row shows no hint %q: %v", entry.Hint, tt.Texts())
		}
		if hint.X+hint.W > right-tk.Space8+0.5 {
			t.Fatalf("the hint reaches %.1f, into the row's right padding at %.1f", hint.X+hint.W, right-tk.Space8)
		}
	}
}

// testPaletteHintStaysInsideTheHighlight checks that a row's hint ends inside
// the highlight's right padding, as far from the edge as the icon on the left.
func testPaletteHintStaysInsideTheHighlight(t *testing.T, locale string, dark bool) {
	tk := tokens(dark, locale)
	a, tt := polishPalette(t, locale, dark, func(a *application, tt *ui.Tester) {
		tt.Type(a.text(locale, "R05Start") + " 45m")
	})
	hint, ok := tt.Find(fmt.Sprintf(a.text(locale, "PaletteMinutes"), 45))
	if !ok {
		t.Fatalf("the selected row shows no hint: %v", tt.Texts())
	}
	if right := rowsRight(a) - tk.Space8; hint.X+hint.W > right+0.5 {
		t.Fatalf("the hint reaches %.1f, past the row's padding edge %.1f", hint.X+hint.W, right)
	}
}

// TestPaletteRowsNameTheirTile checks the secondary text after each title: the
// tile a command belongs to, one gap after the title's ink.
func testPaletteRowsNameTheirTile(t *testing.T, locale string, dark bool) {
	tk := tokens(dark, locale)
	a, tt := polishPalette(t, locale, dark, nil)
	list := a.palette.listBounds
	row, ok := tt.Find(a.text(locale, "R05Start"))
	if !ok {
		t.Fatalf("the selected row is missing: %v", tt.Texts())
	}
	category := a.text(locale, "Awake")
	box, ok := tt.Find(category)
	if !ok {
		t.Fatalf("%q shows no category %q: %v", a.text(locale, "R05Start"), category, tt.Texts())
	}
	if box.Y < row.Y || box.Y+box.H > row.Y+row.H {
		t.Fatalf("the category at %.1f is not on the selected row %.1f", box.Y, row.Y)
	}
	y := box.Y + box.H/2
	highlight := pixelOf(tt, rowsRight(a)-60, row.Y+row.H/2)
	last, found := inkBefore(tt, list.X+tk.Space8+tk.SizeIcon+tk.Space8, box.X-0.5, y, highlight)
	if !found {
		t.Fatal("the row shows no title ink")
	}
	if gap := box.X - last; gap < tk.Space8-1 {
		t.Fatalf("the title ends %.1f before its category, want %.0f", gap, tk.Space8)
	}
	// The category keeps its words and stays inside the rows.
	if box.X+box.W > rowsRight(a)+0.5 {
		t.Fatalf("the category overflows the rows: %.1f", box.X+box.W)
	}
	// It is a step smaller than the title, so the pair reads as two levels.
	var small, body float32
	measuring := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		small, _ = c.MeasureText(0, ui.Span{Text: category, Size: tk.FontSizeSmall})
		body, _ = c.MeasureText(0, ui.Span{Text: category, Size: tk.FontSizeBody})
	}, paletteWindowWidth(), paletteWindowHeight())
	measuring.SetScale(2)
	measuring.SetLocale(locale)
	measuring.Frame()
	if small >= body {
		t.Fatalf("the small size measures %.1f, not less than the body's %.1f", small, body)
	}
	if math.Abs(float64(box.W-small)) > 0.5 {
		t.Fatalf("the category is %.1f wide, the small size's %.1f, the body's %.1f", box.W, small, body)
	}
	// It sits on the title's baseline, not centred on it: the last ink of the
	// two lines is at the same height.
	titleBottom := inkBottom(tt, list.X+tk.Space8+tk.SizeIcon+tk.Space8, box.X-tk.Space8-1, row.Y, row.Y+row.H, highlight)
	categoryBottom := inkBottom(tt, box.X, box.X+box.W, row.Y, row.Y+row.H, highlight)
	if titleBottom < 0 || categoryBottom < 0 {
		t.Fatalf("the row shows no ink: title %.1f, category %.1f", titleBottom, categoryBottom)
	}
	if math.Abs(float64(titleBottom-categoryBottom)) > 1.5 {
		t.Fatalf("the category ends at %.1f, the title at %.1f: not on one baseline", categoryBottom, titleBottom)
	}
	// General commands belong to the app, not to a tile.
	if !tt.HasText(a.text(locale, "R05App")) || !tt.HasText(a.text(locale, "JSON")) {
		t.Fatalf("command categories are missing: %v", tt.Texts())
	}
}

// TestPaletteLongTitleEllipsizes checks that a long title gives way, with an
// ellipsis, before the category and the hint, and stays inside the rows.
func testPaletteLongTitleEllipsizes(t *testing.T, locale string, dark bool) {
	tk := tokens(dark, locale)
	a := paletteTestApp(t)
	long := strings.Repeat("very-long-json-document-name-", 8) + ".json"
	a.addJSONDocument(`{}`, long, true)
	a.palette.query, a.palette.lastQuery = long, ""
	var categoryWidth float32
	tt := ui.NewTester(func(c *ui.Context) {
		a.paletteView(c)
		categoryWidth, _ = c.MeasureText(0, ui.Span{Text: a.text(locale, "JSON"), Size: tk.FontSizeSmall})
	}, paletteWindowWidth(), paletteWindowHeight())
	tt.SetScale(2)
	tt.SetLocale(locale)
	tt.SetDark(dark)
	tt.Frame()
	row, ok := tt.Find(long)
	if !ok {
		t.Fatalf("the long title is not listed: %v", tt.Texts())
	}
	list := a.palette.listBounds
	right := rowsRight(a)
	category, ok := tt.Find(a.text(locale, "JSON"))
	if !ok {
		t.Fatalf("the long row shows no category: %v", tt.Texts())
	}
	if category.Y < row.Y || category.Y+category.H > row.Y+row.H {
		t.Fatalf("the category at %.1f is not on the long row %.1f", category.Y, row.Y)
	}
	highlight := pixelOf(tt, right-2, row.Y+row.H/2)
	last, found := inkBefore(tt, list.X+tk.Space8+tk.SizeIcon+tk.Space8, category.X-0.5, category.Y+category.H/2, highlight)
	if !found {
		t.Fatal("the long title shows no ink")
	}
	if gap := category.X - last; gap < tk.Space8-1 {
		t.Fatalf("the long title leaves %.1f before its category, want %.0f", gap, tk.Space8)
	}
	if last > right {
		t.Fatalf("the long title reaches %.1f, past the rows' edge %.1f", last, right)
	}
	if category.X+category.W > right+0.5 {
		t.Fatalf("the category overflows the rows: %.1f", category.X+category.W)
	}
	if category.W < categoryWidth-0.5 {
		t.Fatalf("category shrank to %.1f, need %.1f to show it in full", category.W, categoryWidth)
	}
	paletteReviewScreenshot(t, tt, "palette-long-title", locale, dark)
}

// TestPaletteKeycapsAreSeparateKeys checks that ⌘ and K are drawn in keycaps of
// their own with a gap between them, not as one run of text.
func testPaletteKeycapsAreSeparateKeys(t *testing.T, locale string, dark bool) {
	tk := tokens(dark, locale)
	a, tt := polishPalette(t, locale, dark, nil)
	command, ok := tt.Find("⌘")
	if !ok {
		t.Fatalf("no ⌘ keycap: %v", tt.Texts())
	}
	key, ok := tt.Find("K")
	if !ok {
		t.Fatalf("no K keycap: %v", tt.Texts())
	}
	if key.X < command.X {
		t.Fatalf("the keys are out of order: %v %v", command, key)
	}
	if gap := key.X - (command.X + command.W); gap < tk.Space4-0.5 {
		t.Fatalf("the keycap texts are %.1f apart, want %.0f at least", gap, tk.Space4)
	}
	// Each key has a cap of its own: the keycap colour inside, the search row
	// between them.
	y := command.Y + command.H/2
	field := pixelOf(tt, a.palette.searchBounds.X+tk.Space8+tk.SizeIcon+tk.SizeIcon, y)
	if cap := pixelOf(tt, command.X-2, y); cap == field {
		t.Fatal("the key shows no keycap of its own")
	}
	if between := pixelOf(tt, (command.X+command.W+key.X)/2, y); between != field {
		t.Fatalf("the two keys share one cap: between = %v, row = %v", between, field)
	}
}

// TestPaletteSearchHasNoDivider checks the band between the search row and the
// list: the panel surface only, no separator line.
func testPaletteSearchHasNoDivider(t *testing.T, locale string, dark bool) {
	a, tt := polishPalette(t, locale, dark, nil)
	search, list := a.palette.searchBounds, a.palette.listBounds
	if list.Y <= search.Y+search.H {
		t.Fatalf("the list starts at %.1f, on the search row %.1f+%.1f", list.Y, search.Y, search.H)
	}
	surface := pixelOf(tt, list.X+list.W/2, search.Y+search.H+1)
	for y := search.Y + search.H + 1; y < list.Y; y += 1 {
		for x := list.X + 2; x < list.X+list.W-2; x += 1 {
			if got := pixelOf(tt, x, y); got != surface {
				t.Fatalf("band pixel %.0f,%.0f = %v, want the panel surface %v", x, y, got, surface)
			}
		}
	}
}

// TestPalettePlaceholderClearsTheCaret checks the gap between the caret and
// the placeholder, and that typed text starts where the caret is.
func testPalettePlaceholderClearsTheCaret(t *testing.T, locale string, dark bool) {
	tk := tokens(dark, locale)
	_, tt := polishPalette(t, locale, dark, nil)
	caret, active := tt.TextCaret()
	if !active {
		t.Fatal("the search field does not take the keyboard")
	}
	y := caret.Y + caret.H/2
	field := pixelOf(tt, caret.X+caret.W+120, y)
	inset := tk.SizePalettePlaceholderInset
	// Nothing shows between the caret and the placeholder's inset: the gap.
	if ink := inkStart(tt, caret.X+caret.W+0.5, caret.X+inset, y, field); ink >= 0 {
		t.Fatalf("ink at %.1f between the caret %.1f and the placeholder inset %.0f", ink, caret.X, inset)
	}
	placeholderInk := inkStart(tt, caret.X+inset, caret.X+inset+6, y, field)
	if placeholderInk < 0 {
		t.Fatalf("the placeholder does not start %.0f after the caret", inset)
	}
	// Typing starts at the caret, not at the placeholder's inset.
	tt.Type("常")
	tt.Frame()
	typedInk := inkStart(tt, caret.X-1, caret.X+caret.W+inset, y, field)
	if typedInk < 0 || typedInk > caret.X+caret.W+3 {
		t.Fatalf("typed text starts at %.1f, not at the caret %.1f", typedInk, caret.X)
	}
	if typedInk >= placeholderInk {
		t.Fatalf("typed text starts at %.1f, at the placeholder's %.1f", typedInk, placeholderInk)
	}
}

// Exercise geometry and pixels in both languages and themes, rather than
// assuming the light Chinese rendering represents all appearances.
func TestPalettePolish(t *testing.T) {
	checks := []struct {
		name string
		run  func(*testing.T, string, bool)
	}{
		{"HighlightIsSymmetric", testPaletteHighlightIsSymmetric},
		{"ScrollbarStaysClearOfRows", testPaletteScrollbarStaysClearOfRows},
		{"HintStaysInsideTheHighlight", testPaletteHintStaysInsideTheHighlight},
		{"JSONWindowTitleIsNotRepeated", testPaletteJSONWindowName},
		{"SearchSurfaceReadsAsAField", testPaletteSearchSurface},
		{"RowsNameTheirTile", testPaletteRowsNameTheirTile},
		{"LongTitleEllipsizes", testPaletteLongTitleEllipsizes},
		{"KeycapsAreSeparateKeys", testPaletteKeycapsAreSeparateKeys},
		{"SearchHasNoDivider", testPaletteSearchHasNoDivider},
		{"PlaceholderClearsTheCaret", testPalettePlaceholderClearsTheCaret},
		{"ThemeSurfaces", testPaletteThemeSurfaces},
		{"EmptyComposition", testPaletteEmptyComposition},
		{"CommandCategories", testPaletteCommandCategories},
		{"LongTitleWithHint", testPaletteLongTitleWithHint},
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			for _, check := range checks {
				t.Run(fmt.Sprintf("%s/dark=%v/%s", locale, dark, check.name), func(t *testing.T) {
					check.run(t, locale, dark)
				})
			}
		}
	}
}

func testPaletteLongTitleWithHint(t *testing.T, locale string, dark bool) {
	for _, invalid := range []bool{false, true} {
		a := paletteTestApp(t)
		tk := tokens(dark, locale)
		category := a.text(locale, "Awake")
		hint := fmt.Sprintf(a.text(locale, "PaletteMinutes"), 45)
		if invalid {
			hint = strings.Repeat("invalid-argument-", 80)
		}
		p := &paletteState{rows: []paletteRow{{Kind: paletteRowCommand, Key: "long", Title: strings.Repeat("long command ", 80), Category: category, Hint: hint}}}
		var categoryWidth, hintWidth float32
		tt := ui.NewTester(func(c *ui.Context) {
			applyTheme(c)
			categoryWidth, _ = c.MeasureText(0, ui.Span{Text: category, Size: tk.FontSizeSmall})
			hintWidth, _ = c.MeasureText(0, ui.Span{Text: hint, Size: tk.FontSizeSmall})
			ui.Column(c).Fill().Padding(tk.SizePaletteShadowMargin + tk.Space8).Children(func() {
				a.paletteRow(c, tk, locale, p, 0)
			})
		}, paletteWindowWidth(), paletteWindowHeight())
		tt.SetLocale(locale)
		tt.SetDark(dark)
		tt.Frame()
		cat, ok := tt.Find(category)
		if !ok || cat.W < categoryWidth-0.5 {
			t.Fatalf("category compressed with hint: %+v, want %.1f", cat, categoryWidth)
		}
		h, ok := tt.Find(hint)
		right := float32(paletteWindowWidth()) - tk.SizePaletteShadowMargin - 2*tk.Space8
		if !ok || h.X < cat.X+cat.W+tk.Space8-0.5 || h.X+h.W > right+0.5 {
			t.Fatalf("hint overlaps the category or leaves the row: category=%+v hint=%+v right=%.1f", cat, h, right)
		}
		if !invalid && h.W < hintWidth-0.5 {
			t.Fatalf("normal duration hint compressed: %.1f, want %.1f", h.W, hintWidth)
		}
	}
}

func testPaletteThemeSurfaces(t *testing.T, locale string, dark bool) {
	a, tt := polishPalette(t, locale, dark, func(a *application, tt *ui.Tester) {
		a.config.PaletteFavorites = []string{"json.new"}
		a.config.PaletteRecents = []string{"settings.page.shortcuts"}
	})
	tk := tokens(dark, locale)
	list, panel, search := a.palette.listBounds, a.palette.panelBounds, a.palette.searchBounds
	y := list.Y
	groups := 0
	for _, row := range a.palette.rows {
		if row.Kind == paletteRowHeader {
			groups++
			probeY := y + tk.SizePaletteGroupHeight/2
			base := pixelOf(tt, panel.X+tk.Space4, probeY)
			if got := pixelOf(tt, list.X+list.W-tk.Space8-1, probeY); got != base {
				t.Fatalf("%s paints a separate band: %v, panel %v", row.Group, got, base)
			}
			y += tk.SizePaletteGroupHeight
		} else {
			y += tk.SizePaletteRowHeight
		}
	}
	if groups != 3 {
		t.Fatalf("want all three group surfaces, got %d", groups)
	}
	if a.palette.list.Header != nil {
		t.Fatal("transparent labels must scroll with rows, not overlap them")
	}
	control := ui.Hex(tk.ColorPaletteSearch)
	want := color.RGBA{control.R, control.G, control.B, control.A}
	if got := pixelOf(tt, search.X+search.W/2, search.Y+tk.Space4); got != want {
		t.Fatalf("search background = %v, want ColorPaletteSearch %v", got, want)
	}
	if pixelOf(tt, panel.X+tk.Space4, search.Y+tk.Space4) == want {
		t.Fatal("search surface merges into panel")
	}
	tt.Type(a.text(locale, "R05Start"))
	tt.Key(ui.Cmd, ui.KeyK)
	if a.palette.mode != paletteActionMode || len(a.palette.rows) < 3 || a.palette.rows[1].Kind != paletteRowSeparator {
		t.Fatal("action title lost its intentional separator")
	}
	// The line is visible below the action title, not below the search field.
	y = a.palette.listBounds.Y + tk.SizePaletteRowHeight
	if pixelOf(tt, list.X+list.W/2, y) == pixelOf(tt, list.X+list.W/2, y-tk.Space4) {
		t.Fatal("action title separator is not painted")
	}
}

func testPaletteEmptyComposition(t *testing.T, locale string, dark bool) {
	a, tt := polishPalette(t, locale, dark, nil)
	// Texts includes the input's accessible name as well as the visible hint.
	countHint := func() int {
		count := 0
		for _, text := range tt.Texts() {
			if text == a.text(locale, "PaletteSearch") {
				count++
			}
		}
		return count
	}
	before := countHint()
	tt.Compose("pinyin", 6)
	if a.palette.query != "" || countHint() != before-1 {
		t.Fatal("empty query shows its placeholder over uncommitted composition")
	}
	if active, noIME := tt.TextInputActive(); !active || noIME {
		t.Fatal("composition disabled text input")
	}
	paletteReviewScreenshot(t, tt, "palette-composition", locale, dark)
	tt.Compose("", 0)
	if countHint() != before {
		t.Fatal("cancelling composition did not restore the placeholder")
	}
	tt.Compose("pinyin", 6)
	tt.Type("拼音")
	if a.palette.query != "拼音" || countHint() != before-1 {
		t.Fatal("composition commit lost text or restored the hint")
	}
}

func testPaletteCommandCategories(t *testing.T, locale string, dark bool) {
	a, tt := polishPalette(t, locale, dark, nil)
	a.addJSONDocument(`{}`, "document.json", true)
	if err := a.awake.start(awakeSession{Mode: "screen"}); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	seen := map[string]bool{}
	for _, row := range a.palette.rows {
		if row.Kind == paletteRowCommand {
			want := a.text(locale, commandCategory(row.Command))
			if row.Category != "" && row.Category != want {
				t.Fatalf("%s names %q, want %q", row.Key, row.Category, want)
			}
			if row.Category == "" && !strings.EqualFold(strings.TrimSpace(row.Title), strings.TrimSpace(want)) {
				t.Fatalf("%s dropped its category %q, which its title does not repeat", row.Key, want)
			}
			seen[row.Key] = true
		}
	}
	for _, cmd := range a.commands() {
		if !seen[cmd.ID] {
			t.Fatalf("command %s is absent from categorized rows", cmd.ID)
		}
	}
}

// testPaletteJSONWindowName checks an unnamed JSON window's row: its title is
// the tile's own name, so the category would only repeat it and is dropped.
func testPaletteJSONWindowName(t *testing.T, locale string, dark bool) {
	a, tt := polishPalette(t, locale, dark, nil)
	a.addJSONDocument(`{}`, "", true)
	a.addJSONDocument(`{}`, "items.json", true)
	tt.Frame()
	json := a.text(locale, "JSON")
	unnamed, named, created := false, false, false
	for _, row := range a.palette.rows {
		if row.Kind != paletteRowCommand {
			continue
		}
		switch {
		case row.Title == a.text(locale, "JSONWindowName"):
			unnamed = true
			if row.Category != "" {
				t.Fatalf("the unnamed window repeats %q after its title", row.Category)
			}
		case row.Title == "items.json":
			named = true
			if row.Category != json {
				t.Fatalf("the named window shows %q, want %q", row.Category, json)
			}
		case row.Key == "json.new":
			created = true
			if row.Category != json {
				t.Fatalf("the new-window command shows %q, want %q", row.Category, json)
			}
		}
	}
	if !unnamed || !named || !created {
		t.Fatalf("the JSON rows are missing: unnamed=%v named=%v new=%v, %v", unnamed, named, created, tt.Texts())
	}
}

// testPaletteSearchSurface checks the field against the panel in both themes:
// the dark fill separates at least as much as the light one, and the
// placeholder keeps WCAG AA on either.
func testPaletteSearchSurface(t *testing.T, locale string, dark bool) {
	separations := map[bool]float64{}
	for _, mode := range []bool{false, true} {
		tk := tokens(mode, locale)
		a, tt := polishPalette(t, locale, mode, nil)
		panel, search := a.palette.panelBounds, a.palette.searchBounds
		field := pixelOf(tt, search.X+search.W/2, search.Y+tk.Space4)
		surface := pixelOf(tt, panel.X+tk.Space4, search.Y+tk.Space4)
		separations[mode] = contrastRatio(field, surface)
		secondary := ui.Hex(tk.ColorTextSecondary)
		placeholder := color.RGBA{secondary.R, secondary.G, secondary.B, 0xFF}
		if ratio := contrastRatio(placeholder, field); ratio < 4.5 {
			t.Fatalf("dark=%v: the placeholder on the field is %.2f:1, want 4.5:1 at least", mode, ratio)
		}
	}
	if separations[true] < separations[false] {
		t.Fatalf("the field stands out by %.2f:1 in dark against %.2f:1 in light", separations[true], separations[false])
	}
	if separations[true] < 1.45 {
		t.Fatalf("the field stands out by %.2f:1 from the dark panel, want 1.45:1 at least", separations[true])
	}
}

func TestShortcutTooltipKeycaps(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			for _, key := range []string{"JSONShortcutLabel", "PaletteShortcutLabel"} {
				t.Run(fmt.Sprintf("%s/dark=%v/%s", locale, dark, key), func(t *testing.T) {
					a := panelTestApp(t)
					a.config.JSONShortcut, a.config.PaletteShortcut = "Ctrl+Alt+Shift+Cmd+F12", "Ctrl+Alt+Shift+Cmd+F12"
					tt := panelView(a, locale)
					tt.SetDark(dark)
					label := a.text(locale, key)
					anchor := label
					if key == "PaletteShortcutLabel" {
						anchor = a.menubarPanelTooltip(locale, key, a.config.PaletteShortcut)
					}
					r, ok := tt.Find(anchor)
					if !ok {
						t.Fatal("shortcut button missing")
					}
					tt.Move(r.X+r.W/2, r.Y+r.H/2)
					tt.Advance(time.Second)
					if !tt.HasText(label) {
						t.Fatal("tooltip label missing")
					}
					var previous ui.Rect
					for _, symbol := range shortcutSymbols(a.config.PaletteShortcut) {
						cap, ok := tt.Find(symbol)
						if !ok || cap.X < 0 || cap.X+cap.W > 320 || cap.Y < 0 || cap.Y+cap.H > float32(a.menubarPanelHeight(tokens(dark, locale))) {
							t.Fatalf("keycap %s missing or clipped: %+v", symbol, cap)
						}
						if previous.W > 0 && cap.X-previous.X-previous.W < tokens(dark, locale).Space4 {
							t.Fatal("tooltip keys overlap")
						}
						previous = cap
					}
					paletteReviewScreenshot(t, tt, "tooltip-"+key, locale, dark)
				})
			}
		}
	}
}

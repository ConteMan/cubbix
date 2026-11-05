package main

import (
	"image/color"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// polishPalette frames the palette once, runs setup, and frames again, so the
// checks read the geometry of a settled frame.
func polishPalette(t *testing.T, setup func(*application, *ui.Tester)) (*application, *ui.Tester) {
	t.Helper()
	a := paletteTestApp(t)
	tt := ui.NewTester(a.paletteView, paletteWindowWidth(), paletteWindowHeight())
	tt.SetScale(2)
	tt.SetLocale("zh-CN")
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

// TestPaletteListKeepsAScrollbarSlot checks that the scroll bar shows in its
// own slot and that neither a row nor the highlight of the selected row
// reaches under it.
func TestPaletteListKeepsAScrollbarSlot(t *testing.T) {
	tk := tokens(false, "zh-CN")
	var a *application
	a, tt := polishPalette(t, func(a *application, tt *ui.Tester) {
		// Point at a row below the selected one: the list shows its bar and the
		// selected row keeps its highlight.
		list := a.palette.listBounds
		tt.Move(list.X+list.W/2, list.Y+24+36*2+18)
	})
	list := a.palette.listBounds
	rowsRight := list.X + list.W - tk.SizePaletteScrollbarSlot
	barLeft := list.X + list.W - tk.SizeScrollbar - 3
	if barLeft < rowsRight {
		t.Fatalf("the slot (%.0f) is narrower than the bar at %.0f", rowsRight, barLeft)
	}
	row, ok := tt.Find(a.text("zh-CN", "R05Start"))
	if !ok {
		t.Fatalf("the selected row is missing: %v", tt.Texts())
	}
	y := row.Y + row.H/2
	highlight := pixelOf(tt, rowsRight-60, y)
	if highlight == pixelOf(tt, rowsRight+1, y) {
		t.Fatalf("the selected row's highlight already shows past the rows' edge at %.0f", y)
	}
	for _, probe := range []float32{rowsRight + 0.5, rowsRight + 2.5, barLeft - 0.5} {
		if pixelOf(tt, probe, y) == highlight {
			t.Fatalf("the selected row's highlight shows at %.0f,%.0f, under the scroll bar", probe, y)
		}
	}
	// The bar itself is there, a colour of its own between the rows and the
	// list's edge.
	if pixelOf(tt, barLeft+3, y) == pixelOf(tt, barLeft-1, y) {
		t.Fatalf("no scroll bar in the slot at %.0f", y)
	}
}

// TestPaletteHintStaysClearOfTheSlot checks that a row's hint ends before the
// scroll bar's slot.
func TestPaletteHintStaysClearOfTheSlot(t *testing.T) {
	tk := tokens(false, "zh-CN")
	a, tt := polishPalette(t, func(a *application, tt *ui.Tester) {
		tt.Type(a.text("zh-CN", "R05Start") + " 45m")
	})
	hint, ok := tt.Find("45 分钟")
	if !ok {
		t.Fatalf("the selected row shows no hint: %v", tt.Texts())
	}
	list := a.palette.listBounds
	if right := list.X + list.W - tk.SizePaletteScrollbarSlot; hint.X+hint.W > right+0.5 {
		t.Fatalf("the hint reaches %.1f, past the rows' edge %.1f", hint.X+hint.W, right)
	}
}

// TestPaletteRowsNameTheirTile checks the secondary text after each title: the
// tile a command belongs to, one gap after the title's ink.
func TestPaletteRowsNameTheirTile(t *testing.T) {
	tk := tokens(false, "zh-CN")
	a, tt := polishPalette(t, nil)
	list := a.palette.listBounds
	row, ok := tt.Find(a.text("zh-CN", "R05Start"))
	if !ok {
		t.Fatalf("the selected row is missing: %v", tt.Texts())
	}
	category := a.text("zh-CN", "Awake")
	box, ok := tt.Find(category)
	if !ok {
		t.Fatalf("%q shows no category %q: %v", a.text("zh-CN", "R05Start"), category, tt.Texts())
	}
	if box.Y < row.Y || box.Y+box.H > row.Y+row.H {
		t.Fatalf("the category at %.1f is not on the selected row %.1f", box.Y, row.Y)
	}
	y := box.Y + box.H/2
	highlight := pixelOf(tt, list.X+list.W-tk.SizePaletteScrollbarSlot-60, row.Y+row.H/2)
	last, found := inkBefore(tt, list.X+tk.Space8+tk.SizeIcon+tk.Space8, box.X-0.5, y, highlight)
	if !found {
		t.Fatal("the row shows no title ink")
	}
	if gap := box.X - last; gap < tk.Space8-1 {
		t.Fatalf("the title ends %.1f before its category, want %.0f", gap, tk.Space8)
	}
	// The category keeps its words and stays inside the rows.
	if box.X+box.W > list.X+list.W-tk.SizePaletteScrollbarSlot+0.5 {
		t.Fatalf("the category overflows the rows: %.1f", box.X+box.W)
	}
	// General commands belong to the app, not to a tile.
	if !tt.HasText(a.text("zh-CN", "R05App")) || !tt.HasText(a.text("zh-CN", "JSON")) {
		t.Fatalf("command categories are missing: %v", tt.Texts())
	}
}

// TestPaletteLongTitleEllipsizes checks that a long title gives way, with an
// ellipsis, before the category and the hint, and stays inside the rows.
func TestPaletteLongTitleEllipsizes(t *testing.T) {
	tk := tokens(false, "zh-CN")
	a := paletteTestApp(t)
	long := strings.Repeat("very-long-json-document-name-", 8) + ".json"
	a.addJSONDocument(`{}`, long, true)
	a.palette.query, a.palette.lastQuery = long, ""
	tt := ui.NewTester(a.paletteView, paletteWindowWidth(), paletteWindowHeight())
	tt.SetScale(2)
	tt.SetLocale("zh-CN")
	tt.Frame()
	row, ok := tt.Find(long)
	if !ok {
		t.Fatalf("the long title is not listed: %v", tt.Texts())
	}
	list := a.palette.listBounds
	right := list.X + list.W - tk.SizePaletteScrollbarSlot
	category, ok := tt.Find(a.text("zh-CN", "JSON"))
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
}

// TestPaletteKeycapsAreSeparateKeys checks that ⌘ and K are drawn in keycaps of
// their own with a gap between them, not as one run of text.
func TestPaletteKeycapsAreSeparateKeys(t *testing.T) {
	tk := tokens(false, "zh-CN")
	a, tt := polishPalette(t, nil)
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
func TestPaletteSearchHasNoDivider(t *testing.T) {
	a, tt := polishPalette(t, nil)
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
func TestPalettePlaceholderClearsTheCaret(t *testing.T) {
	tk := tokens(false, "zh-CN")
	_, tt := polishPalette(t, nil)
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

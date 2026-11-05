package main

import "github.com/egoist/mygo/ui"

// paletteActionKeys is what the palette shows for its action list: one keycap
// per key, as Raycast draws them.
var paletteActionKeys = []string{"⌘", "K"}

// keycaps draws each key of a shortcut as a keycap of its own: one rounded box
// per key, a gap between them, so a modifier and a letter are never squeezed
// together. Pass shortcutSymbols for a recorded shortcut.
func keycaps(c *ui.Context, t tokenValues, keys []string) {
	if len(keys) == 0 {
		return
	}
	ui.Row(c).Gap(t.Space4).AlignItems(ui.Center).Children(func() {
		for _, key := range keys {
			ui.Row(c).Height(t.SizeKeycapHeight).MinWidth(t.SizeKeycapMinWidth).PaddingX(t.Space6).Radius(t.RadiusControl).
				Background(ui.Hex(t.ColorKeycap)).Border(t.StrokeHairline, ui.Hex(t.ColorKeycapStroke)).
				AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, key).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorKeycapText))
			})
		}
	})
}

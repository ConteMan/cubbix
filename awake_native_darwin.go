//go:build darwin

package main

import (
	"bytes"
	"image"
	"unsafe"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo/ui"
	_ "golang.org/x/image/tiff"
)

// Read the icon of a running graphical application; never launch or activate it.
func runningAppIcon(pid int) *ui.Bitmap {
	app := objc.ID(objc.GetClass("NSRunningApplication")).Send(objc.RegisterName("runningApplicationWithProcessIdentifier:"), pid)
	if app == 0 {
		return nil
	}
	icon := app.Send(objc.RegisterName("icon"))
	if icon == 0 {
		return nil
	}
	data := icon.Send(objc.RegisterName("TIFFRepresentation"))
	if data == 0 {
		return nil
	}
	size := objc.Send[uint64](data, objc.RegisterName("length"))
	ptr := objc.Send[unsafe.Pointer](data, objc.RegisterName("bytes"))
	if size == 0 || size > 16<<20 || ptr == nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(unsafe.Slice((*byte)(ptr), int(size))))
	if err != nil {
		return nil
	}
	return ui.NewBitmap(img)
}

// Package qr is the app's one QR Code encoder. It pins the parameters of spec
// 011 §2.4 so the window, the command line and the tests always produce the
// same symbol for the same text.
//
// The library is imported under an alias: this package is called qr too, and a
// reader must be able to tell the wrapper from github.com/piglig/go-qr/v2.
package qr

import (
	"errors"
	"fmt"
	"image"

	codeqr "github.com/piglig/go-qr/v2"
)

const (
	// QuietZone is the margin around the symbol in modules. The standard
	// requires at least 4; a screen symbol keeps it so a phone camera can
	// find the symbol on a busy background.
	QuietZone = 4
	// ExportModule is the side of one module in the exported PNG.
	ExportModule = 16
	// MaxBytes is the byte-mode capacity at ECC M without a boost. It is
	// what the interface may state as the limit of one symbol; a numeric or
	// upper-case payload can hold more, and a longer one is never truncated.
	MaxBytes = 2331
	// MaxChinese is how many three-byte characters fit in MaxBytes: 776
	// characters are 2328 bytes, and 777 are 2331, which is one byte over.
	MaxChinese = (MaxBytes - 3) / 3
)

// ErrTooLong reports text that does not fit in one symbol. The library's own
// ErrDataTooLong is wrapped, so errors.Is sees both.
var ErrTooLong = errors.New("qr: text does not fit in one symbol")

// Code is one encoded symbol.
type Code struct {
	code *codeqr.Code
	// Bytes is the length of the encoded text in bytes.
	Bytes int
}

// Encode builds the symbol with the fixed parameters: ECC M, no boost, a
// single numeric, alphanumeric or byte segment and UTF-8 ECI for non-ASCII text.
// A payload that does not fit is reported as ErrTooLong and never truncated.
func Encode(text string) (*Code, error) {
	code, err := codeqr.Encode(text,
		codeqr.WithECC(codeqr.ECCMedium),
		codeqr.WithoutECCBoost(),
		codeqr.WithSimpleSegmentation(),
		codeqr.WithUTF8ECI())
	if err != nil {
		if errors.Is(err, codeqr.ErrDataTooLong) {
			return nil, fmt.Errorf("%w: %w", ErrTooLong, err)
		}
		return nil, err
	}
	return &Code{code: code, Bytes: len(text)}, nil
}

// Version is the symbol version, from 1 to 40.
func (c *Code) Version() int { return c.code.Version() }

// ECC is the error correction level as the interface names it: L, M, Q or H.
func (c *Code) ECC() string { return c.code.ECC().String() }

// Modules is the side of the symbol in modules, quiet zone excluded.
func (c *Code) Modules() int { return c.code.Size() }

// Matrix renders the symbol as Unicode half blocks: two module rows per line,
// █ ▀ ▄ for dark modules and spaces for light ones. quiet is the quiet zone in
// modules; the caller picks it, because a terminal has no room to spare.
func (c *Code) Matrix(quiet int) string {
	if quiet < 0 {
		quiet = 0
	}
	var out []byte
	lo, hi := -quiet, c.Modules()+quiet
	for y := lo; y < hi; y += 2 {
		for x := lo; x < hi; x++ {
			top, bottom := c.code.Module(x, y), c.code.Module(x, y+1)
			switch {
			case top && bottom:
				out = append(out, []byte("█")...)
			case top:
				out = append(out, []byte("▀")...)
			case bottom:
				out = append(out, []byte("▄")...)
			default:
				out = append(out, ' ')
			}
		}
		out = append(out, '\n')
	}
	return string(out)
}

// Image renders the symbol with a quiet zone of QuietZone modules and the
// given side in pixels per module. The result is an opaque black-on-white
// image: the two colours are the symbol's own and never follow the theme.
func (c *Code) Image(modulePixels int) (image.Image, error) {
	if modulePixels < 1 {
		return nil, fmt.Errorf("qr: module pixels %d must be at least 1", modulePixels)
	}
	img, err := c.code.Image(codeqr.WithScale(modulePixels), codeqr.WithQuietZone(QuietZone))
	if err != nil {
		return nil, err
	}
	return img, nil
}

// PNG renders the symbol at ExportModule pixels per module with a quiet zone
// of QuietZone modules.
func (c *Code) PNG(modulePixels int) ([]byte, error) {
	if modulePixels < 1 {
		return nil, fmt.Errorf("qr: module pixels %d must be at least 1", modulePixels)
	}
	return c.code.PNG(codeqr.WithScale(modulePixels), codeqr.WithQuietZone(QuietZone))
}

// SidePixels is the side of an exported image in pixels, quiet zone included.
func (c *Code) SidePixels(modulePixels int) int {
	return (c.Modules() + 2*QuietZone) * modulePixels
}

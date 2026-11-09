//go:build darwin

package main

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Dictionary Services is a plain C API inside CoreServices. Both entry points
// take CFStringRef values built here with CFStringCreateWithCString; no
// exported constant CFString is looked up with dlsym.
const (
	cfStringEncodingUTF8      = 0x08000100
	coreFoundationLibrary     = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"
	dictionaryServicesLibrary = "/System/Library/Frameworks/CoreServices.framework/Frameworks/DictionaryServices.framework/DictionaryServices"
)

var errDictionaryUnavailable = errors.New("dictionary services unavailable")

// cfRange is the CFRange struct returned by DCSGetTermRangeInString. Its
// location and length count UTF-16 code units.
type cfRange struct {
	Location int64
	Length   int64
}

type dictionarySymbols struct {
	createString  func(uintptr, string, uint32) uintptr
	stringLength  func(uintptr) int64
	stringMaxSize func(int64, uint32) int64
	stringBytes   func(uintptr, unsafe.Pointer, int64, uint32) uint8
	release       func(uintptr)
	termRange     func(uintptr, uintptr, int64) cfRange
	textDef       func(uintptr, uintptr, cfRange) uintptr
}

var (
	dictionaryOnce    sync.Once
	dictionaryHandles dictionarySymbols
	dictionaryLoadErr error
)

func loadDictionarySymbols() {
	cf, err := purego.Dlopen(coreFoundationLibrary, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		dictionaryLoadErr = errDictionaryUnavailable
		return
	}
	defer func() {
		if dictionaryLoadErr != nil {
			purego.Dlclose(cf)
		}
	}()
	ds, err := purego.Dlopen(dictionaryServicesLibrary, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		dictionaryLoadErr = errDictionaryUnavailable
		return
	}
	defer func() {
		if dictionaryLoadErr != nil {
			purego.Dlclose(ds)
		}
	}()
	for _, symbol := range []struct {
		handle uintptr
		name   string
	}{
		{cf, "CFStringCreateWithCString"}, {cf, "CFStringGetLength"},
		{cf, "CFStringGetMaximumSizeForEncoding"}, {cf, "CFStringGetCString"},
		{cf, "CFRelease"}, {ds, "DCSGetTermRangeInString"}, {ds, "DCSCopyTextDefinition"},
	} {
		if _, err := purego.Dlsym(symbol.handle, symbol.name); err != nil {
			dictionaryLoadErr = errDictionaryUnavailable
			return
		}
	}
	purego.RegisterLibFunc(&dictionaryHandles.createString, cf, "CFStringCreateWithCString")
	purego.RegisterLibFunc(&dictionaryHandles.stringLength, cf, "CFStringGetLength")
	purego.RegisterLibFunc(&dictionaryHandles.stringMaxSize, cf, "CFStringGetMaximumSizeForEncoding")
	purego.RegisterLibFunc(&dictionaryHandles.stringBytes, cf, "CFStringGetCString")
	purego.RegisterLibFunc(&dictionaryHandles.release, cf, "CFRelease")
	purego.RegisterLibFunc(&dictionaryHandles.termRange, ds, "DCSGetTermRangeInString")
	purego.RegisterLibFunc(&dictionaryHandles.textDef, ds, "DCSCopyTextDefinition")
}

// goString copies a CFStringRef into Go memory with UTF-8 encoding.
// The buffer is sized by CoreFoundation and bounded so a bad length cannot
// allocate an unbounded slice.
func (symbols dictionarySymbols) goString(ref uintptr) string {
	if ref == 0 {
		return ""
	}
	size := symbols.stringMaxSize(symbols.stringLength(ref), cfStringEncodingUTF8) + 1
	if size <= 0 || size > maxDictionarySystemBytes {
		return ""
	}
	buffer := make([]byte, size)
	if symbols.stringBytes(ref, unsafe.Pointer(&buffer[0]), size, cfStringEncodingUTF8) == 0 {
		return ""
	}
	end := bytes.IndexByte(buffer, 0)
	if end < 0 {
		end = len(buffer)
	}
	return string(buffer[:end])
}

// systemDictionaryLookup reads the local macOS dictionaries. A phrase is
// rejected defensively: DCSGetTermRangeInString may report only one word of
// the phrase, so a range that does not cover the whole term is treated as a
// miss. DCSCopyTextDefinition returning nil also means no dictionary block.
func systemDictionaryLookup(term string) string {
	if !isSingleEnglishWord(term) {
		return ""
	}
	dictionaryOnce.Do(loadDictionarySymbols)
	if dictionaryLoadErr != nil {
		return ""
	}
	return dictionaryHandles.definition(term)
}

func (symbols dictionarySymbols) definition(term string) string {
	text := strings.TrimSpace(term)
	cfText := symbols.createString(0, text, cfStringEncodingUTF8)
	if cfText == 0 {
		return ""
	}
	defer symbols.release(cfText)
	found := symbols.termRange(0, cfText, 0)
	units := int64(len(utf16.Encode([]rune(text))))
	if found.Length <= 0 || found.Location != 0 || found.Length != units {
		return ""
	}
	definition := symbols.textDef(0, cfText, found)
	if definition == 0 {
		return ""
	}
	defer symbols.release(definition)
	return symbols.goString(definition)
}

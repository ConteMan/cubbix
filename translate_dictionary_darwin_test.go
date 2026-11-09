//go:build darwin

package main

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
	"unsafe"
)

// TestDictionarySystemLookup exercises the read-only system API. It asserts
// only that definitions exist and have displayable structure: no test depends
// on the exact wording of an installed dictionary.
func TestDictionarySystemLookup(t *testing.T) {
	for _, term := range []string{"hello world", "take into account", "out of the box"} {
		if got := systemDictionaryLookup(term); got != "" {
			t.Fatalf("%q produced a definition", term)
		}
	}
	for _, term := range []string{"hello", "Hello", "serendipity"} {
		t.Run(term, func(t *testing.T) {
			definition := systemDictionaryLookup(term)
			if strings.TrimSpace(definition) == "" {
				t.Skip("no local dictionary entry available on this system")
			}
			if len(dictionarySegments(definition)) == 0 {
				t.Fatal("definition has no displayable structure")
			}
			for _, limit := range []int{0, dictionaryCacheLimit} {
				cache := newDictionaryCache(systemDictionaryLookup, limit)
				for range 2 {
					if got := cache.definition(term); strings.TrimSpace(got) == "" {
						t.Fatal("repeated lookup lost an available entry")
					}
				}
			}
		})
	}
}

func TestDictionaryCFOwnershipAndRange(t *testing.T) {
	units := int64(len(utf16.Encode([]rune("it’s"))))
	for _, test := range []struct {
		name       string
		created    uintptr
		found      cfRange
		definition uintptr
		maxSize    int64
		converted  uint8
		want       string
		released   []uintptr
	}{
		{"success", 1, cfRange{0, 4}, 2, 32, 1, "a definition", []uintptr{2, 1}},
		{"create failed", 0, cfRange{}, 0, 32, 1, "", nil},
		{"not found", 1, cfRange{-1, 0}, 0, 32, 1, "", []uintptr{1}},
		{"partial range", 1, cfRange{0, 2}, 0, 32, 1, "", []uintptr{1}},
		{"offset range", 1, cfRange{1, 4}, 0, 32, 1, "", []uintptr{1}},
		{"oversized range", 1, cfRange{0, 5}, 0, 32, 1, "", []uintptr{1}},
		{"nil definition", 1, cfRange{0, 4}, 0, 32, 1, "", []uintptr{1}},
		{"invalid size", 1, cfRange{0, 4}, 2, -1, 1, "", []uintptr{2, 1}},
		{"oversized string", 1, cfRange{0, 4}, 2, maxDictionarySystemBytes, 1, "", []uintptr{2, 1}},
		{"conversion failed", 1, cfRange{0, 4}, 2, 32, 0, "", []uintptr{2, 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var released []uintptr
			textDefCalls := 0
			symbols := dictionarySymbols{
				createString: func(allocator uintptr, text string, encoding uint32) uintptr {
					if allocator != 0 || text != "it’s" || encoding != cfStringEncodingUTF8 {
						t.Fatal("incorrect CFString creation arguments")
					}
					return test.created
				},
				termRange: func(dictionary, text uintptr, offset int64) cfRange {
					if dictionary != 0 || text != 1 || offset != 0 {
						t.Fatal("incorrect term range arguments")
					}
					return test.found
				},
				textDef: func(dictionary, text uintptr, found cfRange) uintptr {
					textDefCalls++
					if dictionary != 0 || text != 1 || found != (cfRange{0, units}) {
						t.Fatal("incorrect definition arguments")
					}
					return test.definition
				},
				stringLength: func(ref uintptr) int64 {
					if ref != 2 {
						t.Fatal("incorrect definition reference")
					}
					return 12
				},
				stringMaxSize: func(int64, uint32) int64 { return test.maxSize },
				stringBytes: func(ref uintptr, buffer unsafe.Pointer, size int64, encoding uint32) uint8 {
					if ref != 2 || size != test.maxSize+1 || encoding != cfStringEncodingUTF8 {
						t.Fatal("incorrect conversion arguments")
					}
					copy(unsafe.Slice((*byte)(buffer), int(size)), "a definition\x00")
					return test.converted
				},
				release: func(ref uintptr) { released = append(released, ref) },
			}
			if got := symbols.definition("  it’s  "); got != test.want {
				t.Fatalf("definition = %q; want %q", got, test.want)
			}
			if !reflect.DeepEqual(released, test.released) {
				t.Fatalf("released = %v; want %v", released, test.released)
			}
			wantCalls := 0
			if test.created != 0 && test.found == (cfRange{0, 4}) {
				wantCalls = 1
			}
			if textDefCalls != wantCalls {
				t.Fatalf("definition calls = %d; want %d", textDefCalls, wantCalls)
			}
		})
	}
}

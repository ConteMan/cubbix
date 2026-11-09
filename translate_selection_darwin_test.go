//go:build darwin

package main

import (
	"context"
	"errors"
	"testing"
	"unsafe"
)

// The AXError table is the darwin reader's job; unit tests classify the
// constants without calling AX. A bad attribute name must stay a plain
// "no selection" fallback, and a disabled API is the only failure that also
// offers the accessibility hint.
func TestTranslateAXErrorClassification(t *testing.T) {
	for _, pair := range []struct {
		code int32
		want error
	}{
		{axErrorSuccess, nil},
		{axErrorNoValue, errSelectionNoValue},
		{axErrorAPIDisabled, errSelectionDenied},
		{axErrorIllegalArgument, errSelectionIllegalArgument},
		{axErrorCannotComplete, errSelectionUnavailable},
		{-1, nil},
	} {
		err := axSelectionError(pair.code)
		if pair.want == nil {
			if pair.code == axErrorSuccess && err != nil {
				t.Fatalf("success returned %v", err)
			}
			if pair.code != axErrorSuccess && err == nil {
				t.Fatalf("unexpected code %d was accepted", pair.code)
			}
			continue
		}
		if !errors.Is(err, pair.want) {
			t.Fatalf("code %d = %v; want %v", pair.code, err, pair.want)
		}
	}
}

func TestTranslateAXTimeoutAndOwnership(t *testing.T) {
	for _, scenario := range []string{"success", "focused-error", "value-error", "no-value", "wrong-focused-type", "wrong-value-type", "timeout-error", "expired"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), selectionTimeout)
			defer cancel()
			allocated := map[uintptr]int{}
			allocate := func(ref uintptr) uintptr { allocated[ref]++; return ref }
			release := func(ref uintptr) { allocated[ref]-- }
			var timeouts []uintptr
			copies := 0
			symbols := axSymbols{
				createApp: func(int32) uintptr { return allocate(1) },
				createString: func(_ uintptr, text string, _ uint32) uintptr {
					if text == axAttributeFocusedUIElement {
						return allocate(2)
					}
					return allocate(4)
				},
				release: release,
				setTimeout: func(ref uintptr, seconds float32) int32 {
					if seconds <= 0 || seconds > float32(selectionTimeout.Seconds()) {
						t.Fatalf("timeout = %v", seconds)
					}
					timeouts = append(timeouts, ref)
					if scenario == "timeout-error" {
						return axErrorIllegalArgument
					}
					return axErrorSuccess
				},
				copyAttribute: func(ref, _ uintptr, out unsafe.Pointer) int32 {
					copies++
					if ref == 1 {
						*(*uintptr)(out) = allocate(3)
						if scenario == "focused-error" {
							return axErrorCannotComplete
						}
						if scenario == "expired" {
							cancel()
						}
					} else {
						if scenario == "no-value" {
							return axErrorNoValue
						}
						*(*uintptr)(out) = allocate(5)
						if scenario == "value-error" {
							return axErrorAPIDisabled
						}
					}
					return axErrorSuccess
				},
				getTypeID: func(ref uintptr) uint64 {
					if ref == 3 && scenario != "wrong-focused-type" {
						return 10
					}
					if ref == 5 && scenario != "wrong-value-type" {
						return 11
					}
					return 0
				},
				axTypeID: func() uint64 { return 10 }, cfStringTypeID: func() uint64 { return 11 },
				stringLength:  func(uintptr) int64 { return 4 },
				stringMaxSize: func(int64, uint32) int64 { return 4 },
				stringBytes: func(_ uintptr, buffer unsafe.Pointer, size int64, _ uint32) uint8 {
					copy(unsafe.Slice((*byte)(buffer), int(size)), "text\x00")
					return 1
				},
			}
			text, err := symbols.selectedText(ctx, 123)
			if scenario == "success" {
				if err != nil || text != "text" || len(timeouts) != 2 || timeouts[0] != 1 || timeouts[1] != 3 {
					t.Fatalf("read = %q %v timeouts=%v", text, err, timeouts)
				}
			} else if err == nil || text != "" {
				t.Fatalf("failure accepted: %q %v", text, err)
			}
			if scenario == "expired" && copies != 1 {
				t.Fatal("expired read continued messaging")
			}
			for ref, count := range allocated {
				if count != 0 {
					t.Fatalf("reference %d retained %d times", ref, count)
				}
			}
		})
	}
}

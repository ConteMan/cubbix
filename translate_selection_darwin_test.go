//go:build darwin

package main

import (
	"errors"
	"testing"
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

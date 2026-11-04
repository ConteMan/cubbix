package main

import (
	"github.com/ConteMan/cubbix/internal/lid"
	"os"
	"testing"
)

func TestLidNativeReadOnly(t *testing.T) {
	if os.Getenv("CUBBIX_LID_NATIVE_PROBE") == "" {
		t.Skip("read-only native probe not requested")
	}
	_ = lidOSSupported()
	if !(lid.SystemPower{}).HasInternalBattery() {
		t.Fatal("internal battery not detected")
	}
	kind, p, err := (lid.SystemPower{}).Source()
	if err != nil {
		t.Fatal(err)
	}
	if (kind != "ac" && kind != "battery") || p < 0 || p > 100 {
		t.Fatal("invalid native power reading")
	}
}

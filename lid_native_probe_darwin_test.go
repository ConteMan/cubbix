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
	sample, err := (lid.SystemPower{}).Sample()
	if err != nil {
		t.Fatal(err)
	}
	if (sample.Kind != "ac" && sample.Kind != "battery") || sample.Percent < 0 || sample.Percent > 100 {
		t.Fatal("invalid native power reading")
	}
	_ = (lid.SystemPower{}).Adapter()
}

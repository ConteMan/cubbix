//go:build darwin

package main

import (
	"path/filepath"
	"testing"

	"github.com/ebitengine/purego"
)

func preparePickerNativeEvidence(t *testing.T) {
	t.Helper()
	// ui.Tester intentionally does not initialize AppKit. Load the framework
	// for metadata/icon reads, as the running application already has done.
	// This does not create NSApplication or any windows.
	if _, err := purego.Dlopen(filepath.Join("/System", "Library", "Frameworks", "AppKit.framework", "AppKit"), purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
		t.Fatal(err)
	}
}

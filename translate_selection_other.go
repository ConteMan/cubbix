//go:build !darwin

package main

// Accessibility is a macOS capability. Other platforms never read a
// selection, so both the settings row and the shortcut fall back to typing.
type systemSelectionReader struct{}

func (systemSelectionReader) Trusted() bool { return false }

func (systemSelectionReader) FrontmostPID() int32 { return 0 }

func (systemSelectionReader) SelectedText(int32) (string, error) {
	return "", errSelectionUnavailable
}

func accessibilityTrusted() bool { return false }

func copySelectedText() (string, bool) { return "", false }

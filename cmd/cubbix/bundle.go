package main

import (
	"os"
	"path/filepath"
	"regexp"
)

// versionPattern finds CFBundleShortVersionString in an XML Info.plist.
var versionPattern = regexp.MustCompile(`(?s)<key>CFBundleShortVersionString</key>\s*<string>([^<]*)</string>`)

// plistVersion reads the version of the app bundle, or "unknown".
func plistVersion(bundle string) string {
	data, err := os.ReadFile(filepath.Join(bundle, "Contents", "Info.plist"))
	if err != nil {
		return "unknown"
	}
	match := versionPattern.FindSubmatch(data)
	if match == nil || len(match[1]) == 0 {
		return "unknown"
	}
	return string(match[1])
}

// evalSymlinks is filepath.EvalSymlinks, as a variable so tests can stub it.
var evalSymlinks = filepath.EvalSymlinks

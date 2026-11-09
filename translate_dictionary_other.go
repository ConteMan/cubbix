//go:build !darwin

package main

// macOS Dictionary Services is the only dictionary source. Other platforms
// report no definition, so the floating panel simply omits the block.
func systemDictionaryLookup(string) string { return "" }

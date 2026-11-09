//go:build !darwin

package main

// newSystemSpeech keeps the same fallback shape on other platforms, but both
// backends are unavailable: Cubbix is macOS-only and never starts a voice or
// a child process outside the running application.
func newSystemSpeech() speechEngine {
	return &fallbackSpeech{primary: unavailableSpeech{}, fallback: unavailableSpeech{}}
}

package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxTranslateInput = 4000

// isSingleEnglishWord is the pure word test. The macOS dictionary lookup and
// its collapsible block belong to a later step; the prompt shape uses this
// predicate already so single words get a shorter instruction.
func isSingleEnglishWord(s string) bool {
	term := strings.TrimSpace(s)
	if term == "" {
		return false
	}
	runes := []rune(term)
	if len(runes) > 48 || !unicode.IsLetter(runes[0]) || runes[0] > unicode.MaxASCII {
		return false
	}
	for _, r := range runes {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r == '-' || r == '\'' || r == '\u2019':
		default:
			return false
		}
	}
	return true
}

// detectDirection decides the target language. Kana and Hangul always go to
// Simplified Chinese; any Han character means Chinese to English; everything
// else goes to Simplified Chinese. Pure-Han Japanese therefore reads as
// Chinese, a known deviation recorded in the design.
func detectDirection(s string) (source, target string) {
	for _, r := range s {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			return "auto", "zh-Hans"
		}
	}
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return "zh-Hans", "en"
		}
	}
	return "auto", "zh-Hans"
}

// clipTranslateInput caps the prompt at 4000 runes and reports how many were
// dropped. The remainder is still translated: too long is an overlay, not a
// separate phase.
func clipTranslateInput(s string) (clipped string, dropped int) {
	runes := []rune(s)
	if len(runes) <= maxTranslateInput {
		return s, 0
	}
	return string(runes[:maxTranslateInput]), len(runes) - maxTranslateInput
}

func splitTranslateLines(s string) []string { return strings.Split(s, "\n") }

// translateInputLines estimates the input rows for the height formula. The
// editor scrolls past four rows; wrapping is approximated from the panel width
// so a long single line cannot grow the window past its maximum.
func translateInputLines(s string) int {
	const columns = 56
	lines := 0
	for _, line := range splitTranslateLines(s) {
		lines++
		runes := utf8.RuneCountInString(line)
		if runes > columns {
			lines += (runes - 1) / columns
		}
	}
	if lines < 1 {
		lines = 1
	}
	if lines > 4 {
		lines = 4
	}
	return lines
}

package main

import (
	"strings"
	"testing"
)

func TestIsSingleEnglishWord(t *testing.T) {
	// Case, internal hyphens and both apostrophe forms are still one word;
	// leading/trailing blanks are trimmed before the test.
	yes := []string{"hello", "Hello", "HELLO", "iPhone", "A", "don't", "it’s",
		"well-known", "a-b-c", "O'Reilly", strings.Repeat("a", 48),
		"  serendipity  ", "take-"}
	for _, term := range yes {
		if !isSingleEnglishWord(term) {
			t.Fatalf("%q should be a word", term)
		}
	}
	// Phrases, punctuation, digits, Chinese, whitespace and overlong terms
	// never reach the dictionary: a phrase cannot yield a whole definition.
	no := []string{"", " ", "\t", "hello world", "take into account", "out of the box",
		"hello\tworld", "hello\nworld", "  hello  world  ", "hello, world", "hello.", "hello!",
		"(hello)", "hello?", "a.b", "a/b", "a_b", "hello-world!", "take2", "2hello", "123",
		"不", "你好", "hello 你好", "日本語", "-hello", "'hello", "🙂", "é",
		strings.Repeat("a", 49)}
	for _, term := range no {
		if isSingleEnglishWord(term) {
			t.Fatalf("%q should not be a word", term)
		}
	}
}

func TestDetectDirection(t *testing.T) {
	cases := []struct {
		text           string
		source, target string
	}{
		{"纯中文", "zh-Hans", "en"},
		{"用 Cubbix", "zh-Hans", "en"},
		{"Cubbix 的翻译", "zh-Hans", "en"},
		{"hello 你好", "zh-Hans", "en"},
		{"日本語", "zh-Hans", "en"}, // known deviation: pure-Han Japanese
		{"hello", "auto", "zh-Hans"},
		{"日本語です", "auto", "zh-Hans"},
		{"안녕", "auto", "zh-Hans"},
		{"123 !?", "auto", "zh-Hans"},
	}
	for _, c := range cases {
		source, target := detectDirection(c.text)
		if source != c.source || target != c.target {
			t.Fatalf("%q = %s/%s; want %s/%s", c.text, source, target, c.source, c.target)
		}
	}
}

func TestClipTranslateInput(t *testing.T) {
	if got, dropped := clipTranslateInput("short"); got != "short" || dropped != 0 {
		t.Fatalf("short = %q, %d", got, dropped)
	}
	exact := strings.Repeat("字", maxTranslateInput)
	if got, dropped := clipTranslateInput(exact); got != exact || dropped != 0 {
		t.Fatalf("boundary = %d dropped", dropped)
	}
	long := exact + "🙂\nmore"
	got, dropped := clipTranslateInput(long)
	if got != exact || dropped != len([]rune("\U0001F642\nmore")) {
		t.Fatalf("clipped = %d dropped %d", len([]rune(got)), dropped)
	}
}

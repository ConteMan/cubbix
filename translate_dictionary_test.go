package main

import (
	"strings"
	"testing"
)

func TestDictionaryRepeatedLookup(t *testing.T) {
	calls := make(map[string]int)
	lookup := func(term string) string {
		calls[term]++
		return "noun 1 a definition"
	}
	cache := newDictionaryCache(lookup, 2)
	for _, term := range []string{"hello", "Hello", "HELLO"} {
		if got := cache.definition(term); got == "" {
			t.Fatalf("%q lost the definition", term)
		}
	}
	// The cache is case-insensitive and only avoids repeated system calls.
	if calls["hello"] != 1 {
		t.Fatalf("cached lookups = %d; want 1", calls["hello"])
	}
	if got := cache.definition("take into account"); got != "" {
		t.Fatalf("a phrase was looked up: %q", got)
	}
	cache.definition("world")
	cache.definition("serendipity")
	if _, ok := cache.entries["hello"]; ok {
		t.Fatal("oldest entry was not evicted")
	}
	cache.definition("hello")
	if calls["hello"] != 2 {
		t.Fatalf("evicted entry was not queried again: %d", calls["hello"])
	}
	// With the cache disabled (or after eviction) repeated system calls are
	// legitimate: they are an optimization, not a correctness condition.
	withoutCache := newDictionaryCache(lookup, 0)
	for i := 0; i < 5; i++ {
		if got := withoutCache.definition("hello"); got == "" {
			t.Fatalf("repeated lookup %d failed", i)
		}
	}
	if calls["hello"] != 7 {
		t.Fatalf("repeated lookups = %d; want 7", calls["hello"])
	}
}

func TestDictionaryDefinitionWithoutResult(t *testing.T) {
	cache := newDictionaryCache(func(string) string { return "" }, dictionaryCacheLimit)
	for _, term := range []string{"hello", "take into account", "hello world", "你好", "123", ""} {
		if got := cache.definition(term); got != "" {
			t.Fatalf("%q produced a dictionary block: %q", term, got)
		}
	}
}

func TestCropDictionaryDefinition(t *testing.T) {
	short := "  noun 1 a short definition  "
	if got := cropDictionaryDefinition(short); got != strings.TrimSpace(short) {
		t.Fatalf("short definition changed: %q", got)
	}
	long := strings.Repeat("词", maxDictionaryRunes+100)
	got := cropDictionaryDefinition(long)
	if !strings.HasSuffix(got, "…") || len([]rune(got)) != maxDictionaryRunes+1 {
		t.Fatalf("cropped length = %d", len([]rune(got)))
	}
}

func TestDictionarySegments(t *testing.T) {
	definition := "hello hel·lo | həˈlō | noun 1 a greeting. 2 a question • a surprised greeting ORIGIN Old English"
	segments := dictionarySegments(definition)
	if len(segments) < 4 {
		t.Fatalf("definition was not split: %q", segments)
	}
	if strings.Join(dictionarySegmentsToWords(segments), " ") != strings.Join(strings.Fields(definition), " ") {
		t.Fatalf("segmentation lost text: %q", segments)
	}
	var pos, sense, bullet, heading bool
	for _, segment := range segments {
		if segment == "" {
			t.Fatal("empty segment")
		}
		switch {
		case strings.HasPrefix(segment, "noun "):
			pos = true
		case strings.HasPrefix(segment, "2 "):
			sense = true
		case strings.HasPrefix(segment, "•"):
			bullet = true
		case strings.HasPrefix(segment, "ORIGIN"):
			heading = true
		}
	}
	if !pos || !sense || !bullet || !heading {
		t.Fatalf("part of speech, sense, bullet or heading missing: %q", segments)
	}
	if got := dictionarySegments("one plain line without markers"); len(got) != 1 || got[0] != "one plain line without markers" {
		t.Fatalf("plain line changed: %q", got)
	}
	if got := dictionarySegments(""); len(got) != 0 {
		t.Fatalf("empty definition produced segments: %q", got)
	}
	// A very long entry stays bounded, with a visible truncation marker.
	long := strings.Repeat("noun 1 a sense • another sense ", 400)
	if got := dictionarySegments(long); len(got) > maxDictionarySegments+1 || got[len(got)-1] != "…" {
		t.Fatalf("long definition was not bounded: %d segments", len(got))
	}
}

// dictionarySegmentsToWords recovers the plain words from display segments so
// the test can assert that layout never changes the definition's text.
func dictionarySegmentsToWords(segments []string) []string {
	var words []string
	for _, segment := range segments {
		words = append(words, strings.Fields(segment)...)
	}
	return words
}

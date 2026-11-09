//go:build darwin

package main

import (
	"strings"
	"testing"
	"unicode/utf16"
)

// TestDictionarySystemLookup exercises the read-only system API. It asserts
// only that definitions exist and have displayable structure: no test depends
// on the exact wording of an installed dictionary.
func TestDictionarySystemLookup(t *testing.T) {
	definition := systemDictionaryLookup("hello")
	if strings.TrimSpace(definition) == "" {
		t.Skip("no local dictionary entry available on this system")
	}
	if len(dictionarySegments(definition)) == 0 || !strings.Contains(definition, " ") {
		t.Fatal("definition has no displayable structure")
	}

	// The whole term must be covered by the detected range. A phrase can hit
	// only one of its words, so it is rejected before any definition is used.
	phrase := "take into account"
	cfPhrase := dictionaryHandles.createString(0, phrase, cfStringEncodingUTF8)
	if cfPhrase == 0 {
		t.Fatal("could not build the phrase CFString")
	}
	defer dictionaryHandles.release(cfPhrase)
	found := dictionaryHandles.termRange(0, cfPhrase, 0)
	if found.Length > 0 && found.Length < int64(len(utf16.Encode([]rune(phrase)))) {
		t.Logf("phrase matched only part of the term: %+v", found)
	} else {
		t.Logf("phrase reported a full or empty range; the word predicate still rejects it: %+v", found)
	}
	for _, term := range []string{"hello world", "take into account", "out of the box", "qwertyuiopzzz"} {
		if got := systemDictionaryLookup(term); got != "" {
			t.Fatalf("%q produced a definition", term)
		}
	}

	// Repeated system calls succeed whether or not the cache is enabled.
	withoutCache := newDictionaryCache(systemDictionaryLookup, 0)
	for i := 0; i < 2; i++ {
		if got := withoutCache.definition("serendipity"); strings.TrimSpace(got) == "" {
			t.Skip("serendipity has no local entry")
		}
	}
	if got := dictionaryDefinition("Hello"); strings.TrimSpace(got) == "" {
		t.Fatal("cached definitions are unavailable")
	}
	if got := dictionaryDefinition("take into account"); got != "" {
		t.Fatalf("a phrase opened a dictionary block: %q", got)
	}
}

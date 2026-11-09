package main

import (
	"strconv"
	"strings"
	"sync"
)

// The macOS dictionary returns one unstructured plain-text definition: head
// word, syllables, phonetics, parts of speech, senses, examples and origin all
// share a single string. These helpers only prepare that text for display;
// they never build a typed model of a definition.
const (
	dictionaryCacheLimit     = 64
	maxDictionaryRunes       = 2000
	maxDictionarySegments    = 48
	maxDictionarySystemBytes = 1 << 20
)

// dictionaryLookup is the system seam. Tests inject a counting fake; the
// darwin implementation reads the local dictionaries through purego.
type dictionaryLookup func(string) string

// dictionaryCache is a small process-level cache. It exists only to avoid
// calling the system again for a word already looked up: a miss, an eviction
// or a disabled cache may call the system as often as needed.
type dictionaryCache struct {
	mu      sync.Mutex
	lookup  dictionaryLookup
	limit   int
	entries map[string]string
	order   []string
}

func newDictionaryCache(lookup dictionaryLookup, limit int) *dictionaryCache {
	return &dictionaryCache{lookup: lookup, limit: limit, entries: make(map[string]string)}
}

func (c *dictionaryCache) definition(term string) string {
	if !isSingleEnglishWord(term) {
		return ""
	}
	key := strings.ToLower(strings.TrimSpace(term))
	c.mu.Lock()
	defer c.mu.Unlock()
	if value, ok := c.entries[key]; ok {
		return value
	}
	value := cropDictionaryDefinition(c.lookup(strings.TrimSpace(term)))
	if c.limit > 0 {
		c.entries[key] = value
		c.order = append(c.order, key)
		for len(c.order) > c.limit {
			delete(c.entries, c.order[0])
			c.order = c.order[1:]
		}
	}
	return value
}

var sharedDictionaryCache = newDictionaryCache(systemDictionaryLookup, dictionaryCacheLimit)

// dictionaryDefinition returns "" when the term is not a single English word
// or when the system has no definition. The caller then omits the block.
func dictionaryDefinition(term string) string {
	return sharedDictionaryCache.definition(term)
}

// cropDictionaryDefinition bounds the plain text held in the UI state. Long
// definitions still scroll inside the dictionary block; the bound keeps a
// single system entry from dominating the layout.
func cropDictionaryDefinition(definition string) string {
	text := strings.TrimSpace(definition)
	runes := []rune(text)
	if len(runes) <= maxDictionaryRunes {
		return text
	}
	return strings.TrimSpace(string(runes[:maxDictionaryRunes])) + "…"
}

// Parts of speech and section headings used as display breakpoints. The text
// remains plain text: these are only places to start a new line.
var dictionaryPartOfSpeech = map[string]bool{
	"noun": true, "verb": true, "adjective": true, "adverb": true,
	"pronoun": true, "preposition": true, "conjunction": true,
	"interjection": true, "exclamation": true, "contraction": true,
	"article": true, "determiner": true, "numeral": true,
	"prefix": true, "suffix": true, "abbreviation": true,
	"auxiliary": true, "intransitive": true, "transitive": true,
	"plural": true, "comparative": true, "superlative": true,
}

var dictionarySectionHeadings = map[string]bool{
	"ORIGIN": true, "DERIVATIVES": true, "PHRASES": true,
	"PHRASAL": true, "USAGE": true, "EXAMPLES": true,
	"WORD": true, "SYNONYMS": true, "ANTONYMS": true,
}

func dictionaryBareWord(word string) string {
	return strings.Trim(word, ".,;:!?()[]{}'\"“”‘’")
}

func dictionarySenseNumber(word string) bool {
	value, err := strconv.Atoi(word)
	return err == nil && value >= 1 && value <= 20 && len(word) <= 2
}

// dictionarySegments lays the plain-text definition out in readable lines:
// parts of speech, numbered senses, bullet senses and section headings each
// start a new paragraph. Words are never interpreted or removed.
func dictionarySegments(definition string) []string {
	words := strings.Fields(definition)
	var segments []string
	var current []string
	flush := func() {
		if len(current) > 0 {
			segments = append(segments, strings.Join(current, " "))
			current = nil
		}
	}
	for i, word := range words {
		bare := dictionaryBareWord(word)
		previous := ""
		if i > 0 {
			previous = words[i-1]
			switch {
			case word == "•" || strings.HasPrefix(word, "•"):
				flush()
			case dictionarySectionHeadings[word]:
				flush()
			case dictionaryPartOfSpeech[strings.ToLower(bare)] && word == bare:
				flush()
			case dictionarySenseNumber(word) &&
				(strings.HasSuffix(previous, ".") || strings.HasSuffix(previous, "|")):
				flush()
			}
		}
		current = append(current, word)
	}
	flush()
	if len(segments) > maxDictionarySegments {
		segments = append(segments[:maxDictionarySegments], "…")
	}
	return segments
}

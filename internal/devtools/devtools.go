// Package devtools converts Unix timestamps, dates and UUIDs for the command
// palette and, in a later step, the tool window and the command line.
//
// It reads no clipboard and opens no window. Generation is a separate call so
// a caller can cache one result per query: the palette redraws every frame,
// and a UUID generated during a frame would change before the user copied it.
package devtools

import (
	"strings"
	"time"
)

// Kind says what a result is about. The palette picks an icon from it.
type Kind int

const (
	KindTime Kind = iota
	KindUUID
)

// Row is one copyable result. Title is what the panel shows; Payload is what
// Enter writes to the clipboard. The two differ when the title is a readable
// clock face and the payload keeps its fractional seconds.
type Row struct {
	Kind     Kind
	Title    string
	Payload  string
	HintKey  string
	HintArgs []any
}

// Output is everything one query produces. Payload is the default row's
// payload; an error has no rows and only names a resource key.
type Output struct {
	Kind     Kind
	Rows     []Row
	Payload  string
	ErrorKey string
}

func errorOutput(key string) Output { return Output{ErrorKey: key} }

// Query recognizes a whole query. recognized is false when the text is an
// ordinary search rather than a devtools request.
func Query(text string, now time.Time, loc *time.Location) (Output, bool) {
	if loc == nil {
		loc = time.Local
	}
	query := strings.TrimSpace(text)
	if query == "" {
		return Output{}, false
	}
	token, arg := splitToken(query)
	lower := strings.ToLower(token)

	// Prefixes match a whole token: "timestamped" and "settings" stay searches.
	switch lower {
	case "ts", "timestamp", "时间戳":
		return ConvertTime(arg, now, loc), true
	case "now":
		if arg == "" {
			return CurrentTime(now, loc), true
		}
	}

	if version, count, state := uuidRequest(lower, arg); state != uuidNotRequest {
		if state == uuidInvalidCount {
			return errorOutput("DevtoolsCountInvalid"), true
		}
		if count > 0 {
			return GenerateUUID(version, count, false, false), true
		}
		if version == 7 {
			return GenerateUUID(7, 1, false, false), true
		}
		return GenerateUUIDPair(), true
	}

	if output, ok := ParseUUID(query); ok {
		return output, true
	}
	if output, ok := ConvertBareTimestamp(query, loc); ok {
		return output, true
	}
	if output, ok := ConvertDate(query, loc); ok {
		return output, true
	}
	return Output{}, false
}

// splitToken splits at the first run of whitespace. The argument keeps its
// original case.
func splitToken(text string) (string, string) {
	index := strings.IndexFunc(text, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
	if index < 0 {
		return text, ""
	}
	return text[:index], strings.TrimSpace(text[index:])
}

type uuidRequestState int

const (
	uuidNotRequest uuidRequestState = iota
	uuidGenerate
	uuidInvalidCount
)

// uuidRequest reads the generation forms and their optional count. A signed
// or non-numeric argument is not a generation request, so the query stays a
// search; a numeric count outside 1-100 is an error row.
func uuidRequest(lower, arg string) (int, int, uuidRequestState) {
	version := 0
	switch lower {
	case "uuid", "uuid4", "uuidv4":
		version = 4
	case "uuid7", "uuidv7":
		version = 7
	default:
		return 0, 0, uuidNotRequest
	}
	if arg == "" {
		return version, 0, uuidGenerate
	}
	if arg[0] == '+' {
		return 0, 0, uuidNotRequest
	}
	negative := arg[0] == '-'
	digits := arg
	if negative {
		digits = arg[1:]
	}
	if digits == "" || !allDigits(digits) {
		return 0, 0, uuidNotRequest
	}
	count, ok := parseCount(digits)
	if !ok || negative || count < 1 || count > 100 {
		return version, 0, uuidInvalidCount
	}
	return version, count, uuidGenerate
}

func allDigits(s string) bool {
	for index := 0; index < len(s); index++ {
		if s[index] < '0' || s[index] > '9' {
			return false
		}
	}
	return true
}

// parseCount parses a digit string with an overflow check, never wrapping.
func parseCount(digits string) (int, bool) {
	value := 0
	for index := 0; index < len(digits); index++ {
		digit := int(digits[index] - '0')
		if value > (int(^uint(0)>>1)-digit)/10 {
			return 0, false
		}
		value = value*10 + digit
	}
	return value, true
}

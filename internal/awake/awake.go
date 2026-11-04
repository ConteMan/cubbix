// Package awake parses the keep awake argument grammar shared by the command
// palette, cubbix:// URLs and the command line tool.
//
// It depends on the standard library only: the CLI links it without MyGo.
package awake

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Minutes bounds, mirrored from the custom duration dialog.
const (
	MinutesMinimum = 1
	MinutesMaximum = 525659 // 8760 hours 59 minutes
)

// Kind is what a parsed argument asks for. The zero value uses the remembered
// session, so an empty argument parses into it.
type Kind int

// Argument kinds.
const (
	KindDefault Kind = iota
	KindMinutes
	KindUntil
	KindUnlimited
)

// Arg is a parsed keep awake argument.
type Arg struct {
	Kind    Kind
	Minutes int    // KindMinutes
	Until   string // KindUntil, "18:30"
	Lid     bool   // the 合盖/lid modifier was present
	Error   string // resource key of an invalid argument
}

// ParseArg parses the whole argument; every grammar must match end to end.
// Combined forms come before a lone hour or minute.
func ParseArg(arg string, now time.Time) (result Arg) {
	value, lidOn := StripLidModifier(arg)
	defer func() { result.Lid = lidOn }()
	return parseDuration(value, now)
}

// StripLidModifier removes one leading or trailing 合盖/lid word.
func StripLidModifier(arg string) (string, bool) {
	value := strings.TrimSpace(arg)
	lower := strings.ToLower(value)
	for _, word := range []string{"合盖", "lid1", "lid", "盖"} {
		if strings.HasPrefix(lower, word) {
			return strings.TrimSpace(value[len(word):]), true
		}
		if strings.HasSuffix(lower, word) {
			return strings.TrimSpace(value[:len(value)-len(word)]), true
		}
	}
	return value, false
}

func parseDuration(arg string, now time.Time) Arg {
	value := strings.TrimSpace(arg)
	if value == "" {
		return Arg{Kind: KindDefault}
	}
	lower := strings.ToLower(value)
	switch lower {
	case "∞", "inf", "infinity", "unlimited", "无限", "不限", "不限时", "0":
		return Arg{Kind: KindUnlimited}
	}
	if until, ok := untilArg(lower); ok {
		if _, err := UntilTime(until, now); err != nil {
			return Arg{Kind: KindUntil, Error: "TimeError"}
		}
		return Arg{Kind: KindUntil, Until: until}
	}
	minutes, ok := parseMinutes(lower)
	if !ok || minutes < MinutesMinimum || minutes > MinutesMaximum {
		return Arg{Error: "DurationError"}
	}
	return Arg{Kind: KindMinutes, Minutes: minutes}
}

// untilArg recognizes "until 18:30", "到 18:30", "直到 18:30" and a bare clock
// time. Validation is left to UntilTime, the same function the dialog uses.
func untilArg(value string) (string, bool) {
	for _, prefix := range []string{"until", "直到", "到"} {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimSpace(value[len(prefix):]), true
		}
	}
	if strings.Contains(value, ":") {
		return value, true
	}
	return "", false
}

// parseMinutes parses 45, 45m, 2h, 1h30m, 1h 30m and the Chinese forms,
// returning minutes.
var (
	hoursPattern   = regexp.MustCompile(`^([0-9]+)\s*(?:h|小时|hours?|hrs?)(?:\s*([0-9]+)\s*(?:m|mins?|分钟|分)?)?$`)
	minutesPattern = regexp.MustCompile(`^([0-9]+)\s*(?:m|mins?|分钟|分)?$`)
)

func parseMinutes(value string) (int, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if match := hoursPattern.FindStringSubmatch(value); match != nil {
		hours, err := strconv.Atoi(match[1])
		if err != nil || hours > MinutesMaximum/60 {
			return 0, false
		}
		minutes := 0
		if match[2] != "" {
			minutes, err = strconv.Atoi(match[2])
			if err != nil || minutes > 59 {
				return 0, false
			}
		}
		return hours*60 + minutes, true
	}
	if match := minutesPattern.FindStringSubmatch(value); match != nil {
		minutes, err := strconv.Atoi(match[1])
		return minutes, err == nil
	}
	return 0, false
}

// UntilTime turns "18:30" into the next moment with that clock time: today if
// it is still ahead, tomorrow otherwise.
func UntilTime(value string, now time.Time) (time.Time, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return time.Time{}, err
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, now.Location())
	if !end.After(now) {
		end = end.AddDate(0, 0, 1)
	}
	return end, nil
}

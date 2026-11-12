package devtools

import (
	"strconv"
	"strings"
	"time"
)

// Unit is the unit a timestamp was written in.
type Unit int

const (
	UnitSecond Unit = iota
	UnitMillisecond
	UnitMicrosecond
	UnitNanosecond
)

// Hint resource keys. The package stays free of interface text; the caller
// formats them.
const (
	HintLocal      = "DevtoolsLocal"
	HintUTC        = "DevtoolsUTC"
	HintUnixSecond = "DevtoolsUnixSeconds"
	HintUnixMillis = "DevtoolsUnixMillis"
	HintOverlap    = "DevtoolsOverlap"
)

// displayYearMin and displayYearMax bound every instant this package shows.
// Both the local and the UTC year must fall inside them.
const (
	displayYearMin = 1
	displayYearMax = 9999
)

// ConvertTime reads the argument after a ts prefix. An empty argument or
// "now" means the current time; anything else must be a timestamp or a date.
// A prefixed input never falls back to a search, so junk is an error row.
func ConvertTime(text string, now time.Time, loc *time.Location) Output {
	if loc == nil {
		loc = time.Local
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.EqualFold(text, "now") {
		return CurrentTime(now, loc)
	}
	if instant, unit, key, recognized := parseTimestamp(text); recognized {
		if key != "" {
			return errorOutput(key)
		}
		if !withinDisplayYears(instant, loc) {
			return errorOutput("DevtoolsRange")
		}
		return timestampOutput(instant, loc, unit)
	}
	if output, ok := ConvertDate(text, loc); ok {
		return output
	}
	return errorOutput("DevtoolsDateInvalid")
}

// CurrentTime is the four-row result for now and a bare ts.
func CurrentTime(now time.Time, loc *time.Location) Output {
	if loc == nil {
		loc = time.Local
	}
	local := now.In(loc)
	utc := now.In(time.UTC)
	seconds := strconv.FormatInt(now.Unix(), 10)
	millis := strconv.FormatInt(now.UnixMilli(), 10)
	rows := []Row{
		{Kind: KindTime, Title: seconds, Payload: seconds, HintKey: HintUnixSecond},
		{Kind: KindTime, Title: millis, Payload: millis, HintKey: HintUnixMillis},
		{Kind: KindTime, Title: clockTitle(local), Payload: clockPayload(local), HintKey: HintLocal},
		{Kind: KindTime, Title: utc.Format(time.RFC3339Nano), Payload: utc.Format(time.RFC3339Nano), HintKey: HintUTC},
	}
	return Output{Kind: KindTime, Rows: rows, Payload: rows[0].Payload}
}

// ConvertBareTimestamp recognizes an unprefixed integer only by digit count
// and range: 10, 13, 16 and 19 digits from 2001-09-09 up to 2100-01-01. A
// number outside that window is not an error, it is simply a search.
func ConvertBareTimestamp(text string, loc *time.Location) (Output, bool) {
	if loc == nil {
		loc = time.Local
	}
	text = strings.TrimSpace(text)
	if text == "" || !allDigits(text) {
		return Output{}, false
	}
	var unit Unit
	var lower, upper uint64
	switch len(text) {
	case 10:
		unit, lower, upper = UnitSecond, 1_000_000_000, 4_102_444_800
	case 13:
		unit, lower, upper = UnitMillisecond, 1_000_000_000_000, 4_102_444_800_000
	case 16:
		unit, lower, upper = UnitMicrosecond, 1_000_000_000_000_000, 4_102_444_800_000_000
	case 19:
		unit, lower, upper = UnitNanosecond, 1_000_000_000_000_000_000, 4_102_444_800_000_000_000
	default:
		return Output{}, false
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil || value < lower || value >= upper {
		return Output{}, false
	}
	instant := instantFromUnit(int64(value), unit)
	if !withinDisplayYears(instant, loc) {
		return Output{}, false
	}
	return timestampOutput(instant, loc, unit), true
}

// ConvertDate reads a calendar date, with or without a time. recognized is
// false when the text is not date-shaped; a recognized but invalid date is an
// error row.
func ConvertDate(text string, loc *time.Location) (Output, bool) {
	if loc == nil {
		loc = time.Local
	}
	text = strings.TrimSpace(text)
	fields, key, recognized := parseDateFields(text)
	if !recognized {
		return Output{}, false
	}
	if key != "" {
		return errorOutput(key), true
	}
	if fields.year < displayYearMin || fields.year > displayYearMax {
		return errorOutput("DevtoolsRange"), true
	}
	wall := time.Date(fields.year, time.Month(fields.month), fields.day, fields.hour, fields.minute, fields.second, fields.nanos, time.UTC)
	zone := loc
	if fields.hasOffset {
		zone = time.FixedZone("", fields.offset)
	}
	candidates := resolveWallClock(wall, zone)
	if len(candidates) == 0 {
		return errorOutput("DevtoolsSkipped"), true
	}
	instant := candidates[0]
	if !withinDisplayYears(instant, loc) {
		return errorOutput("DevtoolsRange"), true
	}
	rows := dateRows(instant, loc)
	if len(candidates) > 1 {
		other := candidates[1]
		if !withinDisplayYears(other, loc) {
			return errorOutput("DevtoolsRange"), true
		}
		rows = append(rows, Row{
			Kind:     KindTime,
			Title:    clockTitle(other.In(loc)),
			Payload:  other.In(loc).Format(time.RFC3339Nano),
			HintKey:  HintOverlap,
			HintArgs: []any{other.In(loc).Format("-07:00")},
		})
	}
	return Output{Kind: KindTime, Rows: rows, Payload: rows[0].Payload}, true
}

// timestampOutput lists a timestamp's readable forms. The RFC lines use
// RFC3339Nano, so microseconds and nanoseconds survive even though the clock
// face itself does not show them.
func timestampOutput(instant time.Time, loc *time.Location, unit Unit) Output {
	local := instant.In(loc)
	utc := instant.In(time.UTC)
	localRFC := local.Format(time.RFC3339Nano)
	localClock, localPayload := clockTitle(local), clockPayload(local)
	rows := []Row{
		{Kind: KindTime, Title: localClock, Payload: localPayload, HintKey: HintLocal},
		{Kind: KindTime, Title: clockTitle(utc), Payload: clockPayload(utc), HintKey: HintUTC},
		{Kind: KindTime, Title: localRFC, Payload: localRFC},
	}
	seconds := strconv.FormatInt(instant.Unix(), 10)
	millis := strconv.FormatInt(instant.UnixMilli(), 10)
	switch unit {
	case UnitSecond:
		rows = append(rows, Row{Kind: KindTime, Title: millis, Payload: millis, HintKey: HintUnixMillis})
	case UnitMillisecond:
		rows = append(rows, Row{Kind: KindTime, Title: seconds, Payload: seconds, HintKey: HintUnixSecond})
	default:
		rows = append(rows,
			Row{Kind: KindTime, Title: seconds, Payload: seconds, HintKey: HintUnixSecond},
			Row{Kind: KindTime, Title: millis, Payload: millis, HintKey: HintUnixMillis},
		)
	}
	return Output{Kind: KindTime, Rows: rows, Payload: rows[0].Payload}
}

func dateRows(instant time.Time, loc *time.Location) []Row {
	local := instant.In(loc)
	utc := instant.In(time.UTC)
	seconds := strconv.FormatInt(instant.Unix(), 10)
	millis := strconv.FormatInt(instant.UnixMilli(), 10)
	return []Row{
		{Kind: KindTime, Title: seconds, Payload: seconds, HintKey: HintUnixSecond},
		{Kind: KindTime, Title: millis, Payload: millis, HintKey: HintUnixMillis},
		{Kind: KindTime, Title: utc.Format(time.RFC3339Nano), Payload: utc.Format(time.RFC3339Nano), HintKey: HintUTC},
		{Kind: KindTime, Title: clockTitle(local), Payload: clockPayload(local), HintKey: HintLocal},
	}
}

func clockTitle(t time.Time) string { return t.Format("2006-01-02 15:04:05") }

// clockPayload keeps the fractional part when there is one. A zero fraction
// is dropped so an ordinary whole second copies as the same short text the
// row shows.
func clockPayload(t time.Time) string {
	if t.Nanosecond() == 0 {
		return clockTitle(t)
	}
	return strings.TrimRight(t.Format("2006-01-02 15:04:05.000000000"), "0")
}

func withinDisplayYears(instant time.Time, loc *time.Location) bool {
	if loc == nil {
		loc = time.Local
	}
	for _, year := range []int{instant.In(loc).Year(), instant.UTC().Year()} {
		if year < displayYearMin || year > displayYearMax {
			return false
		}
	}
	return true
}

// parseTimestamp reads an optionally negative integer with an optional unit
// suffix. recognized is false when the text is not timestamp-shaped, so a
// date can be tried next.
func parseTimestamp(text string) (time.Time, Unit, string, bool) {
	text = strings.TrimSpace(text)
	negative := false
	if strings.HasPrefix(text, "-") {
		negative = true
		text = text[1:]
	} else if strings.HasPrefix(text, "+") {
		// A positive sign is not part of the accepted timestamp syntax.
		return time.Time{}, UnitSecond, "", false
	}
	digitsEnd := 0
	for digitsEnd < len(text) && text[digitsEnd] >= '0' && text[digitsEnd] <= '9' {
		digitsEnd++
	}
	if digitsEnd == 0 {
		return time.Time{}, UnitSecond, "", false
	}
	digits := text[:digitsEnd]
	suffix := strings.ToLower(strings.TrimSpace(text[digitsEnd:]))
	unit := UnitSecond
	explicit := true
	switch suffix {
	case "s":
		unit = UnitSecond
	case "ms":
		unit = UnitMillisecond
	case "us", "µs", "μs":
		unit = UnitMicrosecond
	case "ns":
		unit = UnitNanosecond
	case "":
		explicit = false
	default:
		// Not a unit: this is probably a date such as 2026-11-12.
		return time.Time{}, UnitSecond, "", false
	}
	value, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		// Larger than int64: reject explicitly instead of wrapping.
		return time.Time{}, UnitSecond, "DevtoolsRange", true
	}
	if !explicit {
		switch len(digits) {
		case 13:
			unit = UnitMillisecond
		case 16:
			unit = UnitMicrosecond
		case 19:
			unit = UnitNanosecond
		default:
			if len(digits) > 10 {
				return time.Time{}, UnitSecond, "DevtoolsUnit", true
			}
			unit = UnitSecond
		}
	}
	if negative {
		value = -value
	}
	return instantFromUnit(value, unit), unit, "", true
}

// instantFromUnit splits the value into whole seconds and a nanosecond
// remainder before constructing the time. Multiplying a microsecond or
// nanosecond count by its unit first would overflow int64 for years outside
// 1678-2262.
func instantFromUnit(value int64, unit Unit) time.Time {
	switch unit {
	case UnitMillisecond:
		return time.Unix(value/1_000, (value%1_000)*1_000_000)
	case UnitMicrosecond:
		return time.Unix(value/1_000_000, (value%1_000_000)*1_000)
	case UnitNanosecond:
		return time.Unix(value/1_000_000_000, value%1_000_000_000)
	}
	return time.Unix(value, 0)
}

// dateFields are the wall-clock fields of one parsed date.
type dateFields struct {
	year, month, day     int
	hour, minute, second int
	nanos                int
	offset               int
	hasOffset            bool
}

// parseDateFields validates the exact date shapes the module accepts. It
// rejects a positive offset hour above 23 and a minute above 59 before any
// parsing library can normalize +24:00 or +00:60.
func parseDateFields(text string) (dateFields, string, bool) {
	var fields dateFields
	if len(text) < 10 || (text[4] != '-' && text[4] != '/') || text[7] != text[4] {
		return fields, "", false
	}
	number := func(s string) (int, bool) {
		value := 0
		for index := 0; index < len(s); index++ {
			if s[index] < '0' || s[index] > '9' {
				return 0, false
			}
			value = value*10 + int(s[index]-'0')
		}
		return value, true
	}
	var ok bool
	if fields.year, ok = number(text[0:4]); !ok {
		return fields, "", false
	}
	separator := text[4]
	if fields.month, ok = number(text[5:7]); !ok || text[7] != separator {
		return fields, "", false
	}
	if fields.day, ok = number(text[8:10]); !ok {
		return fields, "", false
	}
	rest := text[10:]
	if rest != "" {
		if rest[0] != 'T' && rest[0] != ' ' {
			return fields, "", false
		}
		timePart := rest[1:]
		if timePart == "" {
			return fields, "DevtoolsDateInvalid", true
		}
		if last := timePart[len(timePart)-1]; last == 'Z' || last == 'z' {
			fields.hasOffset = true
			timePart = timePart[:len(timePart)-1]
		} else if len(timePart) >= 6 {
			sign := timePart[len(timePart)-6]
			if (sign == '+' || sign == '-') && timePart[len(timePart)-3] == ':' {
				offsetHour, hourOK := number(timePart[len(timePart)-5 : len(timePart)-3])
				offsetMinute, minuteOK := number(timePart[len(timePart)-2:])
				if !hourOK || !minuteOK || offsetHour > 23 || offsetMinute > 59 {
					return fields, "DevtoolsDateInvalid", true
				}
				fields.hasOffset = true
				fields.offset = offsetHour*3600 + offsetMinute*60
				if sign == '-' {
					fields.offset = -fields.offset
				}
				timePart = timePart[:len(timePart)-6]
			}
		}
		if len(timePart) < 5 || timePart[2] != ':' {
			return fields, "DevtoolsDateInvalid", true
		}
		if fields.hour, ok = number(timePart[0:2]); !ok || fields.hour > 23 {
			return fields, "DevtoolsDateInvalid", true
		}
		if fields.minute, ok = number(timePart[3:5]); !ok || fields.minute > 59 {
			return fields, "DevtoolsDateInvalid", true
		}
		timePart = timePart[5:]
		if timePart != "" {
			if timePart[0] != ':' || len(timePart) < 3 {
				return fields, "DevtoolsDateInvalid", true
			}
			if fields.second, ok = number(timePart[1:3]); !ok {
				return fields, "DevtoolsDateInvalid", true
			}
			if fields.second == 60 {
				// Leap seconds are not supported and keep their own message.
				return fields, "DevtoolsLeapSecond", true
			}
			if fields.second > 59 {
				return fields, "DevtoolsDateInvalid", true
			}
			timePart = timePart[3:]
			if timePart != "" {
				if timePart[0] != '.' || len(timePart) < 2 || len(timePart) > 10 {
					return fields, "DevtoolsDateInvalid", true
				}
				fraction := timePart[1:]
				scale := 100_000_000
				for index := 0; index < len(fraction); index++ {
					if fraction[index] < '0' || fraction[index] > '9' {
						return fields, "DevtoolsDateInvalid", true
					}
					fields.nanos += int(fraction[index]-'0') * scale
					scale /= 10
				}
			}
		}
	}
	if fields.month < 1 || fields.month > 12 || fields.day < 1 || fields.day > daysInMonth(fields.year, fields.month) {
		return fields, "DevtoolsDateInvalid", true
	}
	return fields, "", true
}

func daysInMonth(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	}
	return 0
}

type instantKey struct {
	sec  int64
	nsec int
}

// resolveWallClock finds every instant that really shows the given wall clock
// in zone. Offsets come from the zone's own transitions around that time, not
// from adding or subtracting a fixed hour, so 30-minute and two-hour shifts
// are handled too. Each candidate is verified field by field.
func resolveWallClock(wall time.Time, zone *time.Location) []time.Time {
	wallSeconds := wall.Unix()
	nanos := int64(wall.Nanosecond())
	offsets := map[int]bool{}
	start := time.Unix(wallSeconds-2*24*60*60, 0)
	end := time.Unix(wallSeconds+2*24*60*60, 0)
	for cursor := start; ; {
		_, offset := cursor.In(zone).Zone()
		offsets[offset] = true
		_, next := cursor.In(zone).ZoneBounds()
		if next.IsZero() || !next.After(cursor) || next.After(end) {
			break
		}
		cursor = next
	}
	_, endOffset := end.In(zone).Zone()
	offsets[endOffset] = true

	var candidates []time.Time
	seen := map[instantKey]bool{}
	for offset := range offsets {
		candidate := time.Unix(wallSeconds-int64(offset), nanos)
		shown := candidate.In(zone)
		if shown.Year() != wall.Year() || shown.Month() != wall.Month() || shown.Day() != wall.Day() ||
			shown.Hour() != wall.Hour() || shown.Minute() != wall.Minute() || shown.Second() != wall.Second() ||
			shown.Nanosecond() != wall.Nanosecond() {
			continue
		}
		key := instantKey{sec: candidate.Unix(), nsec: candidate.Nanosecond()}
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, candidate)
	}
	for left := 0; left < len(candidates); left++ {
		for right := left + 1; right < len(candidates); right++ {
			if candidates[right].Before(candidates[left]) {
				candidates[left], candidates[right] = candidates[right], candidates[left]
			}
		}
	}
	return candidates
}

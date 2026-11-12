package devtools

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("zone %s: %v", name, err)
	}
	return loc
}

func hints(output Output) []string {
	var keys []string
	for _, row := range output.Rows {
		keys = append(keys, row.HintKey)
	}
	return keys
}

func TestBareTimestampNeedsDigitsAndRange(t *testing.T) {
	utc := time.UTC
	for _, text := range []string{"1700000000", "1700000000000", "1700000000000000", "1700000000000000000"} {
		if _, ok := ConvertBareTimestamp(text, utc); !ok {
			t.Fatalf("%s did not hit", text)
		}
	}
	for _, text := range []string{"946684800", "17000000000", "170000000000", "4102444800", "9999999999", "-1700000000", "1700000000s"} {
		if _, ok := ConvertBareTimestamp(text, utc); ok {
			t.Fatalf("%s hit", text)
		}
	}
}

func TestTimestampPrefixAndUnits(t *testing.T) {
	utc := time.UTC
	output := ConvertTime("946684800", time.Now(), utc)
	if output.ErrorKey != "" || output.Rows[0].Title != "2000-01-01 00:00:00" {
		t.Fatalf("ts 946684800 = %+v", output)
	}
	output = ConvertTime("-1", time.Now(), utc)
	if output.Rows[0].Title != "1969-12-31 23:59:59" || output.Rows[1].Title != "1969-12-31 23:59:59" {
		t.Fatalf("ts -1 = %+v", output)
	}
	if key := ConvertTime("946684800000", time.Now(), utc).ErrorKey; key != "DevtoolsUnit" {
		t.Fatalf("12-digit without a suffix = %q", key)
	}
	if key := ConvertTime("946684800000ms", time.Now(), utc).ErrorKey; key != "" {
		t.Fatalf("suffixed 12-digit = %q", key)
	}
	if key := ConvertTime("946684800S", time.Now(), utc).ErrorKey; key != "" {
		t.Fatalf("uppercase suffix = %q", key)
	}
	if key := ConvertTime("+8", time.Now(), utc).ErrorKey; key != "DevtoolsDateInvalid" {
		t.Fatalf("positive sign = %q", key)
	}
	if key := ConvertTime("1e9", time.Now(), utc).ErrorKey; key != "DevtoolsDateInvalid" {
		t.Fatalf("exponent = %q", key)
	}
	// 2038 has no overflow.
	if title := ConvertTime("2147483648", time.Now(), utc).Rows[0].Title; title != "2038-01-19 03:14:08" {
		t.Fatalf("2038 = %q", title)
	}
}

func TestTimestampYearsOverflowAndLeapSecond(t *testing.T) {
	utc := time.UTC
	for _, text := range []string{"253402300800s", "9999999999999999999ns", "-62135596801s"} {
		if key := ConvertTime(text, time.Now(), utc).ErrorKey; key != "DevtoolsRange" {
			t.Fatalf("%s = %q", text, key)
		}
	}
	// 19 nines in nanosecond unit is larger than int64: reject, never wrap.
	if key := ConvertTime("9999999999999999999", time.Now(), utc).ErrorKey; key != "DevtoolsRange" {
		t.Fatalf("overflow = %q", key)
	}
	if key := ConvertTime("2016-12-31T23:59:60Z", time.Now(), utc).ErrorKey; key != "DevtoolsLeapSecond" {
		t.Fatalf("leap second = %q", key)
	}
}

func TestTimestampKeepsFraction(t *testing.T) {
	utc := time.UTC
	output := ConvertTime("2026-11-12T00:00:00.123456789Z", time.Now(), utc)
	if output.ErrorKey != "" {
		t.Fatalf("fraction rejected: %s", output.ErrorKey)
	}
	found := false
	for _, row := range output.Rows {
		if row.Payload == "2026-11-12T00:00:00.123456789Z" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no RFC3339Nano row: %+v", output.Rows)
	}
	output = ConvertTime("1700000000123456us", time.Now(), utc)
	if output.ErrorKey != "" {
		t.Fatalf("microseconds rejected: %s", output.ErrorKey)
	}
	found = false
	for _, row := range output.Rows {
		if strings.Contains(row.Payload, ".123456") {
			found = true
		}
	}
	if !found {
		t.Fatalf("microseconds lost: %+v", output.Rows)
	}
}

func TestSkippedWallClock(t *testing.T) {
	loc := zone(t, "America/New_York")
	output, _ := ConvertDate("2026-03-08 02:30:00", loc)
	if output.ErrorKey != "DevtoolsSkipped" || len(output.Rows) != 0 {
		t.Fatalf("skipped = %+v", output)
	}
}

func TestOverlappingWallClocks(t *testing.T) {
	cases := []struct {
		zone, text, other string
	}{
		{"America/New_York", "2026-11-01 01:30:00", "-05:00"},
		{"Europe/London", "2026-10-25 01:30:00", "+00:00"},
		{"Australia/Lord_Howe", "2026-04-05 01:45:00", "+10:30"},
	}
	for _, test := range cases {
		loc := zone(t, test.zone)
		output, _ := ConvertDate(test.text, loc)
		if output.ErrorKey != "" {
			t.Fatalf("%s: %s", test.text, output.ErrorKey)
		}
		if len(output.Rows) != 5 {
			t.Fatalf("%s: %d rows", test.zone, len(output.Rows))
		}
		other := output.Rows[len(output.Rows)-1]
		if other.HintKey != "DevtoolsOverlap" || len(other.HintArgs) != 1 || other.HintArgs[0] != test.other {
			t.Fatalf("%s: other row = %+v", test.zone, other)
		}
		earlierInstant, _ := time.Parse(time.RFC3339Nano, other.Payload)
		if !time.Unix(earlierInstant.Unix(), 0).After(time.Unix(mustInt(output.Payload), 0)) {
			t.Fatalf("%s: the default is not the earlier instant: %q vs %q", test.zone, output.Payload, other.Payload)
		}
	}
}

func mustInt(text string) int64 {
	value, _ := strconv.ParseInt(text, 10, 64)
	return value
}

func TestWallClockYearBoundariesReject(t *testing.T) {
	utc := time.UTC
	if output, _ := ConvertDate("0001-01-01T00:00:00+14:00", utc); output.ErrorKey != "DevtoolsRange" {
		t.Fatalf("year 0 UTC = %q", output.ErrorKey)
	}
	if output, _ := ConvertDate("9999-12-31T23:59:59-12:00", utc); output.ErrorKey != "DevtoolsRange" {
		t.Fatalf("year 10000 UTC = %q", output.ErrorKey)
	}
}

func TestDateShapesAndStrictOffsets(t *testing.T) {
	utc := time.UTC
	for _, text := range []string{"01/02/2006", "2026-3-8", "see 1700000000"} {
		if _, ok := ConvertDate(text, utc); ok {
			t.Fatalf("%s recognized", text)
		}
	}
	if _, ok := ConvertDate("2026/11/12", utc); !ok {
		t.Fatal("slash date not recognized")
	}
	for _, text := range []string{"2026-11-12T00:00:00+24:00", "2026-11-12T00:00:00+00:60", "2026-13-01", "2026-01-32"} {
		if output, _ := ConvertDate(text, utc); output.ErrorKey != "DevtoolsDateInvalid" {
			t.Fatalf("%s = %q", text, output.ErrorKey)
		}
	}
}

func TestQueryDoesNotSwallowSimilarTokens(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	for _, text := range []string{"timestamped", "settings", "ts946684800", "uuid v4", "uuid +8", "uuid 8x"} {
		if _, ok := Query(text, now, time.UTC); ok {
			t.Fatalf("%q was swallowed", text)
		}
	}
	if _, ok := Query("时间戳 1700000000", now, time.UTC); !ok {
		t.Fatal("Chinese prefix not recognized")
	}
}

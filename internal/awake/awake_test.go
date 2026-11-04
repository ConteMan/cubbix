package awake

import (
	"testing"
	"time"
)

func TestParseArg(t *testing.T) {
	now := time.Date(2026, 10, 30, 14, 0, 0, 0, time.Local)
	cases := []struct {
		arg     string
		kind    Kind
		minutes int
		until   string
		errKey  string
		lid     bool
	}{
		{"", KindDefault, 0, "", "", false},
		{"∞", KindUnlimited, 0, "", "", false},
		{"inf", KindUnlimited, 0, "", "", false},
		{"infinity", KindUnlimited, 0, "", "", false},
		{"unlimited", KindUnlimited, 0, "", "", false},
		{"无限", KindUnlimited, 0, "", "", false},
		{"不限", KindUnlimited, 0, "", "", false},
		{"不限时", KindUnlimited, 0, "", "", false},
		{"0", KindUnlimited, 0, "", "", false},
		{"45", KindMinutes, 45, "", "", false},
		{"45m", KindMinutes, 45, "", "", false},
		{"45min", KindMinutes, 45, "", "", false},
		{"45分钟", KindMinutes, 45, "", "", false},
		{"45 分", KindMinutes, 45, "", "", false},
		{"2h", KindMinutes, 120, "", "", false},
		{"2小时", KindMinutes, 120, "", "", false},
		{"2 hour", KindMinutes, 120, "", "", false},
		{"2hours", KindMinutes, 120, "", "", false},
		{"1h30m", KindMinutes, 90, "", "", false},
		{"1h30", KindMinutes, 90, "", "", false},
		{"1h 30m", KindMinutes, 90, "", "", false},
		{"1小时30分钟", KindMinutes, 90, "", "", false},
		{"until 18:30", KindUntil, 0, "18:30", "", false},
		{"到 18:30", KindUntil, 0, "18:30", "", false},
		{"直到 18:30", KindUntil, 0, "18:30", "", false},
		{"18:30", KindUntil, 0, "18:30", "", false},
		{"1", KindMinutes, 1, "", "", false},
		{"525659", KindMinutes, 525659, "", "", false},
		{"8760h59m", KindMinutes, 525659, "", "", false},
		{"525660", KindDefault, 0, "", "DurationError", false},
		{"8761h", KindDefault, 0, "", "DurationError", false},
		{"1h60m", KindDefault, 0, "", "DurationError", false},
		{"45x", KindDefault, 0, "", "DurationError", false},
		{"abc", KindDefault, 0, "", "DurationError", false},
		{"1h30mx", KindDefault, 0, "", "DurationError", false},
		{"0m", KindDefault, 0, "", "DurationError", false},
		{"-1", KindDefault, 0, "", "DurationError", false},
		{"1.5h", KindDefault, 0, "", "DurationError", false},
		{"until 99:99", KindUntil, 0, "", "TimeError", false},
		{"until", KindUntil, 0, "", "TimeError", false},
		// The lid modifier is one leading or trailing word, on either side.
		{"2h 合盖", KindMinutes, 120, "", "", true},
		{"合盖 2h", KindMinutes, 120, "", "", true},
		{"2h lid", KindMinutes, 120, "", "", true},
		{"lid1 45m", KindMinutes, 45, "", "", true},
		{"合盖", KindDefault, 0, "", "", true},
		{"合盖盖", KindDefault, 0, "", "DurationError", true},
		{"∞ 合盖", KindUnlimited, 0, "", "", true},
	}
	for _, scenario := range cases {
		got := ParseArg(scenario.arg, now)
		if got.Kind != scenario.kind || got.Minutes != scenario.minutes || got.Until != scenario.until ||
			got.Error != scenario.errKey || got.Lid != scenario.lid {
			t.Fatalf("ParseArg(%q) = %+v; want kind=%v minutes=%d until=%q error=%q lid=%v",
				scenario.arg, got, scenario.kind, scenario.minutes, scenario.until, scenario.errKey, scenario.lid)
		}
	}
}

func TestParseArgCombinedBeforeSingleUnit(t *testing.T) {
	if arg := ParseArg("1h30m", time.Now()); arg.Kind != KindMinutes || arg.Minutes != 90 {
		t.Fatalf("combined duration = %+v", arg)
	}
	if single := ParseArg("1h", time.Now()); single.Minutes != 60 {
		t.Fatalf("single hour = %+v", single)
	}
	if single := ParseArg("30m", time.Now()); single.Minutes != 30 {
		t.Fatalf("single minute = %+v", single)
	}
}

func TestUntilTime(t *testing.T) {
	now := time.Date(2026, 10, 29, 23, 0, 0, 0, time.Local)
	end, err := UntilTime("02:30", now)
	if err != nil || end.Day() != 30 || end.Sub(now) != 210*time.Minute {
		t.Fatal(end, err)
	}
	if _, err := UntilTime("25:00", now); err == nil {
		t.Fatal("invalid time accepted")
	}
	if end, err := UntilTime("23:30", now); err != nil || end.Sub(now) != 30*time.Minute {
		t.Fatalf("clock time later today = %v %v", end, err)
	}
}

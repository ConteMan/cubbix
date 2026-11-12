package devtools

import (
	"strings"
	"testing"
	"time"
	"uuid"
)

func TestGeneratedUUIDsCarryTheRightVersionAndVariant(t *testing.T) {
	for _, version := range []int{4, 7} {
		for _, presentation := range []struct {
			upper, plain bool
			check        func(string) bool
		}{
			{false, false, func(s string) bool { return strings.Count(s, "-") == 4 && s == strings.ToLower(s) }},
			{true, false, func(s string) bool { return strings.Count(s, "-") == 4 && s == strings.ToUpper(s) }},
			{false, true, func(s string) bool { return !strings.Contains(s, "-") && s == strings.ToLower(s) }},
		} {
			output := GenerateUUID(version, 1, presentation.upper, presentation.plain)
			if output.ErrorKey != "" {
				t.Fatal(output.ErrorKey)
			}
			id := uuid.MustParse(output.Rows[0].Payload)
			if got := int(id[6] >> 4); got != version {
				t.Fatalf("v%d version nibble = %d", version, got)
			}
			if got := id[8] & 0xC0; got != 0x80 {
				t.Fatalf("v%d variant = %#x", version, got)
			}
			if !presentation.check(output.Rows[0].Payload) {
				t.Fatalf("presentation of %q", output.Rows[0].Payload)
			}
		}
	}
}

func TestUUIDPairDefaultsToV4(t *testing.T) {
	output := GenerateUUIDPair()
	if len(output.Rows) != 2 || output.Payload != output.Rows[0].Payload {
		t.Fatalf("pair = %+v", output)
	}
	first := uuid.MustParse(output.Rows[0].Payload)
	second := uuid.MustParse(output.Rows[1].Payload)
	if first[6]>>4 != 4 || second[6]>>4 != 7 {
		t.Fatalf("pair versions = %d, %d", first[6]>>4, second[6]>>4)
	}
}

func TestUUIDBatchIsOneRow(t *testing.T) {
	output := GenerateUUID(4, 7, false, false)
	if len(output.Rows) != 1 || len(strings.Split(output.Payload, "\n")) != 7 {
		t.Fatalf("batch = %d rows, payload %q", len(output.Rows), output.Payload)
	}
	if output.Rows[0].HintKey != "DevtoolsCount" || output.Rows[0].HintArgs[0] != 7 {
		t.Fatalf("batch hint = %+v", output.Rows[0])
	}
	// 21-100 are copied from the panel in this step: still one row.
	if output := GenerateUUID(7, 100, false, false); len(strings.Split(output.Payload, "\n")) != 100 {
		t.Fatalf("100 values = %d lines", len(strings.Split(output.Payload, "\n")))
	}
	for _, count := range []int{0, 101} {
		if key := GenerateUUID(4, count, false, false).ErrorKey; key != "DevtoolsCountInvalid" {
			t.Fatalf("count %d = %q", count, key)
		}
	}
}

func TestUUIDParseShellAndVariants(t *testing.T) {
	canonical := "00000000-0000-7000-8000-000000000000"
	for _, text := range []string{canonical, strings.ToUpper(canonical), strings.ReplaceAll(canonical, "-", ""), "{" + canonical + "}", "urn:uuid:" + canonical, "URN:UUID:{" + canonical + "}"} {
		output, ok := ParseUUID(text)
		if !ok || output.Rows[0].Title != canonical {
			t.Fatalf("%s = %+v %v", text, output, ok)
		}
	}
	for _, text := range []string{"{urn:uuid:" + canonical + "}", "{{" + canonical + "}}", "urn:uuid:" + canonical + "0", "00000000-0000-7000-8000-00000000000"} {
		if _, ok := ParseUUID(text); ok {
			t.Fatalf("%s parsed", text)
		}
	}
	// The first 48 bits are 0, so a real RFC v7 shows 1970 as its time row.
	output, _ := ParseUUID(canonical)
	if len(output.Rows) != 2 || output.Rows[1].Payload != "0" {
		t.Fatalf("v7 time row = %+v", output.Rows)
	}
	// Variants other than RFC must never be read as a v7 time.
	for _, variantByte := range []string{"c000", "e000", "0000"} {
		text := "00000000-0000-7000-" + variantByte + "-000000000000"
		output, ok := ParseUUID(text)
		if !ok || len(output.Rows) != 1 {
			t.Fatalf("%s = %+v %v", text, output, ok)
		}
		if output.Rows[0].HintKey != "DevtoolsVersion" || output.Rows[0].HintArgs[0] != 7 {
			t.Fatalf("%s hint = %+v", text, output.Rows[0])
		}
	}
}

func TestUUIDNilAndMaxAreMarked(t *testing.T) {
	for _, test := range []struct{ text, key string }{
		{"00000000-0000-0000-0000-000000000000", "DevtoolsNil"},
		{"ffffffff-ffff-ffff-ffff-ffffffffffff", "DevtoolsMax"},
	} {
		output, ok := ParseUUID(test.text)
		if !ok || output.Rows[0].HintKey != test.key || len(output.Rows) != 1 {
			t.Fatalf("%s = %+v %v", test.text, output, ok)
		}
	}
}

func TestUUIDQueryCountRules(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	for _, text := range []string{"uuid", "uuid4", "uuidv4", "uuid 8", "uuid7", "uuidv7", "uuid7 08"} {
		if _, ok := Query(text, now, nil); !ok {
			t.Fatalf("%s not recognized", text)
		}
	}
	for _, text := range []string{"uuid 0", "uuid 101", "uuid -3", "uuid7 99999999999999999999"} {
		output, ok := Query(text, now, nil)
		if !ok || output.ErrorKey != "DevtoolsCountInvalid" {
			t.Fatalf("%s = %+v %v", text, output, ok)
		}
	}
	output, _ := Query("uuid7 8", now, nil)
	if len(output.Rows) != 1 || len(strings.Split(output.Payload, "\n")) != 8 {
		t.Fatalf("uuid7 8 = %+v", output)
	}
	for _, row := range strings.Split(output.Payload, "\n") {
		if id := uuid.MustParse(row); id[6]>>4 != 7 {
			t.Fatalf("uuid7 batch made version %d", id[6]>>4)
		}
	}
}

package devtools

import (
	"strconv"
	"strings"
	"time"
	"uuid"
)

// GenerateUUID makes one printable row per batch, or one row per value with
// GenerateUUIDPair. A batch of 2-100 is a single row whose payload joins all
// values with newlines: the panel copies the whole batch in one Enter.
func GenerateUUID(version, count int, upper, plain bool) Output {
	if count < 1 || count > 100 {
		return errorOutput("DevtoolsCountInvalid")
	}
	if version != 4 && version != 7 {
		return errorOutput("DevtoolsCountInvalid")
	}
	ids := make([]uuid.UUID, 0, count)
	values := make([]string, 0, count)
	for index := 0; index < count; index++ {
		id := uuid.NewV4()
		if version == 7 {
			id = uuid.NewV7()
		}
		ids = append(ids, id)
		values = append(values, formatUUID(id, upper, plain))
	}
	rows := []Row{{Kind: KindUUID, Title: values[0], Payload: values[0]}}
	if count > 1 {
		rows[0].HintKey = "DevtoolsCount"
		rows[0].HintArgs = []any{count}
		rows[0].Payload = strings.Join(values, "\n")
		rows[0].Title = values[0]
	} else {
		rows[0].HintKey, rows[0].HintArgs = uuidHint(ids[0])
	}
	if count == 1 && version == 7 {
		if timestamp, ok := uuidV7Time(ids[0]); ok {
			rows = append(rows, Row{Kind: KindTime, Title: timestamp.Format("2006-01-02 15:04:05.000 UTC"), Payload: strconv.FormatInt(timestamp.UnixMilli(), 10), HintKey: HintUnixMillis})
		}
	}
	return Output{Kind: KindUUID, Rows: rows, Payload: rows[0].Payload}
}

// GenerateUUIDPair is the bare uuid command: a v4 row first, then a v7 row.
// The v7 row gets no time line here; that is reserved for an explicit v7.
func GenerateUUIDPair() Output {
	v4 := GenerateUUID(4, 1, false, false)
	v7 := GenerateUUID(7, 1, false, false)
	rows := []Row{v4.Rows[0], v7.Rows[0]}
	return Output{Kind: KindUUID, Rows: rows, Payload: rows[0].Payload}
}

// ParseUUID reads a UUID literal. recognized is false when the text is not a
// UUID shape, so an ordinary search continues.
func ParseUUID(text string) (Output, bool) {
	core, ok := normalizeUUID(text)
	if !ok {
		return Output{}, false
	}
	id, err := uuid.Parse(core)
	if err != nil {
		return Output{}, false
	}
	canonical := formatUUID(id, false, false)
	hintKey, hintArgs := uuidHint(id)
	rows := []Row{{Kind: KindUUID, Title: canonical, Payload: canonical, HintKey: hintKey, HintArgs: hintArgs}}
	if isNilOrMax(id) {
		return Output{Kind: KindUUID, Rows: rows, Payload: canonical}, true
	}
	if variantName(id[8]) == "RFC 9562" && id[6]>>4 == 7 {
		if timestamp, v7 := uuidV7Time(id); v7 {
			rows = append(rows, Row{Kind: KindTime, Title: timestamp.Format("2006-01-02 15:04:05.000 UTC"), Payload: strconv.FormatInt(timestamp.UnixMilli(), 10), HintKey: HintUnixMillis})
		}
	}
	return Output{Kind: KindUUID, Rows: rows, Payload: canonical}, true
}

func isNilOrMax(id uuid.UUID) bool {
	return id == uuid.Nil() || id == uuid.Max()
}

// uuidHint classifies a value for the row's right side. Only a complete
// RFC 9562 variant carries a version; Nil and Max are marked by name.
func uuidHint(id uuid.UUID) (string, []any) {
	variant := variantName(id[8])
	switch {
	case id == uuid.Nil():
		return "DevtoolsNil", []any{variant}
	case id == uuid.Max():
		return "DevtoolsMax", []any{variant}
	}
	return "DevtoolsVersion", []any{int(id[6] >> 4), variant}
}

// variantName follows RFC 9562: the leading bits select NCS (0xxx), RFC
// 9562 (10xx), Microsoft (110x) or a reserved value (111x).
func variantName(b byte) string {
	switch {
	case b&0x80 == 0:
		return "NCS"
	case b&0xC0 == 0x80:
		return "RFC 9562"
	case b&0xE0 == 0xC0:
		return "Microsoft"
	}
	return "Reserved"
}

// uuidV7Time reads the first 48 bits as a Unix millisecond. A year outside
// 1-9999 is not shown as a time row.
func uuidV7Time(id uuid.UUID) (time.Time, bool) {
	millis := int64(id[0])<<40 | int64(id[1])<<32 | int64(id[2])<<24 | int64(id[3])<<16 | int64(id[4])<<8 | int64(id[5])
	timestamp := time.UnixMilli(millis).UTC()
	if timestamp.Year() < displayYearMin || timestamp.Year() > displayYearMax {
		return time.Time{}, false
	}
	return timestamp, true
}

// formatUUID applies the panel's temporary presentation: lowercase hyphenated
// by default, uppercase and/or without hyphens on request.
func formatUUID(id uuid.UUID, upper, plain bool) string {
	text := id.String()
	if upper {
		text = strings.ToUpper(text)
	}
	if plain {
		text = strings.ReplaceAll(text, "-", "")
	}
	return text
}

// normalizeUUID strips the accepted shell in a fixed order: an optional
// case-insensitive urn:uuid: prefix first, then at most one brace pair.
// Therefore urn:uuid:{...}, {urn:uuid:...} is not accepted, and a doubled
// shell is not accepted either. The core must then be 8-4-4-4-12 or 32 hex
// digits; Parse runs only after that check.
func normalizeUUID(text string) (string, bool) {
	text = strings.TrimSpace(text)
	const prefix = "urn:uuid:"
	if len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix) {
		text = text[len(prefix):]
	}
	if len(text) >= 2 && text[0] == '{' && text[len(text)-1] == '}' {
		text = text[1 : len(text)-1]
	}
	switch len(text) {
	case 36:
		for index := 0; index < len(text); index++ {
			if index == 8 || index == 13 || index == 18 || index == 23 {
				if text[index] != '-' {
					return "", false
				}
				continue
			}
			if !isHex(text[index]) {
				return "", false
			}
		}
		return text, true
	case 32:
		for index := 0; index < len(text); index++ {
			if !isHex(text[index]) {
				return "", false
			}
		}
		return text, true
	}
	return "", false
}

func isHex(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}

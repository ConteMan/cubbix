package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"uuid"

	"github.com/ConteMan/cubbix/internal/cli"
	"github.com/ConteMan/cubbix/internal/devtools"
)

// devtoolsPalette opens a palette without a native window and returns it with
// the clipboard read already settled.
func devtoolsPalette(t *testing.T) (*application, *paletteState) {
	t.Helper()
	a := awakeTestApp(t)
	a.createPaletteWindow = func(*paletteState) {}
	a.devtoolsAsync = false
	a.devtoolsOnMainThread = func(fn func()) { fn() }
	a.devtoolsPasteDelay = 0
	a.showPalette()
	if a.palette == nil {
		t.Fatal("palette did not open")
	}
	return a, a.palette
}

func refreshWithQuery(a *application, p *paletteState, query string) {
	p.query = query
	p.lastQuery = ""
	a.refreshPalette(p, "en")
}

func resultRowIndex(p *paletteState) int {
	for index, row := range p.rows {
		if row.Kind == paletteRowResult {
			return index
		}
	}
	return -1
}

func TestDevtoolsConfigDefaults(t *testing.T) {
	a := awakeTestApp(t)
	if !a.config.DevtoolsEnabled {
		t.Fatal("new config does not enable Devtools")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	// An older file without the key keeps the default on.
	if err := os.WriteFile(path, []byte("version = 1\nlanguage = \"zh-CN\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil || !loaded.DevtoolsEnabled {
		t.Fatalf("legacy config = %v, %v", loaded.DevtoolsEnabled, err)
	}
}

func TestPaletteDevtoolsTimestampAndDate(t *testing.T) {
	a, p := devtoolsPalette(t)

	refreshWithQuery(a, p, "1700000000")
	index := resultRowIndex(p)
	if index < 0 || p.selected != index {
		t.Fatalf("timestamp did not default to its result: rows=%v selected=%d", p.rows, p.selected)
	}
	if p.rows[index].Hint == "" {
		t.Fatal("timestamp result has no hint")
	}
	if strings.Contains(p.rows[index].Title, "Unix") {
		t.Fatalf("timestamp title is a hint: %q", p.rows[index].Title)
	}

	refreshWithQuery(a, p, "2026-11-12")
	index = resultRowIndex(p)
	if index < 0 || p.selected != index {
		t.Fatalf("date did not default to its result: %+v", p.rows)
	}
	if p.rows[index].Result.Payload == "" {
		t.Fatal("date result has no payload")
	}

	// A same-named ordinary command stays reachable below the result.
	refreshWithQuery(a, p, "settings")
	if resultRowIndex(p) >= 0 {
		t.Fatal("settings became a devtools result")
	}
}

func TestPaletteDevtoolsNowUUIDAndLiteral(t *testing.T) {
	a, p := devtoolsPalette(t)

	refreshWithQuery(a, p, "now")
	if resultRowIndex(p) < 0 || p.rows[p.selected].Kind != paletteRowResult {
		t.Fatalf("now did not produce a result: %+v", p.rows)
	}
	// The cached "now" must not change between frames of one query.
	first := p.rows[p.selected].Result.Payload
	a.refreshPalette(p, "en")
	if got := p.rows[p.selected].Result.Payload; got != first {
		t.Fatalf("now changed between frames: %q then %q", first, got)
	}

	refreshWithQuery(a, p, "uuid")
	if len(p.rows) < 3 || p.rows[0].Kind != paletteRowHeader || p.rows[1].Kind != paletteRowResult {
		t.Fatalf("uuid rows = %+v", p.rows)
	}
	if id := uuid.MustParse(p.rows[1].Result.Payload); id[6]>>4 != 4 {
		t.Fatalf("default uuid is version %d", id[6]>>4)
	}
	v7 := uuid.MustParse(p.rows[2].Result.Payload)
	if v7[6]>>4 != 7 {
		t.Fatalf("second uuid is version %d", v7[6]>>4)
	}

	refreshWithQuery(a, p, "uuid7 8")
	if lines := strings.Split(p.rows[1].Result.Payload, "\n"); len(lines) != 8 {
		t.Fatalf("uuid7 8 = %d values", len(lines))
	}
	for _, line := range strings.Split(p.rows[1].Result.Payload, "\n") {
		if id := uuid.MustParse(line); id[6]>>4 != 7 {
			t.Fatalf("uuid7 batch made version %d", id[6]>>4)
		}
	}

	literal := "00000000-0000-7000-8000-000000000000"
	if output, ok := devtools.ParseUUID(literal); !ok || len(output.Rows) != 2 {
		t.Fatalf("literal = %+v", output)
	}
	refreshWithQuery(a, p, literal)
	if p.rows[1].Result.Title != literal || len(p.rows) < 2 {
		t.Fatalf("literal rows = %+v", p.rows)
	}

	refreshWithQuery(a, p, "ts 946684800000")
	index := resultRowIndex(p)
	if index < 0 || p.rows[index].ErrorKey != "DevtoolsUnit" || p.selected != index {
		t.Fatalf("unit error = %+v", p.rows)
	}
}

func TestPaletteDevtoolsCopyAndFailure(t *testing.T) {
	a, p := devtoolsPalette(t)
	var copied []string
	a.devtoolsCopy = func(text string) error {
		copied = append(copied, text)
		return nil
	}
	refreshWithQuery(a, p, "uuid 4")
	want := p.rows[resultRowIndex(p)].Result.Payload
	a.runPalette()
	if len(copied) != 1 || copied[0] != want {
		t.Fatalf("copied %q, want %q", copied, want)
	}
	if strings.Contains(copied[0], "\n") == false || len(strings.Split(copied[0], "\n")) != 4 {
		t.Fatalf("batch did not copy every value: %q", copied[0])
	}
	if len(a.config.PaletteRecents) != 0 {
		t.Fatalf("a result entered recents: %v", a.config.PaletteRecents)
	}
	if p.visible {
		t.Fatal("the panel stayed open after a copy")
	}

	// The same Enter from the second handler must not copy again.
	a.runPalette()
	if len(copied) != 1 {
		t.Fatalf("Enter copied twice: %d", len(copied))
	}

	// A later query resets the guard.
	refreshWithQuery(a, p, "1700000000")
	p.visible = true
	a.devtoolsCopy = func(string) error { return errors.New("denied") }
	a.runPalette()
	if !strings.Contains(p.errorKey, "DevtoolsCopyFailed") {
		t.Fatalf("copy failure key = %q", p.errorKey)
	}
	if !p.visible {
		t.Fatal("the panel closed on a failed copy")
	}
}

func TestPaletteDevtoolsRejectsAClosedModule(t *testing.T) {
	a, p := devtoolsPalette(t)
	copied := 0
	a.devtoolsCopy = func(string) error { copied++; return nil }
	refreshWithQuery(a, p, "1700000000")
	a.config.DevtoolsEnabled = false
	a.runPalette()
	if copied != 0 {
		t.Fatal("a result row copied after the module closed")
	}
	a.refreshPalette(p, "en")
	if resultRowIndex(p) >= 0 {
		t.Fatal("a closed module still recognizes timestamps")
	}
}

func TestPaletteDevtoolsSessionChangeBlocksAResult(t *testing.T) {
	a, p := devtoolsPalette(t)
	copied := 0
	a.devtoolsCopy = func(string) error { copied++; return nil }
	refreshWithQuery(a, p, "uuid7 2")
	before := p.devSession
	a.changeDevtoolsEnabled(false)
	if p.devSession == before {
		t.Fatal("a module change did not advance the session")
	}
	// No frame has rebuilt the rows yet: the old result must not copy.
	a.runPalette()
	if copied != 0 {
		t.Fatal("a result from an older panel session copied")
	}

	// Reopening the panel advances the session and invalidates an old action page.
	a.changeDevtoolsEnabled(true)
	refreshWithQuery(a, p, "uuid7 2")
	a.togglePaletteActions()
	a.refreshPalette(p, "en")
	if p.mode != paletteActionMode || !p.actionsResult {
		t.Fatalf("action page not open: %v %v", p.mode, p.actionsResult)
	}
	a.showPalette()
	if p.actionsResultSession != 0 || p.resultSession != 0 {
		t.Fatalf("sessions not reset on reopen: %d %d", p.actionsResultSession, p.resultSession)
	}
}

func TestPaletteDevtoolsMatchingCommandStaysReachable(t *testing.T) {
	a, p := devtoolsPalette(t)
	doc := &jsonDocument{number: 42, custom: "now"}
	a.jsonDocuments = append(a.jsonDocuments, doc)
	refreshWithQuery(a, p, "now")

	resultIdx := -1
	cmdIdx := -1
	for idx, row := range p.rows {
		if row.Kind == paletteRowResult && resultIdx < 0 {
			resultIdx = idx
		}
		if row.Kind == paletteRowCommand && row.Command != nil && strings.Contains(row.Title, "now") {
			cmdIdx = idx
		}
	}
	if resultIdx < 0 || cmdIdx < 0 {
		t.Fatalf("expected both result and command in rows: resultIdx=%d cmdIdx=%d rows=%+v", resultIdx, cmdIdx, p.rows)
	}
	if p.selected != resultIdx {
		t.Fatalf("selected = %d, want result index %d", p.selected, resultIdx)
	}
	if cmdIdx <= resultIdx {
		t.Fatalf("command index %d is not below result index %d", cmdIdx, resultIdx)
	}
}

func TestPaletteDevtoolsErrorRowNeverCloses(t *testing.T) {
	a, p := devtoolsPalette(t)
	copied := 0
	a.devtoolsCopy = func(string) error { copied++; return nil }
	refreshWithQuery(a, p, "ts 946684800000")
	if p.rows[p.selected].ErrorKey != "DevtoolsUnit" {
		t.Fatalf("selected row = %+v", p.rows[p.selected])
	}
	a.runPalette()
	if copied != 0 || !p.visible {
		t.Fatalf("an error row copied or closed: %d %v", copied, p.visible)
	}
	if p.errorKey != "DevtoolsUnit" {
		t.Fatalf("error key = %q", p.errorKey)
	}
}

func TestPaletteDevtoolsPresentationKeepsTheBatch(t *testing.T) {
	a, p := devtoolsPalette(t)
	refreshWithQuery(a, p, "uuid7 3")
	before := strings.Split(p.rows[resultRowIndex(p)].Result.Payload, "\n")
	a.togglePaletteActions()
	a.refreshPalette(p, "en")
	if p.mode != paletteActionMode || !p.actionsResult {
		t.Fatalf("action page = %v %v", p.mode, p.actionsResult)
	}
	find := func(id string) int {
		for index, row := range p.rows {
			if row.Kind == paletteRowAction && row.Key == id {
				return index
			}
		}
		return -1
	}
	upper := find("devtools.upper")
	if upper < 0 {
		t.Fatalf("no uppercase action: %+v", p.rows)
	}
	p.selected = upper
	a.runPalette()
	if p.mode != paletteCommandMode || p.actionsResult {
		t.Fatalf("the action page did not return: %v", p.mode)
	}
	a.refreshPalette(p, "en")
	if p.selected != resultRowIndex(p) {
		t.Fatalf("selection did not return to the result row: %d", p.selected)
	}
	after := strings.Split(p.rows[resultRowIndex(p)].Result.Payload, "\n")
	if len(after) != 3 {
		t.Fatalf("batch size changed: %d", len(after))
	}
	for index := range before {
		if after[index] != strings.ToUpper(before[index]) {
			t.Fatalf("value %d changed instead of case: %q -> %q", index, before[index], after[index])
		}
	}

	// ⌘K opens actions, pressing ⌘K again toggles back and restores the original row.
	a.togglePaletteActions()
	a.refreshPalette(p, "en")
	if p.mode != paletteActionMode {
		t.Fatal("⌘K did not enter actions mode")
	}
	a.togglePaletteActions()
	a.refreshPalette(p, "en")
	if p.mode != paletteCommandMode || p.selected != resultRowIndex(p) {
		t.Fatalf("toggling ⌘K did not restore command mode and selection: %v %d", p.mode, p.selected)
	}

	// No hyphens acts on the same cached values.
	refreshWithQuery(a, p, "uuid 2")
	upperBatch := p.rows[resultRowIndex(p)].Result.Payload
	a.togglePaletteActions()
	a.refreshPalette(p, "en")
	if index := find("devtools.plain"); index >= 0 {
		p.selected = index
		a.runPalette()
	} else {
		t.Fatalf("no no-hyphens action: %+v", p.rows)
	}
	a.refreshPalette(p, "en")
	if strings.Contains(p.rows[resultRowIndex(p)].Result.Payload, "-") {
		t.Fatalf("no-hyphens retained a hyphen: %q", p.rows[resultRowIndex(p)].Result.Payload)
	}
	if strings.ToLower(strings.ReplaceAll(p.rows[resultRowIndex(p)].Result.Payload, "-", "")) != strings.ToLower(strings.ReplaceAll(upperBatch, "-", "")) {
		t.Fatalf("the batch was regenerated")
	}
}

func TestPaletteDevtoolsClipboardSuggestion(t *testing.T) {
	a := awakeTestApp(t)
	a.createPaletteWindow = func(*paletteState) {}
	reads := 0
	a.devtoolsReadClipboard = func() string { reads++; return "1700000000" }
	a.showPalette()
	p := a.palette
	if reads != 1 {
		t.Fatalf("clipboard reads = %d", reads)
	}
	a.refreshPalette(p, "en")
	if reads != 1 {
		t.Fatalf("the open panel read the clipboard again: %d", reads)
	}
	if len(p.rows) < 2 || p.rows[0].Group != "DevtoolsClipboardGroup" {
		t.Fatalf("suggestion group = %+v", p.rows)
	}
	if p.rows[p.selected].Kind != paletteRowCommand {
		t.Fatalf("the suggestion was selected by default: %+v", p.rows[p.selected])
	}
	// Moving into the suggestion is still possible.
	p.selectionKey = p.rows[1].Key
	a.refreshPalette(p, "en")
	if p.selected != 1 {
		t.Fatalf("suggestion not selectable: %d", p.selected)
	}
	// Typing removes the group.
	refreshWithQuery(a, p, "1700000000")
	if p.rows[0].Group == "DevtoolsClipboardGroup" {
		t.Fatal("the suggestion survived typing")
	}

	// 129 bytes, an ordinary sentence and a closed module never suggest.
	long := strings.Repeat("a", 129)
	a.devtoolsReadClipboard = func() string { reads++; return long }
	a.showPalette()
	a.refreshPalette(a.palette, "en")
	if len(a.palette.suggestion) != 0 {
		t.Fatal("a long clipboard text became a suggestion")
	}
	a.devtoolsReadClipboard = func() string { reads++; return "see 1700000000" }
	a.showPalette()
	a.refreshPalette(a.palette, "en")
	if len(a.palette.suggestion) != 0 {
		t.Fatal("a sentence became a suggestion")
	}
	reads = 0
	a.config.DevtoolsEnabled = false
	a.showPalette()
	if reads != 0 {
		t.Fatal("a closed module read the clipboard")
	}
}

func TestPaletteDevtoolsReopenInvalidatesCaches(t *testing.T) {
	a, p := devtoolsPalette(t)
	refreshWithQuery(a, p, "uuid")
	if p.resultCache == nil || len(p.resultCache) == 0 {
		t.Fatal("no cache")
	}
	a.showPalette()
	if p.resultCache != nil || p.suggestion != nil {
		t.Fatal("reopening kept a cached result or suggestion")
	}
	if p.resultSession != 0 || p.actionsResultSession != 0 {
		t.Fatalf("reopening kept resultSession=%d or actionsResultSession=%d", p.resultSession, p.actionsResultSession)
	}
	if p.done {
		t.Fatal("reopening kept the execution guard")
	}
}

func TestDevtoolsSettingsPageAndMappings(t *testing.T) {
	a := awakeTestApp(t)
	tk := tokens(false, "zh-CN")
	if got := devtoolsPageHeight(tk); got != 144 {
		t.Fatalf("page height = %v, want 144", got)
	}
	if got := pageHeight(tk, settingsDevtools, true, false, false, false, false); got != 144 {
		t.Fatalf("pageHeight = %v", got)
	}
	if got := settingsPageName(settingsDevtools); got != "devtools" {
		t.Fatalf("page name = %q", got)
	}
	if got, ok := settingsPageIndex("devtools", a); !ok || got != settingsDevtools {
		t.Fatalf("CLI page = %d %v", got, ok)
	}
	if request, ok := parsePaletteURL("cubbix://settings?page=devtools", true, true, 0); !ok || request.Page != settingsDevtools {
		t.Fatalf("URL page = %+v %v", request, ok)
	}
	if w, h := settingsMinimumSize(); w != 720 || h != 456 {
		t.Fatalf("minimum window = %dx%d", w, h)
	}
	if settingsNav[5].page != settingsDevtools || settingsNav[6].page != settingsShortcuts {
		t.Fatalf("nav order = %+v", settingsNav)
	}
}

// cliDevtoolsRequest is a `cubbix ts` or `cubbix uuid` request.
func cliDevtoolsRequest(action string) cli.Request {
	return cli.Request{V: cli.Protocol, ID: "dev1", Op: cli.OpRun, Action: action}
}

// TestCLIDevtoolsTimeAndUUID covers both actions end to end on the app side:
// the first line is the default result, a batch is one value per line, and a
// bad unit keeps its own code instead of becoming a date failure.
func TestCLIDevtoolsTimeAndUUID(t *testing.T) {
	a := cliTestApp(t)

	result := a.applyCLI(cliDevtoolsRequest(cli.ActionTS), nil)
	if !result.OK || len(result.Lines) != 4 || result.Message != result.Lines[0] {
		t.Fatalf("ts now = %+v", result)
	}
	result = a.applyCLI(cli.Request{V: cli.Protocol, ID: "dev2", Op: cli.OpRun, Action: cli.ActionTS, Arg: "2026-11-12"}, nil)
	if !result.OK || len(result.Lines) != 4 || result.State == nil || result.Bytes != 0 || result.Translate != nil {
		t.Fatalf("ts date = %+v", result)
	}
	result = a.applyCLI(cli.Request{V: cli.Protocol, ID: "dev3", Op: cli.OpRun, Action: cli.ActionTS, Arg: "ts 946684800000"}, nil)
	if result.OK || result.Code != "DevtoolsUnit" || result.Message == "" {
		t.Fatalf("ts ambiguous unit = %+v", result)
	}

	// The default UUID request is one lowercase hyphenated v4.
	result = a.applyCLI(cliDevtoolsRequest(cli.ActionUUID), nil)
	if !result.OK || len(result.Lines) != 1 || result.Message != result.Lines[0] {
		t.Fatalf("uuid default = %+v", result)
	}
	id := uuid.MustParse(result.Lines[0])
	if id[6]>>4 != 4 || result.Lines[0] != strings.ToLower(result.Lines[0]) || !strings.Contains(result.Lines[0], "-") {
		t.Fatalf("uuid default value = %q", result.Lines[0])
	}
	// A batch of v7 values, uppercase and without hyphens, one per line.
	result = a.applyCLI(cli.Request{V: cli.Protocol, ID: "dev4", Op: cli.OpRun, Action: cli.ActionUUID, Count: 3, UUIDVersion: 7, Upper: true, Plain: true}, nil)
	if !result.OK || len(result.Lines) != 3 || len(strings.Split(result.Message, "\n")) != 3 {
		t.Fatalf("uuid batch = %+v", result)
	}
	for _, line := range result.Lines {
		if len(line) != 32 || line != strings.ToUpper(line) {
			t.Fatalf("uuid batch value = %q", line)
		}
	}
	// A closed module answers disabled, as every other module does.
	a.config.DevtoolsEnabled = false
	if result := a.applyCLI(cliDevtoolsRequest(cli.ActionUUID), nil); result.Code != "disabled" {
		t.Fatalf("disabled answer = %+v", result)
	}
}

// TestCLIDevtoolsValidate pins the field whitelist of both actions: a field
// another action owns is a bad request, never a silent no-op.
func TestCLIDevtoolsValidate(t *testing.T) {
	base := func(action string) cli.Request {
		return cli.Request{V: cli.Protocol, ID: "dev1", Op: cli.OpRun, Action: action}
	}
	bad := []cli.Request{}
	request := base(cli.ActionTS)
	request.Bytes = 1
	bad = append(bad, request)
	request = base(cli.ActionTS)
	request.Arg = strings.Repeat("1", cli.MaxArg+1)
	bad = append(bad, request)
	request = base(cli.ActionTS)
	request.Notify = true
	bad = append(bad, request)
	request = base(cli.ActionTS)
	request.Count = 2
	bad = append(bad, request)
	request = base(cli.ActionUUID)
	request.Arg = "1700000000"
	bad = append(bad, request)
	request = base(cli.ActionUUID)
	request.Count = 101
	bad = append(bad, request)
	request = base(cli.ActionUUID)
	request.UUIDVersion = 5
	bad = append(bad, request)
	request = base(cli.ActionUUID)
	request.Notify = true
	bad = append(bad, request)
	request = base(cli.ActionSettings)
	request.Upper = true
	bad = append(bad, request)
	request = cli.Request{V: cli.Protocol, ID: "dev1", Op: cli.OpStatus, Action: cli.ActionTS}
	bad = append(bad, request)
	for _, request := range bad {
		if code := request.Validate(cli.MaxPayload); code != cli.CodeBadRequest {
			t.Fatalf("%+v passed validation as %q", request, code)
		}
	}
	good := []cli.Request{base(cli.ActionTS)}
	request = base(cli.ActionTS)
	request.Arg = "ts 1700000000"
	good = append(good, request)
	request = base(cli.ActionUUID)
	request.Count, request.UUIDVersion, request.Upper, request.Plain = 100, 7, true, true
	good = append(good, request)
	for _, request := range good {
		if code := request.Validate(cli.MaxPayload); code != "" {
			t.Fatalf("%+v was rejected as %q", request, code)
		}
	}
}

// TestCLIDevtoolsTwoHopsKeepLinesAndMessage drives the real socket handler: the
// app builds a Response from the Result and the client turns it back into a
// Result. Neither hop may drop lines or message.
func TestCLIDevtoolsTwoHopsKeepLinesAndMessage(t *testing.T) {
	a := cliTestApp(t)
	_, path := testServer(t, a)
	// One request per connection, as the tool always sends.
	result := do(t, dial(t, path), cli.Request{Op: cli.OpRun, Action: cli.ActionUUID, Count: 4, UUIDVersion: 7}, nil)
	if !result.OK || len(result.Lines) != 4 || result.Message != strings.Join(result.Lines, "\n") {
		t.Fatalf("two-hop uuid = %+v", result)
	}
	result = do(t, dial(t, path), cli.Request{Op: cli.OpRun, Action: cli.ActionTS, Arg: "1700000000"}, nil)
	if !result.OK || len(result.Lines) < 3 || result.Message != result.Lines[0] {
		t.Fatalf("two-hop ts = %+v", result)
	}
	if result.State == nil || result.Bytes != 0 || result.Translate != nil || result.QR != nil {
		t.Fatalf("the devtools answer carried a needless field: %+v", result)
	}
	// A full batch is valid at the action boundary, but its duplicated lines
	// and message exceed the one-line wire limit. The server must answer with
	// an explicit internal failure rather than a truncated partial batch.
	result = do(t, dial(t, path), cli.Request{Op: cli.OpRun, Action: cli.ActionUUID, Count: 100, UUIDVersion: 7}, nil)
	if result.OK || result.Code != cli.CodeInternal || len(result.Lines) != 0 {
		t.Fatalf("overlong batch = %+v", result)
	}
}

package main

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func commandTestApp(t *testing.T) *application {
	t.Helper()
	a, err := newApplication(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.updates.close() })
	return a
}

func TestParseAwakeArg(t *testing.T) {
	now := time.Date(2026, 10, 30, 14, 0, 0, 0, time.Local)
	cases := []struct {
		arg     string
		kind    awakeArgKind
		minutes int
		until   string
		errKey  string
	}{
		{"", awakeArgDefault, 0, "", ""},
		{"∞", awakeArgUnlimited, 0, "", ""},
		{"inf", awakeArgUnlimited, 0, "", ""},
		{"infinity", awakeArgUnlimited, 0, "", ""},
		{"unlimited", awakeArgUnlimited, 0, "", ""},
		{"无限", awakeArgUnlimited, 0, "", ""},
		{"不限", awakeArgUnlimited, 0, "", ""},
		{"不限时", awakeArgUnlimited, 0, "", ""},
		{"0", awakeArgUnlimited, 0, "", ""},
		{"45", awakeArgMinutes, 45, "", ""},
		{"45m", awakeArgMinutes, 45, "", ""},
		{"45min", awakeArgMinutes, 45, "", ""},
		{"45分钟", awakeArgMinutes, 45, "", ""},
		{"45 分", awakeArgMinutes, 45, "", ""},
		{"2h", awakeArgMinutes, 120, "", ""},
		{"2小时", awakeArgMinutes, 120, "", ""},
		{"2 hour", awakeArgMinutes, 120, "", ""},
		{"2hours", awakeArgMinutes, 120, "", ""},
		{"1h30m", awakeArgMinutes, 90, "", ""},
		{"1h30", awakeArgMinutes, 90, "", ""},
		{"1h 30m", awakeArgMinutes, 90, "", ""},
		{"1小时30分钟", awakeArgMinutes, 90, "", ""},
		{"until 18:30", awakeArgUntil, 0, "18:30", ""},
		{"到 18:30", awakeArgUntil, 0, "18:30", ""},
		{"直到 18:30", awakeArgUntil, 0, "18:30", ""},
		{"18:30", awakeArgUntil, 0, "18:30", ""},
		{"1", awakeArgMinutes, 1, "", ""},
		{"525659", awakeArgMinutes, 525659, "", ""},
		{"8760h59m", awakeArgMinutes, 525659, "", ""},
		{"525660", awakeArgDefault, 0, "", "DurationError"},
		{"8761h", awakeArgDefault, 0, "", "DurationError"},
		{"1h60m", awakeArgDefault, 0, "", "DurationError"},
		{"45x", awakeArgDefault, 0, "", "DurationError"},
		{"abc", awakeArgDefault, 0, "", "DurationError"},
		{"1h30mx", awakeArgDefault, 0, "", "DurationError"},
		{"0m", awakeArgDefault, 0, "", "DurationError"},
		{"-1", awakeArgDefault, 0, "", "DurationError"},
		{"1.5h", awakeArgDefault, 0, "", "DurationError"},
		{"until 99:99", awakeArgUntil, 0, "", "TimeError"},
		{"until", awakeArgUntil, 0, "", "TimeError"},
	}
	for _, scenario := range cases {
		got := parseAwakeArg(scenario.arg, now)
		if got.Kind != scenario.kind || got.Minutes != scenario.minutes || got.Until != scenario.until || got.Error != scenario.errKey {
			t.Fatalf("parseAwakeArg(%q) = %+v; want kind=%v minutes=%d until=%q error=%q", scenario.arg, got, scenario.kind, scenario.minutes, scenario.until, scenario.errKey)
		}
	}
}

func TestParseAwakeArgCombinedBeforeSingleUnit(t *testing.T) {
	arg := parseAwakeArg("1h30m", time.Now())
	if arg.Kind != awakeArgMinutes || arg.Minutes != 90 {
		t.Fatalf("combined duration = %+v", arg)
	}
	if single := parseAwakeArg("1h", time.Now()); single.Minutes != 60 {
		t.Fatalf("single hour = %+v", single)
	}
	if single := parseAwakeArg("30m", time.Now()); single.Minutes != 30 {
		t.Fatalf("single minute = %+v", single)
	}
}

func TestMatchScoreTiers(t *testing.T) {
	if score := matchScore("开启常亮", "开启常亮", nil); score != 1000 {
		t.Fatalf("exact title = %d", score)
	}
	if score := matchScore("开", "开启常亮", nil); score != 900-runeLen("开启常亮") {
		t.Fatalf("title prefix = %d", score)
	}
	if score := matchScore("json", "新建 JSON 窗口", []string{"json"}); score != 800 {
		t.Fatalf("keyword exact = %d", score)
	}
	if score := matchScore("js", "新建 JSON 窗口", []string{"json"}); score != 700-runeLen("json") {
		t.Fatalf("keyword prefix = %d", score)
	}
	if score := matchScore("窗口", "新建 JSON 窗口", nil); score != 600-runeIndex("新建 JSON 窗口", "窗口") {
		t.Fatalf("title contains = %d", score)
	}
	if score := matchScore("窗口", "设置", []string{"命令窗口"}); score != 500-runeIndex("命令窗口", "窗口") {
		t.Fatalf("keyword contains = %d", score)
	}
	if score := matchScore("jsonw", "New JSON window", nil); score <= 0 {
		t.Fatalf("ASCII subsequence = %d", score)
	}
	if score := matchScore("cl", "开启常亮", []string{"常亮", "cl"}); score != 800 {
		t.Fatalf("pinyin initials = %d", score)
	}
	if score := matchScore("cl", "常亮", nil); score != 0 {
		t.Fatalf("CJK should not match as a subsequence: %d", score)
	}
	if score := matchScore("json items.json", "New JSON window", []string{"json", "items.json"}); score != 800+800 {
		t.Fatalf("multi word = %d", score)
	}
	if score := matchScore("json items", "New JSON window", []string{"json"}); score != 0 {
		t.Fatalf("multi word requires every word: %d", score)
	}
	if score := matchScore("", "anything", nil); score != 0 {
		t.Fatalf("empty query = %d", score)
	}
}

func TestCommandPrefixSplitting(t *testing.T) {
	a := commandTestApp(t)
	commands := a.commands()
	cases := []struct {
		query   string
		wantID  string
		wantArg string
		matched bool
	}{
		{"常亮 1h", "awake.start", "1h", true},
		{"常亮1h", "awake.start", "1h", true},
		{"常亮", "awake.start", "", true},
		{"常亮∞", "awake.start", "∞", true},
		{"常亮 until 18:30", "awake.start", "until 18:30", true},
		{"开启常亮 30m", "awake.start", "30m", true},
		{"json items.json", "json.new", "items.json", true},
		{"json", "json.new", "", true},
		{"常亮开关", "", "", false},
		{"", "", "", false},
	}
	for _, scenario := range cases {
		prefix, ok := splitCommandQuery(a, "zh-CN", scenario.query, commands)
		if ok != scenario.matched || prefix.Arg != scenario.wantArg {
			t.Fatalf("split(%q) = %+v ok=%v; want id=%q arg=%q matched=%v", scenario.query, prefix, ok, scenario.wantID, scenario.wantArg, scenario.matched)
		}
		if ok && prefix.Command.ID != scenario.wantID {
			t.Fatalf("split(%q) command = %q; want %q", scenario.query, prefix.Command.ID, scenario.wantID)
		}
	}
}

func TestDefaultCommandPrefersRegistrationOrder(t *testing.T) {
	a := commandTestApp(t)
	a.createJSONTestDocuments(2)
	commands := a.commands()
	best, prefix := bestCommand(a, "zh-CN", "json", commands)
	if best == nil || best.ID != "json.new" || !prefix.Shared {
		t.Fatalf("json default = %+v prefix=%+v", best, prefix)
	}
	best, _ = bestCommand(a, "zh-CN", "常亮", commands)
	if best == nil || best.ID != "awake.start" {
		t.Fatalf("awake default = %+v", best)
	}
	best, _ = bestCommand(a, "zh-CN", "JSON", commands)
	if best == nil || best.ID != "json.new" {
		t.Fatalf("uppercase json default = %+v", best)
	}
	best, _ = bestCommand(a, "zh-CN", "常亮 45m", commands)
	if best == nil || best.ID != "awake.start" {
		t.Fatalf("parameter default = %+v", best)
	}
	if forced, prefix := bestCommand(a, "zh-CN", "常亮 45x", commands); forced == nil || forced.ID != "awake.start" || prefix.Arg != "45x" {
		t.Fatalf("invalid parameter default = %+v prefix=%+v", forced, prefix)
	}
}

// createJSONTestDocuments adds documents without windows, which is enough for
// the registry: names and numbers come from the document itself.
func (a *application) createJSONTestDocuments(count int) {
	for index := 0; index < count; index++ {
		a.addJSONDocument("", "", true)
	}
}

func TestCommandAvailability(t *testing.T) {
	a := commandTestApp(t)
	ids := func() map[string]bool {
		out := map[string]bool{}
		for _, command := range a.commands() {
			out[command.ID] = true
		}
		return out
	}
	if ids()["awake.stop"] {
		t.Fatal("stopped keep awake offered a stop command")
	}
	a.config.AwakeEnabled = false
	present := ids()
	for _, id := range []string{"awake.start", "awake.mode.screen", "awake.until", "awake.custom", "awake.follow"} {
		if present[id] {
			t.Fatalf("disabled keep awake still lists %s", id)
		}
	}
	a.config.AwakeEnabled = true
	a.config.JSONEnabled = false
	if ids()["json.new"] {
		t.Fatal("disabled JSON still lists json.new")
	}
	a.config.JSONEnabled = true
	a.updates.state = updateState{Phase: updateReady}
	if ids()["update.check"] || !ids()["update.restart"] {
		t.Fatal("update availability is wrong in ready state")
	}
	a.updates.state = updateState{Phase: updateChecking}
	if ids()["update.restart"] || ids()["update.check"] {
		t.Fatal("update availability is wrong while checking")
	}
}

func TestPaletteGroupingDedupesAndHidesWindows(t *testing.T) {
	a := commandTestApp(t)
	a.config.PaletteFavorites = []string{"settings.page.shortcuts", "app.quit", "missing.command"}
	a.config.PaletteRecents = []string{"app.quit", "update.check", "settings.page.shortcuts", "update.check"}
	a.addJSONDocument("", "", true)
	windowID := "json.window.1"
	a.config.PaletteFavorites = append(a.config.PaletteFavorites, windowID)
	a.config.PaletteRecents = append(a.config.PaletteRecents, windowID)
	a.config.PaletteFavorites = normalizePaletteFavorites(a.config.PaletteFavorites)
	a.config.PaletteRecents = normalizePaletteRecents(a.config.PaletteRecents)

	p := &paletteState{}
	a.refreshPalette(p, "zh-CN")
	groups := map[string][]string{}
	current := ""
	for _, row := range p.rows {
		if row.Kind == paletteRowHeader {
			current = row.Group
			if _, ok := groups[current]; !ok {
				groups[current] = nil
			}
			continue
		}
		if row.Kind == paletteRowCommand {
			groups[current] = append(groups[current], row.Command.ID)
		}
	}
	if want := []string{"settings.page.shortcuts", "app.quit"}; !reflect.DeepEqual(groups["PaletteFavorites"], want) {
		t.Fatalf("favorites = %v; want %v", groups["PaletteFavorites"], want)
	}
	if want := []string{"update.check"}; !reflect.DeepEqual(groups["PaletteRecent"], want) {
		t.Fatalf("recents = %v; want %v", groups["PaletteRecent"], want)
	}
	for _, id := range groups["PaletteAll"] {
		if id == "settings.page.shortcuts" || id == "app.quit" || id == "update.check" {
			t.Fatalf("duplicate command %s in the all group", id)
		}
	}
	seenWindow := false
	for _, id := range groups["PaletteAll"] {
		if id == windowID {
			seenWindow = true
		}
	}
	if !seenWindow {
		t.Fatal("JSON window missing from the all group")
	}
	for _, id := range append(groups["PaletteFavorites"], groups["PaletteRecent"]...) {
		if id == windowID {
			t.Fatal("JSON window entered favorites or recents")
		}
	}
	// A query keeps groups but hides the ones with no hits.
	p.query = "快捷键"
	a.refreshPalette(p, "zh-CN")
	found := false
	for _, row := range p.rows {
		if row.Kind == paletteRowHeader && row.Group == "PaletteFavorites" {
			found = true
		}
	}
	if !found {
		t.Fatal("favorites group should keep its matching row")
	}
	p.query = "jsonw"
	a.refreshPalette(p, "zh-CN")
	for _, row := range p.rows {
		if row.Kind == paletteRowHeader && (row.Group == "PaletteFavorites" || row.Group == "PaletteRecent") {
			t.Fatal("empty groups should hide their headers")
		}
	}
}

func TestAwakeStartActionsCoverInputAndOverrides(t *testing.T) {
	a := awakeTestApp(t)
	arg := awakeArg{Kind: awakeArgMinutes, Minutes: 45}
	req := request{Command: a.awakeCommands()[0], Arg: "45m", Awake: arg}
	actions := awakeStartActions(a, req)
	byID := map[string]paletteAction{}
	for _, action := range actions {
		byID[action.ID] = action
	}
	for _, id := range []string{"awake.start.screen", "awake.start.tasks", "awake.start.30m", "awake.start.60m", "awake.start.unlimited", "awake.start.until", "awake.start.custom", "awake.start.follow", "palette.favorite"} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing action %s", id)
		}
	}
	if byID["awake.start.30m"].Checked(a) || byID["awake.start.unlimited"].Checked(a) {
		t.Fatal("unrelated duration actions must not be checked for 45m")
	}
	if !byID["awake.start.screen"].Checked(a) || byID["awake.start.tasks"].Checked(a) {
		t.Fatal("mode checked state does not follow the configuration")
	}
	if err := byID["awake.start.tasks"].Run(a); err != nil {
		t.Fatal(err)
	}
	state := a.awake.snapshot()
	if !state.Active || state.Session.Mode != "tasks" || a.config.AwakeMinutes != 45 || a.config.AwakeMode != "tasks" {
		t.Fatalf("action did not keep the input duration: %+v %+v", state, a.config)
	}
	// A duration action overrides the input.
	if err := byID["awake.start.60m"].Run(a); err != nil {
		t.Fatal(err)
	}
	if state := a.awake.snapshot(); state.Session.End.Sub(time.Now()) < 59*time.Minute {
		t.Fatalf("duration action = %+v", state)
	}
}

func TestCommandTitlesUseLocalizedText(t *testing.T) {
	a := commandTestApp(t)
	for _, command := range a.commands() {
		for _, locale := range []string{"zh-CN", "en"} {
			title := command.Title(a, locale)
			if title == "" || title == command.ID {
				t.Fatalf("%s title in %s = %q", command.ID, locale, title)
			}
		}
	}
}

func TestSettingsHeightShortcutsPageIsFiveRows(t *testing.T) {
	tokens := tokens(false, "zh-CN")
	if got := settingsHeight(tokens, 3, false); got != 292 {
		t.Fatalf("shortcuts page height = %d; want 292", got)
	}
	if _, minHeight := settingsMinimumSize(); minHeight != 340 {
		t.Fatalf("minimum height = %d", minHeight)
	}
}

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ConteMan/cubbix/internal/cli"
	"github.com/egoist/mygo/ui"
)

// --- Settings rows and layout ---

func TestSettingsRowLevels(t *testing.T) {
	a := awakeTestApp(t)
	rows := func(c *ui.Context) {
		_, t := applyTheme(c)
		a.settingsRow(c, t, "plain", settingsRowOpts{}, func() { ui.Box(c).Size(28, 20) })
		ui.Divider(c)
		a.settingsRow(c, t, "inline", settingsRowOpts{level: 1, hint: "one short line"}, func() { ui.Box(c).Size(34, 20) })
		ui.Divider(c)
		a.settingsRow(c, t, "twoline", settingsRowOpts{level: 2, icon: "gauge", hint: "a rule that needs its own line"}, func() { ui.Box(c).Size(318, 28) })
		ui.Divider(c)
		a.settingsRow(c, t, "explained", settingsRowOpts{level: 3, tooltip: "the reason this row exists"}, func() { ui.Box(c).Size(34, 20) })
	}
	tt := ui.NewTester(rows, 528, 400)
	tt.SetLocale("en")
	tt.Frame()
	tk := tokens(false, "en")
	if got := rowHeight(tk, 2); got != 64 || got != tk.SizeRowHeight+tk.Space16 {
		t.Fatalf("level 2 height = %v", got)
	}
	for _, level := range []int{0, 1, 3} {
		if got := rowHeight(tk, level); got != 48 {
			t.Fatalf("level %d height = %v", level, got)
		}
	}
	// The title and its inline hint share one bottom edge, one step apart in
	// size, so the hint never looks vertically centred.
	title, ok := tt.Find("inline")
	hint, hintOK := tt.Find("one short line")
	if !ok || !hintOK {
		t.Fatalf("missing the level-1 row: %v", tt.Texts())
	}
	if hint.Y >= title.Y {
		t.Fatalf("the hint does not start above the baseline: %v %v", title, hint)
	}
	if bottom, titleBottom := hint.Y+hint.H, title.Y+title.H; bottom < titleBottom-0.5 || bottom > titleBottom+0.5 {
		t.Fatalf("the inline hint is not bottom aligned: %v %v", title, hint)
	}
	// The level-2 second line sits below its title, on its own line.
	twoLine, _ := tt.Find("twoline")
	rule, _ := tt.Find("a rule that needs its own line")
	if rule.Y <= twoLine.Y {
		t.Fatalf("the level-2 hint is not a second line: %v %v", twoLine, rule)
	}
	// The level-3 explanation is a focusable anchor carrying the whole text.
	info, infoOK := tt.Find("the reason this row exists")
	if !infoOK || info.W > 16.5 || info.H > 16.5 {
		t.Fatalf("the level-3 info anchor = %v %v", info, infoOK)
	}
	// A disabled row keeps its explanation readable: the info icon is not
	// inside the disabled group.
	disabled := func(c *ui.Context) {
		_, tk := applyTheme(c)
		a.settingsRow(c, tk, "released", settingsRowOpts{level: 3, tooltip: "released because the module is off", disabled: true}, func() {})
	}
	tt = ui.NewTester(disabled, 528, 80)
	tt.SetLocale("en")
	tt.Frame()
	if _, ok := tt.Find("released because the module is off"); !ok {
		t.Fatalf("the disabled row lost its explanation: %v", tt.Texts())
	}
}

// hasAccessibleText reports whether some element carries s as its accessible
// name, which is how a focusable icon exposes its tooltip text.
func hasAccessibleText(tt *ui.Tester, s string) bool {
	for _, text := range tt.Texts() {
		if text == s {
			return true
		}
	}
	return false
}

func TestSettingsPagesRenderEveryState(t *testing.T) {
	a := awakeTestApp(t)
	_ = evidencePaths(t)
	for _, page := range []int{settingsGeneral, settingsAwake, settingsJSON, settingsTranslate, settingsShortcuts, settingsAbout} {
		for _, awakeOn := range []bool{false, true} {
			a.page = page
			a.config.AwakeEnabled = awakeOn
			a.config.JSONEnabled = awakeOn
			height := settingsHeight(tokens(false, "en"), page, awakeOn, false, false, true, true)
			if height != settingsWindowHeight {
				t.Fatalf("page %d: window height %d", page, height)
			}
			tt := ui.NewTester(a.view, 720, height)
			tt.SetLocale("en")
			// Every page draws the brand and its own page content.
			if _, ok := tt.Find(a.text("en", "Cubbix")); !ok {
				t.Fatalf("page %d: missing the brand: %v", page, tt.Texts())
			}
		}
	}
}

// --- Module switch and shortcut ownership ---

func TestModuleCloseReleasesOnlyWhatItOwns(t *testing.T) {
	a := awakeTestApp(t)
	registered := map[string]string{} // combination -> owner
	a.registerShortcut = func(acc string, _ func()) error {
		if owner, taken := registered[acc]; taken && owner != "" {
			return errors.New("already registered")
		}
		registered[acc] = "pending"
		return nil
	}
	a.unregisterShortcut = func(acc string) { delete(registered, acc) }
	own := func(target string) {
		// The registry does not know slot names; the app's map does.
		if acc := a.ownedShortcut(target); acc != "" {
			registered[acc] = target
		}
	}
	a.registerAwakeShortcut()
	own("awake")
	awakeAcc := a.config.AwakeShortcut
	if registered[awakeAcc] != "awake" {
		t.Fatalf("keep awake did not register: %v", registered)
	}

	// Closing keep awake releases its own combination.
	a.changeAwakeEnabled(false)
	if _, taken := registered[awakeAcc]; taken {
		t.Fatalf("the close kept its own shortcut: %v", registered)
	}

	// JSON records the combination keep awake used to hold.
	a.config.JSONShortcut = awakeAcc
	a.registerJSONShortcut()
	own("json")
	if registered[awakeAcc] != "json" {
		t.Fatalf("JSON did not take the free combination: %v", registered)
	}

	// Reopening keep awake conflicts and owns nothing.
	a.changeAwakeEnabled(true)
	if a.ownedShortcut("awake") != "" || a.shortcutError != "JSONShortcutConflict" {
		t.Fatalf("reopen = owned %q error %q", a.ownedShortcut("awake"), a.shortcutError)
	}

	// Closing it again must not release the combination JSON now holds.
	a.changeAwakeEnabled(false)
	if registered[awakeAcc] != "json" {
		t.Fatalf("the second close released JSON's shortcut: %v", registered)
	}
	if a.ownedShortcut("json") != awakeAcc {
		t.Fatal("JSON lost ownership")
	}
}

func TestFailedRegistrationTakesNoOwnership(t *testing.T) {
	a := awakeTestApp(t)
	var registered string
	a.registerShortcut = func(acc string, _ func()) error {
		registered = acc
		return errors.New("occupied")
	}
	a.registerAwakeShortcut()
	if a.ownedShortcut("awake") != "" {
		t.Fatal("a refused registration took ownership")
	}
	if a.shortcutError != "ShortcutUnavailable" {
		t.Fatalf("error = %q", a.shortcutError)
	}
	// A close with nothing owned unregisters nothing at all.
	released := 0
	a.unregisterShortcut = func(string) { released++ }
	a.changeAwakeEnabled(false)
	if released != 0 {
		t.Fatalf("close released %d combinations it never owned", released)
	}
	_ = registered
}

func TestRecordingRefusedAndDroppedWhenModuleCloses(t *testing.T) {
	a := awakeTestApp(t)
	var registered []string
	a.registerShortcut = func(acc string, _ func()) error { registered = append(registered, acc); return nil }
	a.unregisterShortcut = func(string) {}

	// A closed module has nothing to record.
	a.config.AwakeEnabled = false
	a.beginTileShortcutRecording("awake")
	if a.shortcutRecording {
		t.Fatal("a closed module opened its recorder")
	}

	// Closing the module while its recorder listens drops the draft.
	a.config.AwakeEnabled = true
	a.registerAwakeShortcut()
	a.beginTileShortcutRecording("awake")
	a.shortcutDraft = "Ctrl+Alt+Cmd+Z"
	a.changeAwakeEnabled(false)
	if a.shortcutRecording || a.shortcutDraft != "" || a.shortcutTarget != "" {
		t.Fatalf("recording survived the module close: %v %q %q", a.shortcutRecording, a.shortcutDraft, a.shortcutTarget)
	}
	if a.config.AwakeShortcut == "Ctrl+Alt+Cmd+Z" {
		t.Fatal("the draft was saved")
	}
	if a.shortcutSuspended {
		t.Fatal("the suspended flag survived")
	}

	// Reopening the module registers its remembered value again.
	a.changeAwakeEnabled(true)
	if a.ownedShortcut("awake") != a.config.AwakeShortcut {
		t.Fatalf("reopen did not register the module: %v", registered)
	}
}

func TestPageSwitchAndHideCancelRecording(t *testing.T) {
	a := awakeTestApp(t)
	a.registerShortcut = func(string, func()) error { return nil }
	a.unregisterShortcut = func(string) {}
	a.beginTileShortcutRecording("awake")
	a.selectSettingsPage(settingsShortcuts)
	if a.shortcutRecording || a.shortcutTarget != "" || a.shortcutSuspended {
		t.Fatal("a page switch kept the recorder")
	}
	a.beginTileShortcutRecording("awake")
	a.hide()
	if a.shortcutRecording || a.shortcutTarget != "" {
		t.Fatal("hiding the window kept the recorder")
	}
}

// --- Safe module close ---

func TestAwakeCloseRestoresLidBeforeCommitting(t *testing.T) {
	a, sleep, _, svc, _ := lidTestApp(t)
	session := a.selectedSession()
	session.Minutes, session.End, session.Lid = 60, time.Now().Add(time.Hour), true
	if err := a.startAwake(session); err != nil {
		t.Fatal(err)
	}
	if sleep.value != 1 || !a.config.AwakeEnabled {
		t.Fatalf("session did not start: sleep=%d enabled=%v", sleep.value, a.config.AwakeEnabled)
	}
	a.changeAwakeEnabled(false)
	if a.config.AwakeEnabled {
		t.Fatal("the module stayed open after a safe close")
	}
	if sleep.value != 0 || a.lid.markerExists() {
		t.Fatalf("the system sleep setting was not restored: value=%d marker=%v", sleep.value, a.lid.markerExists())
	}
	if a.awake.snapshot().Active {
		t.Fatal("keep awake kept running")
	}
	if svc.unregisters != 1 {
		t.Fatalf("helper unregisters = %d", svc.unregisters)
	}
	if a.config.AwakeSession != nil {
		t.Fatal("a stopped session was remembered")
	}
	if a.ownedShortcut("awake") != "" {
		t.Fatal("the module close kept its shortcut")
	}
}

func TestAwakeCloseFailureKeepsModuleOpen(t *testing.T) {
	a, sleep, _, svc, _ := lidTestApp(t)
	session := a.selectedSession()
	session.Minutes, session.End, session.Lid = 60, time.Now().Add(time.Hour), true
	if err := a.startAwake(session); err != nil {
		t.Fatal(err)
	}
	// The helper cannot restore the sleep setting: nothing may follow.
	sleep.failZero = true
	a.changeAwakeEnabled(false)
	if !a.config.AwakeEnabled || a.moduleError != "LidRestoreFailed" {
		t.Fatalf("unsafe close committed: enabled=%v error=%q", a.config.AwakeEnabled, a.moduleError)
	}
	if svc.unregisters != 0 {
		t.Fatal("the helper was uninstalled before the restore was safe")
	}
	if !a.awake.snapshot().Active {
		t.Fatal("keep awake stopped before the lid was restored")
	}
	if a.lid.snapshot().state != lidRestoreFailed || !a.lid.canForce() {
		t.Fatal("the recovery entry was lost")
	}
}

func TestAwakeCloseRefusesWhileLidIsBusy(t *testing.T) {
	a, sleep, _, svc, _ := lidTestApp(t)
	a.lidBusy = true
	a.changeAwakeEnabled(false)
	if !a.config.AwakeEnabled || a.moduleError != "ModuleBusy" {
		t.Fatalf("busy close = enabled %v error %q", a.config.AwakeEnabled, a.moduleError)
	}
	if sleep.writes != nil || svc.unregisters != 0 {
		t.Fatal("a busy close touched the system")
	}
}

// stuckChild reports its exit only when the test explicitly releases it.
type stuckChild struct{ done chan error }

func (f stuckChild) Wait() error { return <-f.done }
func (stuckChild) Kill() error   { return nil }

func TestAwakeCloseReportsAnUnconfirmedStop(t *testing.T) {
	a := awakeTestApp(t)
	defer func(previous time.Duration) { moduleStopTimeout = previous }(moduleStopTimeout)
	moduleStopTimeout = 50 * time.Millisecond
	child := stuckChild{done: make(chan error)}
	t.Cleanup(func() { close(child.done) })
	a.awake.launch = func([]string) (awakeChild, error) { return child, nil }
	if err := a.startAwake(awakeSession{Mode: "screen", End: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	a.changeAwakeEnabled(false)
	if !a.config.AwakeEnabled || a.moduleError != "AwakeStopError" {
		t.Fatalf("unconfirmed stop = enabled %v error %q", a.config.AwakeEnabled, a.moduleError)
	}
	if !a.awake.snapshot().Active || a.config.AwakeSession == nil {
		t.Fatal("an unconfirmed child was reported as stopped")
	}
	// Retrying before the child exits must not turn the previous timeout
	// into a successful close simply because the controller lost its handle.
	a.changeAwakeEnabled(false)
	if !a.config.AwakeEnabled || a.moduleError != "AwakeStopError" {
		t.Fatal("a retry forgot the still unconfirmed child")
	}
}

func TestAwakeCloseGateBlocksNewWork(t *testing.T) {
	a := awakeTestApp(t)
	a.moduleClosing = true
	if err := a.toggleAwakeResult(); err == nil || err.Error() != "ModuleBusy" {
		t.Fatalf("toggle during a close = %v", err)
	}
	if err := a.startAwake(awakeSession{Mode: "screen", End: time.Now().Add(time.Hour)}); err == nil || err.Error() != "ModuleBusy" {
		t.Fatalf("start during a close = %v", err)
	}
	if err := a.runURL(paletteURLRequest{Kind: "awake"}); err == nil || err.Error() != "ModuleBusy" {
		t.Fatalf("URL during a close = %v", err)
	}
	if a.awake.snapshot().Active {
		t.Fatal("a refused request started keep awake")
	}
}

// --- Command line and URLs ---

func TestSettingsPageNamesAreStable(t *testing.T) {
	a := awakeTestApp(t)
	pages := map[string]int{
		"general": settingsGeneral, "awake": settingsAwake, "tiles": settingsAwake,
		"json": settingsJSON, "shortcuts": settingsShortcuts, "about": settingsAbout,
	}
	for name, want := range pages {
		if got, ok := settingsPageIndex(name, a); !ok || got != want {
			t.Fatalf("%s -> %d %v, want %d", name, got, ok, want)
		}
	}
	if _, ok := settingsPageIndex("bogus", a); ok {
		t.Fatal("an unknown page was accepted")
	}
}

func TestSettingsURLNamesAndAlias(t *testing.T) {
	for _, test := range []struct {
		url  string
		page int
	}{
		{"cubbix://settings?page=awake", settingsAwake},
		{"cubbix://settings?page=tiles", settingsAwake},
		{"cubbix://settings?page=json", settingsJSON},
		{"cubbix://settings?page=shortcuts", settingsShortcuts},
		{"cubbix://settings?page=about", settingsAbout},
		{"cubbix://settings?page=general", settingsGeneral},
	} {
		request, ok := parsePaletteURL(test.url, true, true, jsonFileLimit)
		if !ok || request.Page != test.page {
			t.Fatalf("%s -> %d %v, want %d", test.url, request.Page, ok, test.page)
		}
	}
}

func TestSettingsCommandsStayVisibleWhenModulesAreOff(t *testing.T) {
	a := awakeTestApp(t)
	a.config.AwakeEnabled, a.config.JSONEnabled = false, false
	ids := map[string]*command{}
	for _, cmd := range a.commands() {
		ids[cmd.ID] = cmd
	}
	for _, id := range []string{"settings.page.tiles", "settings.page.json"} {
		if ids[id] == nil {
			t.Fatalf("%s disappeared with its module", id)
		}
	}
	if got := ids["settings.page.tiles"].Title(a, "en"); got != a.text("en", "Awake") {
		t.Fatalf("tiles command title = %q", got)
	}
	// The old favorites ID keeps opening the keep awake page, and the JSON
	// command opens the JSON page.
	ids["settings.page.tiles"].Run(a, request{})
	if a.page != settingsAwake {
		t.Fatalf("tiles page = %d", a.page)
	}
	ids["settings.page.json"].Run(a, request{})
	if a.page != settingsJSON {
		t.Fatalf("json page = %d", a.page)
	}
	// The keywords still find the old and the new words.
	keywords := strings.Join(ids["settings.page.tiles"].Keywords, " ")
	for _, word := range []string{"格子", "tiles", "gezi", "gz", "模块", "modules", "mokuai", "mk", "常亮", "awake"} {
		if !strings.Contains(keywords, word) {
			t.Fatalf("tiles command lost the keyword %q", word)
		}
	}
}

func TestPaletteActionsFallBackWhenAModuleCloses(t *testing.T) {
	a := paletteTestApp(t)
	a.addJSONDocument("", "", true)
	p := a.palette
	p.query = "json"
	cmd := findCommand(a.commands(), "json.new")
	if cmd == nil {
		t.Fatal("missing the JSON command")
	}
	p.mode, p.actionsCommand, p.actionsRequest = paletteActionMode, cmd, request{Command: cmd}
	tt := ui.NewTester(a.paletteView, paletteWindowWidth(), paletteWindowHeight())
	tt.SetLocale("en")
	tt.Frame()
	a.changeJSONEnabled(false)
	a.invalidatePaletteAfterModuleChange()
	if p.mode != paletteCommandMode || p.actionsCommand != nil || p.runError != "" {
		t.Fatalf("a stale action page survived: %v %v %q", p.mode, p.actionsCommand, p.runError)
	}
	if p.query != "json" {
		t.Fatalf("the search text was cleared: %q", p.query)
	}
	tt.Frame()
	for _, row := range p.rows {
		if row.Kind == paletteRowCommand && strings.HasPrefix(row.Key, "json") {
			t.Fatalf("a closed module still lists %q", row.Key)
		}
	}
}

func TestPaletteRefusesACachedCommandThatIsGone(t *testing.T) {
	a := awakeTestApp(t)
	ran := false
	stale := &command{ID: "awake.start", Title: localizedTitle("R05Start"), Run: func(*application, request) error { ran = true; return nil }}
	a.palette = &paletteState{mode: paletteCommandMode, selected: 0, rows: []paletteRow{{Kind: paletteRowCommand, Key: stale.ID, Command: stale}}, request: request{Command: stale}}
	a.config.AwakeEnabled = false
	a.runPalette()
	if ran {
		t.Fatal("a command from a closed module ran")
	}
}

func TestCLIPageOptionAcceptsModuleNames(t *testing.T) {
	a := awakeTestApp(t)
	a.config.AwakeEnabled = false
	// A closed module reports disabled; its settings page still opens.
	for page, want := range map[string]int{"awake": settingsAwake, "json": settingsJSON, "shortcuts": settingsShortcuts} {
		result := a.applyCLI(cli.Request{Action: cli.ActionSettings, Page: page}, nil)
		if !result.OK {
			t.Fatalf("%s page rejected: %+v", page, result)
		}
		if a.page != want {
			t.Fatalf("%s page = %d, want %d", page, a.page, want)
		}
	}
	// A closed module is disabled for its actions, not for its page.
	if result := a.applyCLI(cli.Request{Action: cli.ActionAwakeStop}, nil); result.OK || result.Code != cli.CodeDisabled {
		t.Fatalf("stopped a closed module: %+v", result)
	}
}

func TestCLIBusyWhileAModuleCloses(t *testing.T) {
	a := awakeTestApp(t)
	a.moduleClosing = true
	result := a.applyCLI(cli.Request{Action: cli.ActionAwakeStop}, nil)
	if result.OK || result.Code != cli.CodeBusy {
		t.Fatalf("close in flight reported %+v", result)
	}
	if a.settingsError == "disabled" || a.config.AwakeEnabled == false {
		t.Fatal("a busy close changed the state")
	}
}

// --- Update check time ---

func TestUpdateCheckTimeRecordedOnFinishedAttempts(t *testing.T) {
	var result error
	block := false
	u := newUpdateManager(func(ctx context.Context) (*updateCandidate, error) {
		if block {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return nil, result
	}, nil, nil)
	clock := time.Date(2026, 11, 6, 13, 20, 0, 0, time.Local)
	u.now = func() time.Time { return clock }
	defer u.close()

	u.request(true)
	eventually(t, 2*time.Second, func() bool { return !u.isBusy() })
	if got := u.lastCheck(); !got.Equal(clock) {
		t.Fatalf("successful check time = %v", got)
	}

	// A finished failure is remembered as well.
	clock = clock.Add(time.Hour)
	u.now = func() time.Time { return clock }
	result = errors.New("network down")
	u.request(true)
	eventually(t, 2*time.Second, func() bool { return !u.isBusy() })
	if got := u.lastCheck(); !got.Equal(clock) {
		t.Fatalf("failed check time = %v", got)
	}

	// A cancellation is not a check: the request is cancelled while the check
	// is still running, as turning automatic updates off does.
	clock = clock.Add(time.Hour)
	u.now = func() time.Time { return clock }
	before := u.lastCheck()
	block = true
	u.mu.Lock()
	u.automatic = true
	u.mu.Unlock()
	u.request(false)
	u.setAutomatic(false)
	eventually(t, 2*time.Second, func() bool { return !u.isBusy() })
	if got := u.lastCheck(); !got.Equal(before) {
		t.Fatalf("a cancelled check moved the time to %v", got)
	}
	// A deadline is a finished failure.
	block = false
	clock = clock.Add(time.Hour)
	u.now = func() time.Time { return clock }
	result = context.DeadlineExceeded
	u.request(true)
	eventually(t, 2*time.Second, func() bool { return !u.isBusy() })
	if got := u.lastCheck(); !got.Equal(clock) {
		t.Fatalf("timed out check time = %v", got)
	}
}

func TestCheckedAtFormatting(t *testing.T) {
	a := awakeTestApp(t)
	now := time.Date(2026, 11, 6, 13, 20, 0, 0, time.Local)
	cases := []struct {
		checked time.Time
		locale  string
		want    string
	}{
		{time.Date(2026, 11, 6, 9, 5, 0, 0, time.Local), "zh-CN", "09:05"},
		{time.Date(2026, 11, 5, 9, 5, 0, 0, time.Local), "zh-CN", "昨天 09:05"},
		{time.Date(2026, 3, 2, 9, 5, 0, 0, time.Local), "zh-CN", "3月2日 09:05"},
		{time.Date(2025, 3, 2, 9, 5, 0, 0, time.Local), "zh-CN", "2025年3月2日 09:05"},
		{time.Date(2026, 11, 5, 9, 5, 0, 0, time.Local), "en", "Yesterday 09:05"},
		{time.Date(2026, 3, 2, 9, 5, 0, 0, time.Local), "en", "Mar 2, 09:05"},
		{time.Date(2025, 3, 2, 9, 5, 0, 0, time.Local), "en", "Mar 2, 2025, 09:05"},
	}
	for _, c := range cases {
		if got := a.checkedAtText(c.locale, c.checked, now); got != c.want {
			t.Fatalf("checkedAtText(%s, %v) = %q, want %q", c.locale, c.checked, got, c.want)
		}
	}
	// Yesterday is a calendar day, not 24 hours: a summer time change must not
	// shift it.
	if got := a.checkedAtText("zh-CN", time.Date(2026, 3, 8, 10, 0, 0, 0, time.Local), time.Date(2026, 3, 9, 10, 0, 0, 0, time.Local)); !strings.HasPrefix(got, "昨天 ") {
		t.Fatalf("daylight saving yesterday = %q", got)
	}
	// A record from the future is hidden, not rewritten.
	a.config.AutomaticUpdates = true
	a.updates.mu.Lock()
	a.updates.state.CheckedAt = now.Add(time.Hour)
	a.updates.mu.Unlock()
	a.updates.now = func() time.Time { return now }
	if got := a.updateScheduleHint("zh-CN"); got != a.text("zh-CN", "UpdateSchedule") {
		t.Fatalf("future record = %q", got)
	}
}

func TestUpdateScheduleSecondLine(t *testing.T) {
	a := awakeTestApp(t)
	a.updates.now = func() time.Time { return time.Date(2026, 11, 6, 13, 20, 0, 0, time.Local) }
	a.updates.mu.Lock()
	a.updates.state.CheckedAt = time.Date(2026, 11, 6, 13, 20, 0, 0, time.Local)
	a.updates.mu.Unlock()
	want := a.text("zh-CN", "UpdateSchedule") + " · " + fmt.Sprintf(a.text("zh-CN", "UpdateLastCheck"), "13:20")
	if got := a.updateScheduleHint("zh-CN"); got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
	a.config.AutomaticUpdates = false
	if got := a.updateScheduleHint("zh-CN"); !strings.HasPrefix(got, a.text("zh-CN", "UpdateScheduleOff")) {
		t.Fatalf("off hint = %q", got)
	}
}

func TestCheckedAtConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	now := time.Now().UTC()
	valid := now.Add(-time.Hour).Format(time.RFC3339Nano)
	content := fmt.Sprintf("version=1\nlanguage='zh-CN'\nupdate_last_check='%s'\n", valid)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.UpdateLastCheck != valid {
		t.Fatalf("check time = %q, want %q", config.UpdateLastCheck, valid)
	}
	// Counted as "still has it after a save and load".
	if err := saveConfig(path, config); err != nil {
		t.Fatal(err)
	}
	reloaded, err := loadConfig(path)
	if err != nil || reloaded.UpdateLastCheck != valid {
		t.Fatalf("reloaded = %q %v", reloaded.UpdateLastCheck, err)
	}
	for _, bad := range []string{"not-a-time", "1969-12-31T23:59:59Z", time.Now().Add(48 * time.Hour).Format(time.RFC3339Nano)} {
		content := fmt.Sprintf("version=1\nlanguage='zh-CN'\nupdate_last_check='%s'\n", bad)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		config, err := loadConfig(path)
		if err != nil {
			t.Fatalf("%q failed the whole load: %v", bad, err)
		}
		if config.UpdateLastCheck != "" {
			t.Fatalf("%q was kept as %q", bad, config.UpdateLastCheck)
		}
	}
}

func TestUpdateCheckTimeSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	a, err := newApplication(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.updates.close()
	checked := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	a.updates.mu.Lock()
	a.updates.state.CheckedAt = checked
	a.updates.mu.Unlock()
	a.syncUpdateCheck()
	loaded, err := loadConfig(path)
	if err != nil || loaded.UpdateLastCheck == "" {
		t.Fatalf("check time not saved: %+v %v", loaded, err)
	}
	// A restart shows the record even with automatic updates off.
	restarted := newUpdateManager(nil, nil, nil)
	defer restarted.close()
	restarted.restoreCheck(loaded.UpdateLastCheck, time.Now())
	if got := restarted.lastCheck(); !got.Equal(checked) {
		t.Fatalf("restored time = %v, want %v", got, checked)
	}
}

// --- Localization ---

func TestModuleLocaleKeys(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		a := awakeTestApp(t)
		for _, key := range []string{
			"Modules", "FeedbackGroup", "AwakePageDesc", "JSONPageDesc", "ShortcutsPageDesc",
			"GeneralPageDesc", "AboutPageDesc", "GroupSettings", "GroupShortcuts",
			"GroupPreferences", "LidBatteryHint", "CLILinkHint", "ModuleShortcutOff",
			"FeedbackModeInfo", "FeedbackPositionInfo", "UpdateSchedule",
			"UpdateLastCheck", "UpdateYesterday", "UpdateScheduleOff",
			"AwakeStopError", "ModuleBusy",
		} {
			// Some names are the same in both languages ("JSON", "Modules").
			if value := a.text(locale, key); value == "" || strings.Contains(value, "%!") {
				t.Fatalf("%s: missing %s", locale, key)
			}
		}
		empty := map[string]string{"zh-CN": "模块", "en": "modules"}[locale]
		if got := a.text(locale, "PanelEmpty"); !strings.Contains(strings.ToLower(got), empty) {
			t.Fatalf("%s: the empty panel does not say modules: %q", locale, got)
		}
		// Placeholders are the same in both languages.
		for _, key := range []string{"ModuleShortcutOff", "UpdateLastCheck", "UpdateYesterday", "CLIInstalled", "CLILinkConflict"} {
			if strings.Count(a.text(locale, key), "%s") != 1 {
				t.Fatalf("%s: %s placeholders = %q", locale, key, a.text(locale, key))
			}
		}
	}
}

// --- Helpers used above ---

// isBusy reports whether the manager is working, without exposing its lock.
func (u *updateManager) isBusy() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.busy
}

func TestAwakeCloseWithLeftoverMarker(t *testing.T) {
	a, _, _, svc, _ := lidTestApp(t)
	// No session at all, but a marker left by an interrupted restore: the close
	// still has to make sure the system setting is ours again.
	if err := os.WriteFile(a.lid.marker, []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.changeAwakeEnabled(false)
	if a.config.AwakeEnabled {
		t.Fatal("the module stayed open after a safe close")
	}
	if a.lid.markerExists() {
		t.Fatal("the marker survived the close")
	}
	if svc.unregisters != 1 {
		t.Fatalf("helper unregisters = %d", svc.unregisters)
	}
}

func TestAwakeCloseStopsWhenTheHelperIsUnreachable(t *testing.T) {
	a, _, _, svc, conn := lidTestApp(t)
	if err := os.WriteFile(a.lid.marker, []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The helper is not answering: an unknown system state is not safe to close.
	conn.gone = true
	a.changeAwakeEnabled(false)
	if !a.config.AwakeEnabled || a.moduleError != "LidRestoreFailed" {
		t.Fatalf("an unreachable helper closed the module: enabled=%v error=%q", a.config.AwakeEnabled, a.moduleError)
	}
	if svc.unregisters != 0 {
		t.Fatal("the helper was uninstalled without a restored system state")
	}
	// The local read-back still ran, so the panel keeps showing the truth.
	if a.lid.snapshot().state == lidOn {
		t.Fatal("the helper is reported as holding the setting")
	}
}

func TestSettingsRowSecondLineStaysOneLine(t *testing.T) {
	a := awakeTestApp(t)
	long := strings.Repeat("a long rule that must not wrap ", 6)
	view := func(c *ui.Context) {
		_, tk := applyTheme(c)
		a.settingsRow(c, tk, "automatic updates", settingsRowOpts{icon: "refresh-cw", level: 2, hint: long}, func() {
			ui.Box(c).Size(tk.SizeSwitchWidth, tk.SizeSwitchHeight)
		})
	}
	tt := ui.NewTester(view, 300, 200)
	tt.SetLocale("en")
	tt.Frame()
	hint, ok := tt.Find(long)
	if !ok {
		t.Fatal("the long hint is missing")
	}
	if hint.H > 18.5 {
		t.Fatalf("the second line wrapped: %v", hint)
	}
	if hint.W > 300 {
		t.Fatalf("the second line escaped the card: %v", hint)
	}
	// The full text stays available even when the line is cut.
	if !hasAccessibleText(tt, long) {
		t.Fatal("the truncated line lost its full text")
	}
}

func TestCLIRowHiddenWhenPathsCannotBeProbed(t *testing.T) {
	a := awakeTestApp(t)
	original := cliPathsFor
	cliPathsFor = func() (cliPaths, bool) { return cliPaths{}, false }
	t.Cleanup(func() { cliPathsFor = original })
	a.page = settingsGeneral
	tt := ui.NewTester(a.view, 720, settingsWindowHeight)
	tt.SetLocale("en")
	if _, found := tt.Find(a.text("en", "CLIName")); found {
		t.Fatal("the command line row was drawn without paths")
	}
	if l := newSettingsLayout(a, settingsGeneral); l.hasCLI {
		t.Fatal("the layout claimed a CLI row")
	}
}

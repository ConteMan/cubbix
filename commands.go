package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo"
)

// commandParam is the kind of argument a command accepts. Only keep awake
// takes one today; add new kinds here.
type commandParam int

const (
	paramNone  commandParam = iota
	paramAwake              // duration, until, unlimited, or the remembered setting
)

// awakeArgKind is the parsed form of a keep awake argument.
type awakeArgKind int

const (
	awakeArgDefault   awakeArgKind = iota // use the remembered setting
	awakeArgMinutes                       // fixed number of minutes
	awakeArgUntil                         // until a clock time
	awakeArgUnlimited                     // no deadline
)

type awakeArg struct {
	Kind    awakeArgKind
	Minutes int    // awakeArgMinutes
	Until   string // awakeArgUntil, "18:30"
	Error   string // non-empty is a resource key: DurationError or TimeError
}

// command is pure data plus functions; it never holds a window.
type command struct {
	ID        string                                     // stable: favorites, recents, URL routing
	Title     func(a *application, locale string) string // always through application.text
	Keywords  []string                                   // aliases, pinyin and initials
	Tile      string                                     // "awake" / "json" / "" (general)
	Icon      string                                     // Lucide file name
	Param     commandParam
	Available func(a *application) bool
	Run       func(a *application, req request) error
	Actions   func(a *application, req request) []paletteAction
}

// request is one execution: the raw argument and its parsed keep awake form.
type request struct {
	Command *command
	Arg     string
	Awake   awakeArg
}

// paletteAction is one row of the ⌘K action list.
type paletteAction struct {
	ID        string
	Title     func(a *application, locale string) string
	Icon      string // empty draws no icon
	Group     int    // 0 primary, 1 parameter, 2 organization
	Checked   func(a *application) bool
	Available func(a *application) bool
	Run       func(a *application) error
}

// localizedTitle wraps a resource key into a title function. Titles must go
// through application.text; a raw key would show "R05Start" in the panel.
func localizedTitle(key string) func(a *application, locale string) string {
	return func(a *application, locale string) string { return a.text(locale, key) }
}

// commands assembles the registry each time the palette shows: keep awake,
// the open JSON windows, then the general commands. Availability is evaluated
// once here; the panel is a short-lived overlay and does not refresh.
func (a *application) commands() []*command {
	all := a.awakeCommands()
	all = append(all, a.jsonCommands()...)
	all = append(all, a.systemCommands()...)
	commands := make([]*command, 0, len(all))
	for _, command := range all {
		if command.Available == nil || command.Available(a) {
			commands = append(commands, command)
		}
	}
	return commands
}

func (a *application) awakeCommands() []*command {
	available := func(a *application) bool { return a.config.AwakeEnabled }
	active := func(a *application) bool { return a.awake.snapshot().Active }
	runStart := func(a *application, req request) error { return a.runAwakeStart(req) }
	return []*command{
		{
			ID:        "awake.start",
			Title:     localizedTitle("R05Start"),
			Keywords:  []string{"常亮", "awake", "kaiqi", "kq", "changliang", "cl", "keep", "on"},
			Tile:      "awake",
			Icon:      "sun",
			Param:     paramAwake,
			Available: available,
			Run:       runStart,
			Actions:   awakeStartActions,
		},
		{
			ID:        "awake.stop",
			Title:     localizedTitle("PaletteAwakeStop"),
			Keywords:  []string{"停止", "关闭常亮", "stop", "tingzhi", "tz", "off"},
			Tile:      "awake",
			Icon:      "power",
			Available: active,
			Run: func(a *application, req request) error {
				a.awake.stop()
				return nil
			},
			Actions: func(a *application, req request) []paletteAction {
				return favoriteActions(a, req.Command.ID)
			},
		},
		{
			ID:        "awake.mode.screen",
			Title:     localizedTitle("R05Screen"),
			Keywords:  []string{"screen", "屏幕", "pingmu", "pm"},
			Tile:      "awake",
			Icon:      "sun",
			Available: available,
			Run: func(a *application, req request) error {
				return a.setMode("screen")
			},
			Actions: func(a *application, req request) []paletteAction {
				return favoriteActions(a, req.Command.ID)
			},
		},
		{
			ID:        "awake.mode.tasks",
			Title:     localizedTitle("R05Tasks"),
			Keywords:  []string{"tasks", "任务", "仅任务", "renwu", "rw"},
			Tile:      "awake",
			Icon:      "gauge",
			Available: available,
			Run: func(a *application, req request) error {
				return a.setMode("tasks")
			},
			Actions: func(a *application, req request) []paletteAction {
				return favoriteActions(a, req.Command.ID)
			},
		},
		{
			ID:        "awake.until",
			Title:     localizedTitle("R05Until"),
			Keywords:  []string{"until", "直到", "zhidao", "zd"},
			Tile:      "awake",
			Icon:      "clock-3",
			Available: available,
			Run: func(a *application, req request) error {
				a.openAwakeDialog("until")
				return nil
			},
			Actions: func(a *application, req request) []paletteAction {
				return favoriteActions(a, req.Command.ID)
			},
		},
		{
			ID:        "awake.custom",
			Title:     localizedTitle("R05Custom"),
			Keywords:  []string{"custom", "自定义", "zidingyi", "zdy"},
			Tile:      "awake",
			Icon:      "clock-3",
			Available: available,
			Run: func(a *application, req request) error {
				a.openAwakeDialog("custom")
				return nil
			},
			Actions: func(a *application, req request) []paletteAction {
				return favoriteActions(a, req.Command.ID)
			},
		},
		{
			ID:        "awake.follow",
			Title:     localizedTitle("R05Follow"),
			Keywords:  []string{"follow", "跟随", "进程", "gensui", "gs", "jincheng"},
			Tile:      "awake",
			Icon:      "terminal",
			Available: available,
			Run: func(a *application, req request) error {
				a.openAwakeDialog("follow")
				return nil
			},
			Actions: func(a *application, req request) []paletteAction {
				return favoriteActions(a, req.Command.ID)
			},
		},
	}
}

func (a *application) jsonCommands() []*command {
	if !a.config.JSONEnabled {
		return nil
	}
	available := func(a *application) bool { return a.config.JSONEnabled }
	commands := []*command{{
		ID:        "json.new",
		Title:     localizedTitle("R07NewWindow"),
		Keywords:  []string{"json", "新建", "xinjian", "xj", "new"},
		Tile:      "json",
		Icon:      "braces",
		Available: available,
		Run: func(a *application, req request) error {
			a.newJSONWindow()
			return nil
		},
		Actions: jsonNewActions,
	}}
	for _, document := range a.jsonDocuments {
		d := document
		if d.closed {
			continue
		}
		commands = append(commands, &command{
			ID:        fmt.Sprintf("json.window.%d", d.number),
			Title:     func(a *application, locale string) string { return d.name(a, locale) },
			Keywords:  jsonWindowKeywords(a, d),
			Tile:      "json",
			Icon:      "file-type",
			Available: func(a *application) bool { return !d.closed },
			Run: func(a *application, req request) error {
				a.showJSON(d)
				return nil
			},
			Actions: func(a *application, req request) []paletteAction {
				return jsonWindowActions(a, d)
			},
		})
	}
	return commands
}

func jsonWindowKeywords(a *application, d *jsonDocument) []string {
	var keywords []string
	seen := map[string]bool{}
	for _, name := range []string{"json", d.name(a, "zh-CN"), d.name(a, "en")} {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		keywords = append(keywords, name)
	}
	if base := filepath.Base(d.name(a, "zh-CN")); base != "" && !seen[base] {
		keywords = append(keywords, base)
	}
	return keywords
}

func (a *application) systemCommands() []*command {
	updateAvailable := func(a *application) bool {
		phase := a.updates.snapshot().Phase
		return phase != updateChecking && phase != updateDownloading && phase != updateReady
	}
	restartAvailable := func(a *application) bool { return a.updates.snapshot().Phase == updateReady }
	openPage := func(page int) func(a *application, req request) error {
		return func(a *application, req request) error {
			a.page, a.notesPage = page, false
			a.invalidateWindow()
			a.show()
			return nil
		}
	}
	return []*command{
		{
			ID:       "settings.open",
			Title:    localizedTitle("MenuSettings"),
			Keywords: []string{"设置", "settings", "shezhi", "sz", "preferences"},
			Icon:     "settings-2",
			Run: func(a *application, req request) error {
				a.show()
				return nil
			},
			Actions: func(a *application, req request) []paletteAction {
				return favoriteActions(a, req.Command.ID)
			},
		},
		{
			ID:       "settings.page.general",
			Title:    localizedTitle("General"),
			Keywords: []string{"设置", "通用", "general", "tongyong", "ty"},
			Icon:     "settings-2",
			Run:      openPage(0),
			Actions: func(a *application, req request) []paletteAction {
				return favoriteActions(a, req.Command.ID)
			},
		},
		{
			ID:       "settings.page.tiles",
			Title:    localizedTitle("R05Tiles"),
			Keywords: []string{"设置", "格子", "tiles", "gezi", "gz"},
			Icon:     "settings-2",
			Run:      openPage(2),
			Actions: func(a *application, req request) []paletteAction {
				return favoriteActions(a, req.Command.ID)
			},
		},
		{
			ID:       "settings.page.shortcuts",
			Title:    localizedTitle("R05Shortcuts"),
			Keywords: []string{"设置", "快捷键", "shortcuts", "kuaijie", "kjj"},
			Icon:     "settings-2",
			Run:      openPage(3),
			Actions: func(a *application, req request) []paletteAction {
				return favoriteActions(a, req.Command.ID)
			},
		},
		{
			ID:       "settings.page.about",
			Title:    localizedTitle("About"),
			Keywords: []string{"设置", "关于", "about", "guanyu", "gy"},
			Icon:     "settings-2",
			Run:      openPage(1),
			Actions: func(a *application, req request) []paletteAction {
				return favoriteActions(a, req.Command.ID)
			},
		},
		{
			ID:        "update.check",
			Title:     localizedTitle("CheckAction"),
			Keywords:  []string{"更新", "检查更新", "update", "gengxin", "gx"},
			Icon:      "refresh-cw",
			Available: updateAvailable,
			Run: func(a *application, req request) error {
				a.updates.request(true)
				return nil
			},
			Actions: func(a *application, req request) []paletteAction {
				return favoriteActions(a, req.Command.ID)
			},
		},
		{
			ID:        "update.restart",
			Title:     localizedTitle("MenuRestartUpdate"),
			Keywords:  []string{"重启", "restart", "chongqi"},
			Icon:      "refresh-cw",
			Available: restartAvailable,
			Run: func(a *application, req request) error {
				a.restartUpdate()
				return nil
			},
			Actions: func(a *application, req request) []paletteAction {
				return favoriteActions(a, req.Command.ID)
			},
		},
		{
			ID:       "app.quit",
			Title:    localizedTitle("Quit"),
			Keywords: []string{"退出", "quit", "tuichu", "tc"},
			Icon:     "power",
			Run: func(a *application, req request) error {
				mygo.App.Quit()
				return nil
			},
			Actions: func(a *application, req request) []paletteAction {
				return favoriteActions(a, req.Command.ID)
			},
		},
	}
}

// runAwakeStart executes awake.start and follows the same paths as the card
// panel and the menu: a remembered session, a fixed duration, until a time,
// or no deadline.
func (a *application) runAwakeStart(req request) error {
	switch req.Awake.Kind {
	case awakeArgMinutes:
		return a.setDuration(req.Awake.Minutes)
	case awakeArgUntil:
		end, err := untilTime(req.Awake.Until, time.Now())
		if err != nil {
			return err
		}
		a.config.AwakeUntil = req.Awake.Until
		a.save()
		return a.startAwake(awakeSession{Mode: a.config.AwakeMode, End: end})
	case awakeArgUnlimited:
		return a.setDuration(0)
	default:
		return a.startAwake(a.selectedSession())
	}
}

// startAwakeWith starts keep awake with the given parsed argument and mode.
// It is the action-list path: the mode is an explicit choice and replaces the
// remembered one, while the duration comes from the input unless the action
// itself chose a preset.
func (a *application) startAwakeWith(arg awakeArg, mode string) error {
	if arg.Error != "" {
		// The action keeps the typed parameter, so an invalid one stays an error.
		return errors.New(arg.Error)
	}
	a.config.AwakeMode = mode
	switch arg.Kind {
	case awakeArgMinutes:
		a.config.AwakeMinutes = arg.Minutes
		a.config.AwakeUntil = ""
	case awakeArgUnlimited:
		a.config.AwakeMinutes = 0
		a.config.AwakeUntil = ""
	case awakeArgUntil:
		a.config.AwakeUntil = arg.Until
	}
	a.save()
	var session awakeSession
	switch arg.Kind {
	case awakeArgUntil:
		end, err := untilTime(arg.Until, time.Now())
		if err != nil {
			return err
		}
		session = awakeSession{Mode: mode, End: end}
	case awakeArgMinutes:
		session = awakeSession{Mode: mode, End: time.Now().Add(time.Duration(arg.Minutes) * time.Minute)}
	default:
		session = a.selectedSession()
		session.Mode = mode
	}
	return a.startAwake(session)
}

func awakeStartActions(a *application, req request) []paletteAction {
	arg := req.Awake
	actions := []paletteAction{
		{
			ID:      "awake.start.screen",
			Title:   localizedTitle("R05Screen"),
			Icon:    "sun",
			Checked: func(a *application) bool { return a.config.AwakeMode == "screen" },
			Run:     func(a *application) error { return a.startAwakeWith(arg, "screen") },
		},
		{
			ID:      "awake.start.tasks",
			Title:   localizedTitle("R05Tasks"),
			Icon:    "gauge",
			Checked: func(a *application) bool { return a.config.AwakeMode == "tasks" },
			Run:     func(a *application) error { return a.startAwakeWith(arg, "tasks") },
		},
	}
	for _, preset := range []struct {
		minutes int
		key     string
	}{{30, "R05Half"}, {60, "R05Hour"}, {120, "R05Two"}, {240, "R05Four"}} {
		minutes := preset.minutes
		actions = append(actions, paletteAction{
			ID:      fmt.Sprintf("awake.start.%dm", minutes),
			Title:   localizedTitle(preset.key),
			Icon:    "clock-3",
			Group:   1,
			Checked: func(a *application) bool { return arg.Kind == awakeArgMinutes && arg.Minutes == minutes },
			Run:     func(a *application) error { return a.setDuration(minutes) },
		})
	}
	actions = append(actions,
		paletteAction{
			ID:      "awake.start.unlimited",
			Title:   localizedTitle("R05Unlimited"),
			Icon:    "clock-3",
			Group:   1,
			Checked: func(a *application) bool { return arg.Kind == awakeArgUnlimited },
			Run:     func(a *application) error { return a.setDuration(0) },
		},
		paletteAction{
			ID:    "awake.start.until",
			Title: localizedTitle("R05Until"),
			Icon:  "clock-3",
			Group: 1,
			Run: func(a *application) error {
				a.openAwakeDialog("until")
				return nil
			},
		},
		paletteAction{
			ID:    "awake.start.custom",
			Title: localizedTitle("R05Custom"),
			Icon:  "clock-3",
			Group: 1,
			Run: func(a *application) error {
				a.openAwakeDialog("custom")
				return nil
			},
		},
		paletteAction{
			ID:    "awake.start.follow",
			Title: localizedTitle("R05Follow"),
			Icon:  "terminal",
			Group: 1,
			Run: func(a *application) error {
				a.openAwakeDialog("follow")
				return nil
			},
		},
	)
	return append(actions, favoriteActions(a, req.Command.ID)...)
}

func jsonNewActions(a *application, req request) []paletteAction {
	return append([]paletteAction{
		{
			ID:    "json.new.clipboard",
			Title: localizedTitle("PaletteJSONClipboard"),
			Icon:  "clipboard-copy",
			Run: func(a *application) error {
				a.newJSONWindow()
				return nil
			},
		},
		{
			ID:    "json.new.blank",
			Title: localizedTitle("PaletteJSONBlank"),
			Icon:  "braces",
			Run: func(a *application) error {
				a.newJSONWindowBlank()
				return nil
			},
		},
	}, favoriteActions(a, req.Command.ID)...)
}

func jsonWindowActions(a *application, d *jsonDocument) []paletteAction {
	pin := "R07Pin"
	if d.pinned {
		pin = "R07Unpin"
	}
	return []paletteAction{
		{
			ID:    "json.window.switch",
			Title: localizedTitle("PaletteSwitchTo"),
			Run: func(a *application) error {
				a.showJSON(d)
				return nil
			},
		},
		{
			ID:    "json.window.pin",
			Title: localizedTitle(pin),
			Icon:  "pin",
			Group: 1,
			Run: func(a *application) error {
				a.setJSONPinned(d, !d.pinned)
				return nil
			},
		},
		{
			ID:    "json.window.close",
			Title: localizedTitle("JSONClose"),
			Icon:  "x",
			Group: 1,
			Run: func(a *application) error {
				if d.window != nil {
					d.window.Close()
				}
				return nil
			},
		},
	}
}

// favoriteActions offers the one organization action of a favoritable
// command. JSON windows are left out: they vanish with their document.
func favoriteActions(a *application, id string) []paletteAction {
	if strings.HasPrefix(id, "json.window.") {
		return nil
	}
	title, glyph := "PaletteAddFavorite", "star"
	if a.isFavorite(id) {
		title, glyph = "PaletteRemoveFavorite", "star-off"
	}
	return []paletteAction{{
		ID:    "palette.favorite",
		Title: localizedTitle(title),
		Icon:  glyph,
		Group: 2,
		Run: func(a *application) error {
			a.toggleFavorite(id)
			return nil
		},
	}}
}

// --- Search ---

// normalizeQuery trims and folds runs of whitespace into one space.
func normalizeQuery(query string) string { return strings.Join(strings.Fields(query), " ") }

// commandNames returns every searchable name of a command: its title and each
// keyword.
func commandNames(a *application, locale string, cmd *command) []string {
	names := []string{cmd.Title(a, locale)}
	return append(names, cmd.Keywords...)
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func runeIndex(s, sub string) int {
	index := strings.Index(s, sub)
	if index < 0 {
		return -1
	}
	return utf8.RuneCountInString(s[:index])
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// subsequenceSkips counts the characters of name skipped to spell query in
// order. It only applies to ASCII, so Chinese titles are never split into
// meaningless single-character hits.
func subsequenceSkips(name, query string) (int, bool) {
	if !isASCII(name) || !isASCII(query) {
		return 0, false
	}
	skips := 0
	matched := 0
	for i := 0; i < len(name); i++ {
		if matched == len(query) {
			break
		}
		if name[i] == query[matched] {
			matched++
			continue
		}
		skips++
	}
	if matched != len(query) {
		return 0, false
	}
	return skips, true
}

// matchScore returns the best score for a query against a title and its
// keywords, 0 when it does not match. A multi-word query needs every word.
func matchScore(query, title string, keywords []string) int {
	tokens := strings.Fields(strings.ToLower(query))
	if len(tokens) == 0 {
		return 0
	}
	total := 0
	for _, token := range tokens {
		best := 0
		consider := func(name string, exact, prefix, contains, subsequence int) {
			lower := strings.ToLower(name)
			if lower == token {
				best = max(best, exact)
			}
			if strings.HasPrefix(lower, token) {
				best = max(best, prefix-runeLen(lower))
			}
			if index := runeIndex(lower, token); index >= 0 {
				best = max(best, contains-index)
			}
			if skips, ok := subsequenceSkips(lower, token); ok {
				best = max(best, subsequence-2*skips)
			}
		}
		consider(title, 1000, 900, 600, 400)
		for _, keyword := range keywords {
			consider(keyword, 800, 700, 500, 300)
		}
		if best <= 0 {
			return 0
		}
		total += best
	}
	return total
}

// commandPrefix is the result of splitting a query into command and argument.
type commandPrefix struct {
	Command *command
	Arg     string
	Shared  bool // more than one command carries this exact name
}

// splitCommandQuery matches whole command names at the start of a query. A
// name matches when the query equals it, starts with it and a space, or has a
// non-letter right after it (a digit, ∞, :, -, +). The longest name wins;
// ties follow registration order.
func splitCommandQuery(a *application, locale, query string, commands []*command) (commandPrefix, bool) {
	query = strings.ToLower(normalizeQuery(query))
	if query == "" {
		return commandPrefix{}, false
	}
	best := commandPrefix{}
	bestLen := 0
	exact := 0
	for _, cmd := range commands {
		for _, name := range commandNames(a, locale, cmd) {
			name = strings.ToLower(name)
			if name == "" {
				continue
			}
			if query == name {
				exact++
			}
			arg, ok := commandPrefixMatch(query, name)
			if !ok {
				continue
			}
			if length := runeLen(name); length > bestLen {
				best = commandPrefix{Command: cmd, Arg: arg}
				bestLen = length
			}
		}
	}
	if best.Command == nil {
		return commandPrefix{}, false
	}
	best.Shared = exact > 1
	return best, true
}

func commandPrefixMatch(query, name string) (string, bool) {
	if query == name {
		return "", true
	}
	if strings.HasPrefix(query, name+" ") {
		return strings.TrimSpace(query[len(name):]), true
	}
	if strings.HasPrefix(query, name) && len(query) > len(name) {
		r, _ := utf8.DecodeRuneInString(query[len(name):])
		if !unicode.IsLetter(r) {
			return strings.TrimSpace(query[len(name):]), true
		}
	}
	return "", false
}

// bestCommand picks the default selection for a query. A parameter prefix
// with an argument wins outright; otherwise the highest score wins, with
// registration order as the tie-break (so "json" selects json.new).
func bestCommand(a *application, locale, query string, commands []*command) (*command, commandPrefix) {
	prefix, ok := splitCommandQuery(a, locale, query, commands)
	if ok && prefix.Arg == "" {
		// Shared names and no argument: registration order decides, not the
		// title length, so "json" always selects json.new.
		for _, cmd := range commands {
			for _, name := range commandNames(a, locale, cmd) {
				if strings.ToLower(name) == strings.ToLower(normalizeQuery(query)) {
					return cmd, prefix
				}
			}
		}
		return prefix.Command, prefix
	}
	if ok && prefix.Command.Param != paramNone {
		return prefix.Command, prefix
	}
	var best *command
	bestScore := 0
	for _, cmd := range commands {
		if cmd.Available != nil && !cmd.Available(a) {
			continue
		}
		score := matchScore(query, cmd.Title(a, locale), cmd.Keywords)
		if score > bestScore {
			best, bestScore = cmd, score
		}
	}
	if best == nil {
		return nil, commandPrefix{}
	}
	return best, prefix
}

// sortedCommands orders matches by score, then shorter title, then
// registration order. The input order is the registration order.
func sortedCommands(a *application, locale, query string, commands []*command) []*command {
	out := append([]*command(nil), commands...)
	if normalizeQuery(query) == "" {
		// Without a query the registration order is the stable order.
		return out
	}
	sort.SliceStable(out, func(i, j int) bool {
		left, right := out[i], out[j]
		leftScore := matchScore(query, left.Title(a, locale), left.Keywords)
		rightScore := matchScore(query, right.Title(a, locale), right.Keywords)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return runeLen(left.Title(a, locale)) < runeLen(right.Title(a, locale))
	})
	return out
}

// --- Keep awake argument parsing ---

const (
	awakeMinutesMinimum = 1
	awakeMinutesMaximum = 525659 // 8760 hours 59 minutes
)

// parseAwakeArg parses the whole argument; every grammar in the table must
// match end to end. Combined forms come before a lone hour or minute.
func parseAwakeArg(arg string, now time.Time) awakeArg {
	value := strings.TrimSpace(arg)
	if value == "" {
		return awakeArg{Kind: awakeArgDefault}
	}
	lower := strings.ToLower(value)
	switch lower {
	case "∞", "inf", "infinity", "unlimited", "无限", "不限", "不限时", "0":
		return awakeArg{Kind: awakeArgUnlimited}
	}
	if until, ok := awakeUntilArg(lower); ok {
		if _, err := untilTime(until, now); err != nil {
			return awakeArg{Kind: awakeArgUntil, Error: "TimeError"}
		}
		return awakeArg{Kind: awakeArgUntil, Until: until}
	}
	minutes, ok := parseAwakeMinutes(lower)
	if !ok || minutes < awakeMinutesMinimum || minutes > awakeMinutesMaximum {
		return awakeArg{Error: "DurationError"}
	}
	return awakeArg{Kind: awakeArgMinutes, Minutes: minutes}
}

// awakeUntilArg recognizes "until 18:30", "到 18:30", "直到 18:30" and a bare
// clock time. Validation is left to untilTime, the same function the dialog
// uses.
func awakeUntilArg(value string) (string, bool) {
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

// parseAwakeMinutes parses 45, 45m, 2h, 1h30m, 1h 30m and the Chinese forms,
// returning minutes.
func parseAwakeMinutes(value string) (int, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.NewReplacer(
		"小时", "h",
		"分钟", "m",
		"hours", "h",
		"hour", "h",
		"hrs", "h",
		"hr", "h",
		"mins", "m",
		"min", "m",
		"分", "m",
	).Replace(normalized)
	digits := func(at int) (int, int, bool) {
		start := at
		for at < len(normalized) && normalized[at] >= '0' && normalized[at] <= '9' {
			at++
		}
		if at == start {
			return 0, at, false
		}
		number, err := strconv.Atoi(normalized[start:at])
		return number, at, err == nil
	}
	first, index, ok := digits(0)
	if !ok {
		return 0, false
	}
	if index == len(normalized) {
		return first, true // bare digits are minutes
	}
	switch normalized[index] {
	case 'h':
		index++
		if index == len(normalized) {
			return first * 60, true
		}
		minutes, end, ok := digits(index)
		if !ok || minutes > 59 {
			return 0, false
		}
		if end == len(normalized) || end+1 == len(normalized) && normalized[end] == 'm' {
			return first*60 + minutes, true
		}
		return 0, false
	case 'm':
		if index+1 != len(normalized) {
			return 0, false
		}
		return first, true
	}
	return 0, false
}

// awakeHint describes the parsed argument for the row's right side. It only
// talks about the duration; the mode lives in the ⌘K action list.
func awakeHint(a *application, locale string, arg awakeArg) string {
	text := func(key string) string { return a.text(locale, key) }
	switch arg.Kind {
	case awakeArgMinutes:
		return awakeDurationText(a, locale, arg.Minutes)
	case awakeArgUntil:
		return fmt.Sprintf(text("PaletteAwakeUntil"), arg.Until)
	case awakeArgUnlimited:
		return text("R05Unlimited")
	default:
		if a.config.AwakeUntil != "" {
			return fmt.Sprintf(text("PaletteAwakeUntil"), a.config.AwakeUntil)
		}
		return awakeDurationText(a, locale, a.config.AwakeMinutes)
	}
}

func awakeDurationText(a *application, locale string, minutes int) string {
	text := func(key string) string { return a.text(locale, key) }
	switch minutes {
	case 0:
		return text("R05Unlimited")
	case 30:
		return text("R05Half")
	case 60:
		return text("R05Hour")
	case 120:
		return text("R05Two")
	case 240:
		return text("R05Four")
	}
	return fmt.Sprintf(text("PaletteMinutes"), minutes)
}

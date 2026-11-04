package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ConteMan/cubbix/internal/awake"
)

// helpText is the whole help output. The tool's own strings are English: the
// app's answers already carry the interface language.
const helpText = `Usage: cubbix <command> [options]

Commands:
  awake on [options]   Start keep awake  (--minutes N | --until HH:MM | --follow PID)
                                             [--mode screen|tasks] [--lid|--no-lid]
  awake off            Stop keep awake
  awake toggle         Toggle keep awake (same as the global shortcut)
  awake status         Print the keep awake state
  status               Print keep awake, JSON windows and version
  json new [options]   Open a JSON window  (--text STR | --file PATH | --blank)
  palette              Open the command palette
  settings [--page P]  Open Settings  (general|tiles|shortcuts|about)
  version              Print the version of this tool and its app bundle
  help [command]       Show this help

Options:
  --json               Print one JSON object with stable fields
  --notify             Show the same hint or notification as the shortcut
  --no-launch          Do not start Cubbix when it is not running
  --timeout DURATION   Wait at most this long for Cubbix (default 5s, max 60s)

Debugging:
  --socket PATH        Talk to the app at PATH instead of the default socket

Exit status: 0 ok, 1 internal error, 2 usage, 3 Cubbix unavailable,
             4 rejected, 5 timeout
`

var settingsPages = map[string]bool{"general": true, "tiles": true, "shortcuts": true, "about": true}

// parse reads the command line. Every mistake is a usage error.
func parse(argv []string) (inv invocation, err error) {
	inv.flags.timeout = 5 * time.Second
	inv.page = "general"
	var positional []string
	var seen []string
	value := func(i *int, name, inline string, hasInline bool) (string, error) {
		if hasInline {
			return inline, nil
		}
		if *i+1 >= len(argv) {
			return "", usagef("Option %s needs a value.", name)
		}
		*i++
		return argv[*i], nil
	}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			positional = append(positional, argv[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		name, inline, hasInline := strings.Cut(arg, "=")
		switch name {
		case "--json", "--notify", "--no-launch", "--blank", "--lid", "--no-lid":
			if hasInline {
				return inv, usagef("Option %s takes no value.", name)
			}
			seen = append(seen, name)
			switch name {
			case "--json":
				inv.flags.json = true
			case "--notify":
				inv.flags.notify = true
			case "--no-launch":
				inv.flags.noLaunch = true
			case "--blank":
				inv.body.blank = true
			case "--lid":
				on := true
				inv.awake.lid = &on
			case "--no-lid":
				off := false
				inv.awake.lid = &off
			}
		case "-h", "--help":
			seen = append(seen, name)
			positional = append(positional, "help")
		case "--minutes":
			raw, err := value(&i, name, inline, hasInline)
			if err != nil {
				return inv, err
			}
			seen = append(seen, name)
			minutes, err := strconv.Atoi(raw)
			if err != nil || minutes < 0 || minutes > awake.MinutesMaximum {
				return inv, usagef("Invalid value for --minutes: %q", raw)
			}
			inv.awake.minutes, inv.awake.hasMin = minutes, true
		case "--until":
			raw, err := value(&i, name, inline, hasInline)
			if err != nil {
				return inv, err
			}
			seen = append(seen, name)
			if _, err := awake.UntilTime(raw, time.Now()); err != nil {
				return inv, usagef("Invalid value for --until: %q", raw)
			}
			inv.awake.until = raw
		case "--follow":
			raw, err := value(&i, name, inline, hasInline)
			if err != nil {
				return inv, err
			}
			seen = append(seen, name)
			pid, err := strconv.Atoi(raw)
			if err != nil || pid <= 0 {
				return inv, usagef("Invalid value for --follow: %q", raw)
			}
			inv.awake.follow = pid
		case "--mode":
			raw, err := value(&i, name, inline, hasInline)
			if err != nil {
				return inv, err
			}
			seen = append(seen, name)
			if raw != "screen" && raw != "tasks" {
				return inv, usagef("Invalid value for --mode: %q", raw)
			}
			inv.awake.mode = raw
		case "--timeout":
			raw, err := value(&i, name, inline, hasInline)
			if err != nil {
				return inv, err
			}
			seen = append(seen, name)
			duration, err := time.ParseDuration(raw)
			if err != nil || duration < time.Second || duration > 60*time.Second {
				return inv, usagef("Invalid value for --timeout: %q", raw)
			}
			inv.flags.timeout = duration
		case "--socket":
			raw, err := value(&i, name, inline, hasInline)
			if err != nil {
				return inv, err
			}
			seen = append(seen, name)
			inv.flags.socket = raw
		case "--text":
			raw, err := value(&i, name, inline, hasInline)
			if err != nil {
				return inv, err
			}
			seen = append(seen, name)
			inv.body.text = raw
		case "--file":
			raw, err := value(&i, name, inline, hasInline)
			if err != nil {
				return inv, err
			}
			seen = append(seen, name)
			inv.body.file = raw
		case "--page":
			raw, err := value(&i, name, inline, hasInline)
			if err != nil {
				return inv, err
			}
			seen = append(seen, name)
			if !settingsPages[raw] {
				return inv, usagef("Invalid value for --page: %q", raw)
			}
			inv.page = raw
		default:
			return inv, usagef("Unknown option %q.", name)
		}
	}
	if len(positional) == 0 {
		return inv, usagef("Missing command. See \"cubbix help\".")
	}
	if err := resolve(&inv, positional); err != nil {
		return inv, err
	}
	if err := checkOptions(inv.command, seen); err != nil {
		return inv, err
	}
	if inv.command == "awake.on" {
		chosen := 0
		for _, used := range []bool{inv.awake.hasMin, inv.awake.until != "", inv.awake.follow > 0} {
			if used {
				chosen++
			}
		}
		if chosen > 1 {
			return inv, usagef("--minutes, --until and --follow are mutually exclusive.")
		}
		if seenContains(seen, "--lid") && seenContains(seen, "--no-lid") {
			return inv, usagef("--lid and --no-lid are mutually exclusive.")
		}
		inv.awake.arg = normalizeAwake(inv.awake)
	}
	return inv, nil
}

func seenContains(seen []string, name string) bool {
	for _, item := range seen {
		if item == name {
			return true
		}
	}
	return false
}

// resolve names the command from the positional words.
func resolve(inv *invocation, positional []string) error {
	word := positional[0]
	rest := positional[1:]
	take := func(count int) error {
		if len(rest) != count {
			return usagef("Unknown command %q. See \"cubbix help\".", strings.Join(positional, " "))
		}
		return nil
	}
	switch word {
	case "awake":
		if len(rest) == 0 {
			return usagef("Missing awake command. See \"cubbix help\".")
		}
		switch rest[0] {
		case "on":
			if err := take(1); err != nil {
				return err
			}
			inv.command = "awake.on"
		case "off":
			if err := take(1); err != nil {
				return err
			}
			inv.command = "awake.off"
		case "toggle":
			if err := take(1); err != nil {
				return err
			}
			inv.command = "awake.toggle"
		case "status":
			if err := take(1); err != nil {
				return err
			}
			inv.command = "awake.status"
		default:
			return usagef("Unknown command %q. See \"cubbix help\".", strings.Join(positional, " "))
		}
	case "json":
		if len(rest) == 0 || rest[0] != "new" {
			return usagef("Unknown command %q. See \"cubbix help\".", strings.Join(positional, " "))
		}
		if err := take(1); err != nil {
			return err
		}
		inv.command = "json.new"
	case "status":
		if err := take(0); err != nil {
			return err
		}
		inv.command = "status"
	case "palette":
		if err := take(0); err != nil {
			return err
		}
		inv.command = "palette"
	case "settings":
		if err := take(0); err != nil {
			return err
		}
		inv.command = "settings"
	case "version":
		if err := take(0); err != nil {
			return err
		}
		inv.command = "version"
	case "help":
		if len(rest) > 1 {
			return usagef("Unknown command %q. See \"cubbix help\".", strings.Join(positional, " "))
		}
		inv.command = "help"
	default:
		return usagef("Unknown command %q. See \"cubbix help\".", strings.Join(positional, " "))
	}
	return nil
}

// checkOptions rejects options the chosen command does not take: accepting them
// silently would let a script believe they took effect.
func checkOptions(command string, seen []string) error {
	global := map[string]bool{"--json": true, "--notify": true, "--no-launch": true, "--timeout": true, "--socket": true}
	awakeOnly := map[string]bool{"--minutes": true, "--until": true, "--follow": true, "--mode": true, "--lid": true, "--no-lid": true}
	bodyOnly := map[string]bool{"--text": true, "--file": true, "--blank": true}
	allowed := map[string]bool{}
	for name := range global {
		allowed[name] = true
	}
	allow := func(names map[string]bool) {
		for name := range names {
			allowed[name] = true
		}
	}
	switch command {
	case "awake.on":
		allow(awakeOnly)
	case "json.new":
		allow(bodyOnly)
	case "settings":
		allowed["--page"] = true
	}
	// A read-only command never carries --notify: nothing would show.
	if strings.HasPrefix(command, "awake.status") || command == "status" || command == "version" || command == "help" {
		delete(allowed, "--notify")
	}
	if command == "help" || command == "version" {
		delete(allowed, "--json")
	}
	for _, name := range seen {
		if name == "-h" || name == "--help" {
			continue
		}
		if !allowed[name] {
			if command == "awake.off" || command == "awake.toggle" {
				if awakeOnly[name] {
					return usagef("%s does not take %s: it uses the remembered session.", commandWord(command), name)
				}
			}
			return usagef("Unknown option %q.", name)
		}
	}
	return nil
}

func commandWord(command string) string {
	return strings.ReplaceAll(command, ".", " ")
}

// normalizeAwake turns the options into the shared argument grammar: the app
// parses it with the very parser the command palette uses.
func normalizeAwake(options awakeOptions) string {
	if options.follow > 0 {
		return ""
	}
	if options.until != "" {
		return "until " + options.until
	}
	if options.hasMin {
		return formatMinutes(options.minutes)
	}
	return ""
}

func formatMinutes(minutes int) string {
	if minutes == 0 {
		return "∞"
	}
	hours, rest := minutes/60, minutes%60
	switch {
	case hours > 0 && rest > 0:
		return fmt.Sprintf("%dh%dm", hours, rest)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dm", minutes)
}

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const (
	configVersion      = 1
	paletteRecentLimit = 5
	defaultPaletteKey  = "Alt+."
	defaultPanelKey    = "Ctrl+Alt+Cmd+P"
)

// normalizeCommandIDs drops blanks and duplicates while keeping the order.
func normalizeCommandIDs(ids []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func normalizePaletteFavorites(ids []string) []string {
	return normalizeCommandIDs(ids)
}

func normalizePaletteRecents(ids []string) []string {
	out := normalizeCommandIDs(ids)
	if len(out) > paletteRecentLimit {
		out = out[:paletteRecentLimit]
	}
	return out
}

type Config struct {
	AwakeLid           bool            `toml:"awake_lid"`
	AwakeLidBatteryMin int             `toml:"awake_lid_battery_min"`
	JSONEnabled        bool            `toml:"json_enabled"`
	JSONShortcut       string          `toml:"json_shortcut"`
	JSONWidth          int             `toml:"json_width,omitempty"`
	JSONHeight         int             `toml:"json_height,omitempty"`
	JSONSplit          int             `toml:"json_split,omitempty"`
	Version            int             `toml:"version"`
	Language           string          `toml:"language"`
	OpenAtLogin        bool            `toml:"open_at_login"`
	FeedbackMode       string          `toml:"feedback_mode"`
	FeedbackCorner     string          `toml:"feedback_corner"`
	AutomaticUpdates   bool            `toml:"automatic_updates"`
	AwakeEnabled       bool            `toml:"awake_enabled"`
	AwakeMode          string          `toml:"awake_mode"`
	AwakeMinutes       int             `toml:"awake_minutes"`
	AwakeUntil         string          `toml:"awake_until,omitempty"`
	AwakeAgents        string          `toml:"awake_agents"`
	AwakeShortcut      string          `toml:"awake_shortcut"`
	PaletteShortcut    string          `toml:"palette_shortcut"`
	PanelShortcut      string          `toml:"panel_shortcut"`
	PalettePositions   map[string]seat `toml:"palette_positions,omitempty"`
	PaletteFavorites   []string        `toml:"palette_favorites,omitempty"`
	PaletteRecents     []string        `toml:"palette_recents,omitempty"`
	AwakeSession       *awakeSession   `toml:"awake_session,omitempty"`
	RestoreAwake       bool            `toml:"restore_awake,omitempty"`
	WindowWidth        int             `toml:"window_width,omitempty"`
	WindowHeight       int             `toml:"window_height,omitempty"`
	// UpdateLastCheck is the UTC RFC3339Nano time of the last finished update
	// check. It is optional: an older config simply has none.
	UpdateLastCheck string `toml:"update_last_check,omitempty"`
}

func defaultConfig() Config {
	return Config{AwakeLidBatteryMin: 20, Version: configVersion, Language: "zh-CN", AutomaticUpdates: true, AwakeEnabled: true, JSONEnabled: true, JSONSplit: jsonSplitDefault, JSONShortcut: "Ctrl+Alt+Cmd+J", AwakeMode: "screen", AwakeMinutes: 60, AwakeAgents: "codex,claude,gemini,aider", AwakeShortcut: "Ctrl+Alt+Cmd+K", PaletteShortcut: defaultPaletteKey, PanelShortcut: defaultPanelKey, FeedbackMode: "hint", FeedbackCorner: "top-right"}
}

func loadConfig(path string) (Config, error) {
	config := defaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return Config{}, err
	}
	config.Version = 0
	if err := toml.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if config.Version != configVersion {
		return Config{}, fmt.Errorf("unsupported config version: %d", config.Version)
	}
	switch config.Language {
	case "system", "zh-CN", "en":
	default:
		return Config{}, fmt.Errorf("unsupported language: %q", config.Language)
	}
	// Older configs lack the feedback keys. Fill them from the defaults, but
	// reject an edit that names a mode or corner the app cannot draw.
	if config.FeedbackMode == "" {
		config.FeedbackMode = "hint"
	}
	switch config.FeedbackMode {
	case "hint", "notification", "off":
	default:
		return Config{}, fmt.Errorf("unsupported feedback mode: %q", config.FeedbackMode)
	}
	if config.FeedbackCorner == "" {
		config.FeedbackCorner = "top-right"
	}
	switch config.FeedbackCorner {
	case "top-left", "top-right", "bottom-left", "bottom-right", "center":
	default:
		return Config{}, fmt.Errorf("unsupported feedback corner: %q", config.FeedbackCorner)
	}
	if config.JSONSplit == 0 {
		config.JSONSplit = jsonSplitDefault
	}
	config.JSONSplit = clampJSONSplit(config.JSONSplit)
	if config.PaletteShortcut == "" {
		config.PaletteShortcut = defaultPaletteKey
	}
	if config.PanelShortcut == "" {
		config.PanelShortcut = defaultPanelKey
	}
	if config.AwakeLidBatteryMin != 10 && config.AwakeLidBatteryMin != 20 && config.AwakeLidBatteryMin != 30 {
		config.AwakeLidBatteryMin = 20
	}
	config.UpdateLastCheck = normalizeCheckedAt(config.UpdateLastCheck, time.Now())
	config.PaletteFavorites = normalizePaletteFavorites(config.PaletteFavorites)
	config.PaletteRecents = normalizePaletteRecents(config.PaletteRecents)
	return config, nil
}

// normalizeCheckedAt accepts only a plausible completion time. An invalid,
// pre-epoch or future value is dropped in memory; the file is left untouched
// and the next save rewrites it.
func normalizeCheckedAt(value string, now time.Time) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Before(time.Unix(0, 0)) || parsed.After(now) {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339Nano)
}

// putPaletteSeat remembers where the palette sits on a display, by display
// key. It is the string-keyed form of the shared seat table.
func (c *Config) putPaletteSeat(key string, value seat) {
	if c.PalettePositions == nil {
		c.PalettePositions = map[string]seat{}
	}
	c.PalettePositions[key] = value
}

func saveConfig(path string, config Config) (err error) {
	data, err := toml.Marshal(config)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".config-*.toml")
	if err != nil {
		return err
	}
	defer func() {
		file.Close()
		os.Remove(file.Name())
	}()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

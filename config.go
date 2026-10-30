package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	configVersion      = 1
	paletteRecentLimit = 5
	defaultPaletteKey  = "Alt+."
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
	JSONEnabled      bool          `toml:"json_enabled"`
	JSONShortcut     string        `toml:"json_shortcut"`
	JSONWidth        int           `toml:"json_width,omitempty"`
	JSONHeight       int           `toml:"json_height,omitempty"`
	JSONSplit        int           `toml:"json_split,omitempty"`
	Version          int           `toml:"version"`
	Language         string        `toml:"language"`
	OpenAtLogin      bool          `toml:"open_at_login"`
	FeedbackMode     string        `toml:"feedback_mode"`
	FeedbackCorner   string        `toml:"feedback_corner"`
	AutomaticUpdates bool          `toml:"automatic_updates"`
	AwakeEnabled     bool          `toml:"awake_enabled"`
	AwakeMode        string        `toml:"awake_mode"`
	AwakeMinutes     int           `toml:"awake_minutes"`
	AwakeUntil       string        `toml:"awake_until,omitempty"`
	AwakeAgents      string        `toml:"awake_agents"`
	AwakeShortcut    string        `toml:"awake_shortcut"`
	PaletteShortcut  string        `toml:"palette_shortcut"`
	PaletteFavorites []string      `toml:"palette_favorites,omitempty"`
	PaletteRecents   []string      `toml:"palette_recents,omitempty"`
	AwakeSession     *awakeSession `toml:"awake_session,omitempty"`
	RestoreAwake     bool          `toml:"restore_awake,omitempty"`
	WindowWidth      int           `toml:"window_width,omitempty"`
	WindowHeight     int           `toml:"window_height,omitempty"`
}

func defaultConfig() Config {
	return Config{Version: configVersion, Language: "zh-CN", AutomaticUpdates: true, AwakeEnabled: true, JSONEnabled: true, JSONSplit: jsonSplitDefault, JSONShortcut: "Ctrl+Alt+Cmd+J", AwakeMode: "screen", AwakeMinutes: 60, AwakeAgents: "codex,claude,gemini,aider", AwakeShortcut: "Ctrl+Alt+Cmd+K", PaletteShortcut: defaultPaletteKey, FeedbackMode: "hint", FeedbackCorner: "top-right"}
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
	config.PaletteFavorites = normalizePaletteFavorites(config.PaletteFavorites)
	config.PaletteRecents = normalizePaletteRecents(config.PaletteRecents)
	return config, nil
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

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const configVersion = 1

type Config struct {
	Version          int           `toml:"version"`
	Language         string        `toml:"language"`
	OpenAtLogin      bool          `toml:"open_at_login"`
	AutomaticUpdates bool          `toml:"automatic_updates"`
	AwakeEnabled     bool          `toml:"awake_enabled"`
	AwakeMode        string        `toml:"awake_mode"`
	AwakeMinutes     int           `toml:"awake_minutes"`
	AwakeUntil       string        `toml:"awake_until,omitempty"`
	AwakeShortcut    string        `toml:"awake_shortcut"`
	AwakeSession     *awakeSession `toml:"awake_session,omitempty"`
	RestoreAwake     bool          `toml:"restore_awake,omitempty"`
	WindowWidth      int           `toml:"window_width,omitempty"`
	WindowHeight     int           `toml:"window_height,omitempty"`
}

func defaultConfig() Config {
	return Config{Version: configVersion, Language: "zh-CN", AutomaticUpdates: true, AwakeEnabled: true, AwakeMode: "screen", AwakeMinutes: 60, AwakeShortcut: "Ctrl+Alt+Cmd+K"}
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

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewConfigEnablesAutomaticUpdates(t *testing.T) {
	config, err := loadConfig(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil || !config.AutomaticUpdates {
		t.Fatalf("new config = %#v, %v; automatic updates should be enabled", config, err)
	}
}

func TestSavedAutomaticUpdatesOffIsPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	config := defaultConfig()
	config.AutomaticUpdates = false
	if err := saveConfig(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil || loaded.AutomaticUpdates {
		t.Fatalf("saved config = %#v, %v; automatic updates should remain off", loaded, err)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cubbix", "config.toml")
	initial, err := loadConfig(path)
	if err != nil || initial != defaultConfig() {
		t.Fatalf("default config = %#v, %v", initial, err)
	}
	want := Config{Version: configVersion, Language: "en", OpenAtLogin: true, AutomaticUpdates: true}
	if err := saveConfig(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig(path)
	if err != nil || got != want {
		t.Fatalf("loaded = %#v, %v; want %#v", got, err, want)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("config permissions: %v, %v", info, err)
	}
}

func TestConfigRejectsUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("version = 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected unsupported version error")
	}
}

func TestConfigRejectsMissingVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("language = 'en'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected missing version error")
	}
}

func TestWindowSizePersistsAndOldConfigStillLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("version=1\nlanguage='zh-CN'\nautomatic_updates=false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	w, h := settingsSize(c.WindowWidth, c.WindowHeight)
	if w != 720 || h != 340 || c.AutomaticUpdates {
		t.Fatalf("old config: %+v", c)
	}
	c.WindowWidth, c.WindowHeight = 1000, 600
	if err := saveConfig(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig(path)
	if err != nil || got != c {
		t.Fatalf("saved window size: %+v %v", got, err)
	}
}

func TestJSONConfigDefaultsAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("version=1\nlanguage='zh-CN'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil || !c.JSONEnabled || c.JSONShortcut != "Ctrl+Alt+Cmd+J" {
		t.Fatalf("old config JSON defaults: %+v %v", c, err)
	}
	c.JSONEnabled = false
	c.JSONShortcut = "Ctrl+Alt+Cmd+M"
	c.JSONWidth = 1000
	c.JSONHeight = 600
	if err := saveConfig(path, c); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil || loaded != c {
		t.Fatalf("JSON config not persisted: %+v %v", loaded, err)
	}
}

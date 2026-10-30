package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	if err != nil || !reflect.DeepEqual(initial, defaultConfig()) {
		t.Fatalf("default config = %#v, %v", initial, err)
	}
	want := Config{Version: configVersion, Language: "en", OpenAtLogin: true, FeedbackMode: "hint", FeedbackCorner: "top-right", AutomaticUpdates: true, JSONSplit: jsonSplitDefault, PaletteShortcut: defaultPaletteKey}
	if err := saveConfig(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded = %#v, %v; want %#v", got, err, want)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("config permissions: %v, %v", info, err)
	}
}

func TestConfigDefaultsAndClampsSplit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	legacy := "version = 1\nlanguage = \"zh-CN\"\n"
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig(path)
	if err != nil || config.JSONSplit != jsonSplitDefault {
		t.Fatalf("legacy config split = %d, %v", config.JSONSplit, err)
	}
	for value, expected := range map[int]int{0: jsonSplitDefault, 10: jsonSplitMinimum, 95: jsonSplitMaximum, 45: 45} {
		raw := fmt.Sprintf("version = 1\nlanguage = \"zh-CN\"\njson_split = %d\n", value)
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		config, err := loadConfig(path)
		if err != nil || config.JSONSplit != expected {
			t.Fatalf("json_split = %d loaded as %d, %v; want %d", value, config.JSONSplit, err, expected)
		}
	}
}

func TestFeedbackConfigDefaultsAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	// A config written before the feedback settings still gets the defaults.
	if err := os.WriteFile(path, []byte("version = 1\nlanguage = \"zh-CN\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig(path)
	if err != nil || config.FeedbackMode != "hint" || config.FeedbackCorner != "top-right" {
		t.Fatalf("legacy feedback defaults = %q/%q, %v", config.FeedbackMode, config.FeedbackCorner, err)
	}
	for _, mode := range feedbackModes {
		for _, corner := range feedbackCorners {
			raw := fmt.Sprintf("version = 1\nlanguage = 'zh-CN'\nfeedback_mode = %q\nfeedback_corner = %q\n", mode, corner)
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			config, err := loadConfig(path)
			if err != nil || config.FeedbackMode != mode || config.FeedbackCorner != corner {
				t.Fatalf("feedback %q/%q = %q/%q, %v", mode, corner, config.FeedbackMode, config.FeedbackCorner, err)
			}
		}
	}
	for _, raw := range []string{
		"version = 1\nlanguage = 'zh-CN'\nfeedback_mode = 'toast'\n",
		"version = 1\nlanguage = 'zh-CN'\nfeedback_corner = 'middle'\n",
	} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadConfig(path); err == nil {
			t.Fatalf("invalid feedback setting accepted: %q", raw)
		}
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
	if err != nil || !reflect.DeepEqual(got, c) {
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
	if err != nil || !reflect.DeepEqual(loaded, c) {
		t.Fatalf("JSON config not persisted: %+v %v", loaded, err)
	}
}

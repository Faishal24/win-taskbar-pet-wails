package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Tiny JSON config in %APPDATA% (foundation for a future Settings window).

type config struct {
	RoamAll bool `json:"roam_all"`
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "win-taskbar-pet", "config.json")
}

func defaultConfig() config {
	return config{RoamAll: true}
}

func loadConfig() config {
	cfg := defaultConfig()
	p := configPath()
	if p == "" {
		return cfg
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultConfig()
	}
	return cfg
}

func saveConfig(cfg config) error {
	p := configPath()
	if p == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

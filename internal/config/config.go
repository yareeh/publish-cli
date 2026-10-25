// Package config keeps non-secret settings at
// ~/.config/publish-cli/config.json. Secrets live in the keychain.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	SubstackPublication string `json:"substack_publication,omitempty"` // e.g. https://name.substack.com
	SubstackUserID      int    `json:"substack_user_id,omitempty"`
}

func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "publish-cli")
}

func Path() string {
	return filepath.Join(Dir(), "config.json")
}

func Load() (*Config, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

func Save(cfg *Config) error {
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), data, 0600)
}

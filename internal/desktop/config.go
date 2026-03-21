package desktop

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type AppConfig struct {
	APIKey string `json:"api_key"`
}

func DefaultConfig() AppConfig {
	return AppConfig{APIKey: "quantix-dev-key"}
}

func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "QuantixConnector"), nil
}

func ConfigPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func LoadConfig() (AppConfig, error) {
	cfg := DefaultConfig()
	path, err := ConfigPath()
	if err != nil {
		return cfg, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return DefaultConfig(), err
	}
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	if cfg.APIKey == "" {
		cfg.APIKey = DefaultConfig().APIKey
	}
	return cfg, nil
}

func SaveConfig(cfg AppConfig) error {
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	if cfg.APIKey == "" {
		cfg.APIKey = DefaultConfig().APIKey
	}
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

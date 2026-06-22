package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	User  UserConfig  `json:"user"`
	Core  CoreConfig  `json:"core"`
}

type UserConfig struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type CoreConfig struct {
	DefaultBranch string `json:"default_branch"`
}

func DefaultConfig() *Config {
	return &Config{
		Core: CoreConfig{
			DefaultBranch: "main",
		},
	}
}

func LoadConfig(driftDir string) (*Config, error) {
	path := filepath.Join(driftDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func SaveConfig(driftDir string, cfg *Config) error {
	path := filepath.Join(driftDir, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}

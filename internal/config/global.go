package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GlobalConfig stores drift-wide settings that apply across all projects.
// It lives at ~/.drift/global.json so it survives project cloning.
type GlobalConfig struct {
	User GlobalUserConfig `json:"user,omitempty"`

	Protocol string `json:"protocol,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Path     string `json:"path,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	TLS      bool   `json:"tls,omitempty"`

	InsecureSkipVerify bool `json:"insecure_skip_verify,omitempty"`
	Share              string `json:"share,omitempty"`
	KeyPath            string `json:"key_path,omitempty"`
}

// GlobalUserConfig holds the default author identity stored in the global
// config (~/.drift/global.json). Project-level config takes precedence.
type GlobalUserConfig struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

var globalConfigPathOverride string

func globalConfigPath() (string, error) {
	if globalConfigPathOverride != "" {
		return globalConfigPathOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".drift", "global.json"), nil
}

// SetGlobalConfigPathForTest overrides the global config path. Pass an empty
// string to restore the default. Intended for testing only.
func SetGlobalConfigPathForTest(path string) {
	globalConfigPathOverride = path
}

// LoadGlobalConfig reads the global config, returning an empty config if
// the file does not exist yet.
func LoadGlobalConfig() (*GlobalConfig, error) {
	path, err := globalConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &GlobalConfig{}, nil
		}
		return nil, fmt.Errorf("cannot read global config: %w", err)
	}
	cfg := &GlobalConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid global config: %w", err)
	}
	return cfg, nil
}

// SaveGlobalConfig writes the global config atomically.
func SaveGlobalConfig(cfg *GlobalConfig) error {
	path, err := globalConfigPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// NewProjectID generates a random 16-byte hex project identifier.
// Called once at 'drift init' time.
func NewProjectID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", os.Getpid())
	}
	return hex.EncodeToString(b)
}

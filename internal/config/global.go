package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GlobalConfig stores drift-wide settings that apply across all projects.
// It lives at ~/.drift/global.json so it survives project cloning.
type GlobalConfig struct {
	User UserConfig `json:"user,omitempty"`

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

// UserConfig holds the default author identity used in both project-level
// (.drift/config.json) and global (~/.drift/global.json) configs.
type UserConfig struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

func globalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".drift", "global.json"), nil
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
	if strings.HasPrefix(cfg.Password, "$aes$") {
		pc, err := NewPasswordCrypto()
		if err != nil {
			fmt.Fprintf(os.Stderr, "drift: cannot initialize password crypto: %v\n", err)
			cfg.Password = ""
			return cfg, nil
		}
		plaintext, err := pc.DecryptPassword(cfg.Password)
		if err != nil {
			fmt.Fprintf(os.Stderr, "drift: cannot decrypt stored password, run 'drift remote setup' to re-enter it\n")
			cfg.Password = ""
		} else {
			cfg.Password = plaintext
		}
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
	password := cfg.Password
	if password != "" && !strings.HasPrefix(password, "$aes$") {
		pc, err := NewPasswordCrypto()
		if err != nil {
			return fmt.Errorf("cannot initialize password crypto: %w", err)
		}
		encrypted, err := pc.EncryptPassword(password)
		if err != nil {
			return fmt.Errorf("cannot encrypt password: %w", err)
		}
		password = encrypted
	}
	prev := cfg.Password
	cfg.Password = password
	data, err := json.MarshalIndent(cfg, "", "  ")
	cfg.Password = prev
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

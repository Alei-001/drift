package app

import (
	"fmt"
	"strconv"

	"github.com/drift/drift/internal/config"
	driftsync "github.com/drift/drift/internal/sync"
)

type ConfigScope string

const (
	LocalScope  ConfigScope = "local"
	GlobalScope ConfigScope = "global"
)

type ConfigEntry struct {
	Key   string
	Value string
}

func (a *App) ConfigGet(scope ConfigScope, key string) (string, error) {
	switch scope {
	case LocalScope:
		if !a.IsInitialized() {
			return "", fmt.Errorf("not a drift repository")
		}
		return getLocalConfigValue(a.config, key)
	case GlobalScope:
		gcfg, err := driftsync.LoadGlobalConfig()
		if err != nil {
			return "", err
		}
		return getGlobalConfigValue(gcfg, key)
	default:
		return "", fmt.Errorf("invalid config scope: %s", scope)
	}
}

func (a *App) ConfigSet(scope ConfigScope, key, value string) error {
	switch scope {
	case LocalScope:
		if !a.IsInitialized() {
			return fmt.Errorf("not a drift repository")
		}
		if err := setLocalConfigValue(a.config, key, value); err != nil {
			return err
		}
		return config.SaveConfig(a.store.DriftDir(), a.config)
	case GlobalScope:
		gcfg, err := driftsync.LoadGlobalConfig()
		if err != nil {
			return err
		}
		if err := setGlobalConfigValue(gcfg, key, value); err != nil {
			return err
		}
		return driftsync.SaveGlobalConfig(gcfg)
	default:
		return fmt.Errorf("invalid config scope: %s", scope)
	}
}

func (a *App) ConfigUnset(scope ConfigScope, key string) error {
	switch scope {
	case LocalScope:
		if !a.IsInitialized() {
			return fmt.Errorf("not a drift repository")
		}
		if err := unsetLocalConfigValue(a.config, key); err != nil {
			return err
		}
		return config.SaveConfig(a.store.DriftDir(), a.config)
	case GlobalScope:
		gcfg, err := driftsync.LoadGlobalConfig()
		if err != nil {
			return err
		}
		if err := unsetGlobalConfigValue(gcfg, key); err != nil {
			return err
		}
		return driftsync.SaveGlobalConfig(gcfg)
	default:
		return fmt.Errorf("invalid config scope: %s", scope)
	}
}

func (a *App) ConfigList(scope ConfigScope) ([]ConfigEntry, error) {
	switch scope {
	case LocalScope:
		if !a.IsInitialized() {
			return nil, fmt.Errorf("not a drift repository")
		}
		return listLocalConfig(a.config), nil
	case GlobalScope:
		gcfg, err := driftsync.LoadGlobalConfig()
		if err != nil {
			return nil, err
		}
		return listGlobalConfig(gcfg), nil
	default:
		return nil, fmt.Errorf("invalid config scope: %s", scope)
	}
}

func getLocalConfigValue(cfg *config.Config, key string) (string, error) {
	switch key {
	case "user.name":
		return cfg.User.Name, nil
	case "user.email":
		return cfg.User.Email, nil
	case "core.autocrlf":
		return cfg.Core.AutoCRLF, nil
	case "core.default_branch":
		return cfg.Core.DefaultBranch, nil
	case "sync.enabled":
		return strconv.FormatBool(cfg.Sync.Enabled), nil
	default:
		return "", fmt.Errorf("unknown config key: %s", key)
	}
}

func setLocalConfigValue(cfg *config.Config, key, value string) error {
	switch key {
	case "user.name":
		cfg.User.Name = value
	case "user.email":
		cfg.User.Email = value
	case "core.autocrlf":
		cfg.Core.AutoCRLF = value
	case "core.default_branch":
		cfg.Core.DefaultBranch = value
	case "sync.enabled":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean value for sync.enabled: %s", value)
		}
		cfg.Sync.Enabled = v
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

func unsetLocalConfigValue(cfg *config.Config, key string) error {
	switch key {
	case "user.name":
		cfg.User.Name = ""
	case "user.email":
		cfg.User.Email = ""
	case "core.autocrlf":
		cfg.Core.AutoCRLF = ""
	case "core.default_branch":
		cfg.Core.DefaultBranch = ""
	case "sync.enabled":
		cfg.Sync.Enabled = false
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

func listLocalConfig(cfg *config.Config) []ConfigEntry {
	var entries []ConfigEntry
	if cfg.User.Name != "" {
		entries = append(entries, ConfigEntry{Key: "user.name", Value: cfg.User.Name})
	}
	if cfg.User.Email != "" {
		entries = append(entries, ConfigEntry{Key: "user.email", Value: cfg.User.Email})
	}
	if cfg.Core.AutoCRLF != "" {
		entries = append(entries, ConfigEntry{Key: "core.autocrlf", Value: cfg.Core.AutoCRLF})
	}
	if cfg.Core.DefaultBranch != "" {
		entries = append(entries, ConfigEntry{Key: "core.default_branch", Value: cfg.Core.DefaultBranch})
	}
	entries = append(entries, ConfigEntry{Key: "sync.enabled", Value: strconv.FormatBool(cfg.Sync.Enabled)})
	return entries
}

func getGlobalConfigValue(gcfg *driftsync.GlobalConfig, key string) (string, error) {
	switch key {
	case "user.name":
		return gcfg.User.Name, nil
	case "user.email":
		return gcfg.User.Email, nil
	default:
		return "", fmt.Errorf("unknown global config key: %s", key)
	}
}

func setGlobalConfigValue(gcfg *driftsync.GlobalConfig, key, value string) error {
	switch key {
	case "user.name":
		gcfg.User.Name = value
	case "user.email":
		gcfg.User.Email = value
	default:
		return fmt.Errorf("unknown global config key: %s", key)
	}
	return nil
}

func unsetGlobalConfigValue(gcfg *driftsync.GlobalConfig, key string) error {
	switch key {
	case "user.name":
		gcfg.User.Name = ""
	case "user.email":
		gcfg.User.Email = ""
	default:
		return fmt.Errorf("unknown global config key: %s", key)
	}
	return nil
}

func listGlobalConfig(gcfg *driftsync.GlobalConfig) []ConfigEntry {
	var entries []ConfigEntry
	if gcfg.User.Name != "" {
		entries = append(entries, ConfigEntry{Key: "user.name", Value: gcfg.User.Name})
	}
	if gcfg.User.Email != "" {
		entries = append(entries, ConfigEntry{Key: "user.email", Value: gcfg.User.Email})
	}
	return entries
}


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
	return []ConfigEntry{
		{Key: "core.autocrlf", Value: cfg.Core.AutoCRLF},
		{Key: "core.default_branch", Value: cfg.Core.DefaultBranch},
		{Key: "sync.enabled", Value: strconv.FormatBool(cfg.Sync.Enabled)},
		{Key: "user.name", Value: cfg.User.Name},
		{Key: "user.email", Value: cfg.User.Email},
	}
}

func portStr(p int) string {
	if p == 0 {
		return ""
	}
	return strconv.Itoa(p)
}

func listGlobalConfig(gcfg *driftsync.GlobalConfig) []ConfigEntry {
	return []ConfigEntry{
		{Key: "remote.protocol", Value: gcfg.Protocol},
		{Key: "remote.host", Value: gcfg.Host},
		{Key: "remote.port", Value: portStr(gcfg.Port)},
		{Key: "remote.path", Value: gcfg.Path},
		{Key: "remote.username", Value: gcfg.Username},
		{Key: "remote.tls", Value: strconv.FormatBool(gcfg.TLS)},
		{Key: "remote.insecure_skip_verify", Value: strconv.FormatBool(gcfg.InsecureSkipVerify)},
		{Key: "remote.share", Value: gcfg.Share},
		{Key: "remote.key_path", Value: gcfg.KeyPath},
		{Key: "user.name", Value: gcfg.User.Name},
		{Key: "user.email", Value: gcfg.User.Email},
	}
}

func getGlobalConfigValue(gcfg *driftsync.GlobalConfig, key string) (string, error) {
	switch key {
	case "user.name":
		return gcfg.User.Name, nil
	case "user.email":
		return gcfg.User.Email, nil
	case "remote.protocol":
		return gcfg.Protocol, nil
	case "remote.host":
		return gcfg.Host, nil
	case "remote.port":
		if gcfg.Port != 0 {
			return strconv.Itoa(gcfg.Port), nil
		}
		return "", nil
	case "remote.path":
		return gcfg.Path, nil
	case "remote.username":
		return gcfg.Username, nil
	case "remote.tls":
		return strconv.FormatBool(gcfg.TLS), nil
	case "remote.insecure_skip_verify":
		return strconv.FormatBool(gcfg.InsecureSkipVerify), nil
	case "remote.share":
		return gcfg.Share, nil
	case "remote.key_path":
		return gcfg.KeyPath, nil
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
	case "remote.protocol":
		gcfg.Protocol = value
	case "remote.host":
		gcfg.Host = value
	case "remote.port":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer value for %s: %s", key, value)
		}
		gcfg.Port = v
	case "remote.path":
		gcfg.Path = value
	case "remote.username":
		gcfg.Username = value
	case "remote.tls":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean value for %s: %s", key, value)
		}
		gcfg.TLS = v
	case "remote.insecure_skip_verify":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean value for %s: %s", key, value)
		}
		gcfg.InsecureSkipVerify = v
	case "remote.share":
		gcfg.Share = value
	case "remote.key_path":
		gcfg.KeyPath = value
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
	case "remote.protocol":
		gcfg.Protocol = ""
	case "remote.host":
		gcfg.Host = ""
	case "remote.port":
		gcfg.Port = 0
	case "remote.path":
		gcfg.Path = ""
	case "remote.username":
		gcfg.Username = ""
	case "remote.tls":
		gcfg.TLS = false
	case "remote.insecure_skip_verify":
		gcfg.InsecureSkipVerify = false
	case "remote.share":
		gcfg.Share = ""
	case "remote.key_path":
		gcfg.KeyPath = ""
	default:
		return fmt.Errorf("unknown global config key: %s", key)
	}
	return nil
}


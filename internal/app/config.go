package app

import (
	"fmt"
	"strconv"

	"github.com/drift/drift/internal/config"
)

type ConfigScope string

const (
	LocalScope  ConfigScope = "local"
	GlobalScope ConfigScope = "global"
)

func (a *App) ConfigGet(scope ConfigScope, key string) (string, error) {
	switch scope {
	case LocalScope:
		if !a.IsInitialized() {
			return "", fmt.Errorf("not a drift repository")
		}
		return getLocalConfigValue(a.config, key)
	case GlobalScope:
		gcfg, err := config.LoadGlobalConfig()
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
		gcfg, err := config.LoadGlobalConfig()
		if err != nil {
			return err
		}
		if err := setGlobalConfigValue(gcfg, key, value); err != nil {
			return err
		}
		return config.SaveGlobalConfig(gcfg)
	default:
		return fmt.Errorf("invalid config scope: %s", scope)
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
		case "gc.auto":
			v := cfg.Core.GCAuto
			if v == 0 {
				v = 1000
			}
			return strconv.Itoa(v), nil
		case "gc.reflogExpire":
			v := cfg.Core.ReflogExpire
			if v == 0 {
				v = 90
			}
			return strconv.Itoa(v), nil
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
		case "gc.auto":
			v, err := strconv.Atoi(value)
			if err != nil || v < 0 {
				return fmt.Errorf("invalid integer value for gc.auto: %s", value)
			}
			cfg.Core.GCAuto = v
		case "gc.reflogExpire":
			v, err := strconv.Atoi(value)
			if err != nil || v < 0 {
				return fmt.Errorf("invalid integer value for gc.reflogExpire: %s", value)
			}
			cfg.Core.ReflogExpire = v
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}


func getGlobalConfigValue(gcfg *config.GlobalConfig, key string) (string, error) {
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

func setGlobalConfigValue(gcfg *config.GlobalConfig, key, value string) error {
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



package cli

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/drift/drift/internal/config"
	driftsync "github.com/drift/drift/internal/sync"
	"github.com/spf13/cobra"
)

var (
	configList  bool
	configUnset bool
)

var configCmd = &cobra.Command{
	Use:   "config <key> [value]",
	Short: "Get or set configuration options",
	Long: `Get or set configuration options.

Configuration is split between:
  - Global config (~/.drift/global.json): user identity, remote storage
  - Project config (.drift/config.json): per-project user override, sync, core

For user.name/user.email, the project config overrides the global config.
Use 'drift config --global user.name "Jane"' to set global user identity.

Examples:
  drift config user.name "Jane Doe"       # set per-project override
  drift config --global user.name "Jane"  # set global default
  drift config user.name                  # get effective value
  drift config --list                     # list all config
  drift config --unset user.email         # remove per-project override`,
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// --list: print all config values.
		if configList {
			return printConfigAll()
		}

		// --unset <key>: reset a config value to its default/empty.
		if configUnset {
			if len(args) == 0 {
				return fmt.Errorf("--unset requires a key")
			}
			if err := unsetConfigValue(args[0]); err != nil {
				return err
			}
			fmt.Printf("Unset %s\n", args[0])
			return nil
		}

		if len(args) == 0 {
			return cmd.Help()
		}

		key := args[0]

		if len(args) == 1 {
			// Get mode.
			val, err := getConfigValue(key)
			if err != nil {
				return err
			}
			fmt.Println(val)
			return nil
		}

		// Set mode.
		value := args[1]
		if err := setConfigValue(key, value); err != nil {
			return err
		}
		fmt.Printf("Set %s to %s\n", key, value)
		return nil
	},
}

var configGlobal bool

func init() {
	configCmd.Flags().BoolVar(&configList, "list", false, "List all configuration values")
	configCmd.Flags().BoolVar(&configUnset, "unset", false, "Remove a configuration value")
	configCmd.Flags().BoolVar(&configGlobal, "global", false, "Set/get/unset in global config (~/.drift/global.json)")
	rootCmd.AddCommand(configCmd)
}

// configKeyInfo pairs a config key with its current value.
type configKeyInfo struct {
	Key   string
	Value string
}

// printConfigAll prints all config entries from both global and project config.
func printConfigAll() error {
	var entries []configKeyInfo

	// Load global config for user identity and remote settings.
	gcfg, _ := driftsync.LoadGlobalConfig()

	// User identity: show global first, then project override if set.
	if gcfg.User.Name != "" {
		entries = append(entries, configKeyInfo{"user.name (global)", gcfg.User.Name})
	}
	if gcfg.User.Email != "" {
		entries = append(entries, configKeyInfo{"user.email (global)", gcfg.User.Email})
	}
	if sharedConfig.User.Name != "" && sharedConfig.User.Name != gcfg.User.Name {
		entries = append(entries, configKeyInfo{"user.name (project)", sharedConfig.User.Name})
	}
	if sharedConfig.User.Email != "" && sharedConfig.User.Email != gcfg.User.Email {
		entries = append(entries, configKeyInfo{"user.email (project)", sharedConfig.User.Email})
	}

	// Core settings (project config).
	entries = append(entries, configKeyInfo{"core.default_branch", sharedConfig.Core.DefaultBranch})
	if sharedConfig.Core.AutoCRLF != "" {
		entries = append(entries, configKeyInfo{"core.autocrlf", sharedConfig.Core.AutoCRLF})
	}

	// Sync settings (project config).
	if sharedConfig.Sync.Enabled {
		entries = append(entries, configKeyInfo{"sync.enabled", "true"})
	} else {
		entries = append(entries, configKeyInfo{"sync.enabled", "false"})
	}
	if sharedConfig.Sync.ProjectID != "" {
		entries = append(entries, configKeyInfo{"sync.project_id", sharedConfig.Sync.ProjectID})
	}
	if sharedConfig.Sync.RemoteName != "" {
		entries = append(entries, configKeyInfo{"sync.remote_name", sharedConfig.Sync.RemoteName})
	}
	if sharedConfig.Sync.LastSync != "" {
		entries = append(entries, configKeyInfo{"sync.last_sync", sharedConfig.Sync.LastSync})
	}

	// Remote settings (global config).
	if gcfg.Protocol != "" {
		entries = append(entries, configKeyInfo{"remote.protocol", gcfg.Protocol})
	}
	if gcfg.Host != "" {
		entries = append(entries, configKeyInfo{"remote.host", gcfg.Host})
	}
	if gcfg.Port != 0 {
		entries = append(entries, configKeyInfo{"remote.port", strconv.Itoa(gcfg.Port)})
	}
	if gcfg.Path != "" {
		entries = append(entries, configKeyInfo{"remote.path", gcfg.Path})
	}
	if gcfg.Username != "" {
		entries = append(entries, configKeyInfo{"remote.username", gcfg.Username})
	}
	if gcfg.TLS {
		entries = append(entries, configKeyInfo{"remote.tls", "true"})
	}
	if gcfg.InsecureSkipVerify {
		entries = append(entries, configKeyInfo{"remote.insecure", "true"})
	}
	if gcfg.Share != "" {
		entries = append(entries, configKeyInfo{"remote.share", gcfg.Share})
	}
	if gcfg.KeyPath != "" {
		entries = append(entries, configKeyInfo{"remote.key_path", gcfg.KeyPath})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})
	for _, e := range entries {
		fmt.Printf("%s=%s\n", e.Key, e.Value)
	}
	return nil
}

// getConfigValue returns the effective value for a config key.
// For user.name/user.email, project config overrides global.
func getConfigValue(key string) (string, error) {
	switch key {
	case "user.name":
		if sharedConfig.User.Name != "" {
			return sharedConfig.User.Name, nil
		}
		if gcfg, err := driftsync.LoadGlobalConfig(); err == nil {
			return gcfg.User.Name, nil
		}
		return "", nil
	case "user.email":
		if sharedConfig.User.Email != "" {
			return sharedConfig.User.Email, nil
		}
		if gcfg, err := driftsync.LoadGlobalConfig(); err == nil {
			return gcfg.User.Email, nil
		}
		return "", nil
	case "core.default_branch":
		return sharedConfig.Core.DefaultBranch, nil
	case "core.autocrlf":
		return sharedConfig.Core.AutoCRLF, nil
	case "sync.enabled":
		return strconv.FormatBool(sharedConfig.Sync.Enabled), nil
	case "sync.project_id":
		return sharedConfig.Sync.ProjectID, nil
	case "sync.remote_name":
		return sharedConfig.Sync.RemoteName, nil
	case "sync.last_sync":
		return sharedConfig.Sync.LastSync, nil
	case "remote.protocol", "remote.host", "remote.port", "remote.path",
		"remote.username", "remote.tls", "remote.insecure",
		"remote.share", "remote.key_path":
		return getRemoteConfigValue(key)
	default:
		return "", fmt.Errorf("unknown config key: %s", key)
	}
}

func getRemoteConfigValue(key string) (string, error) {
	gcfg, err := driftsync.LoadGlobalConfig()
	if err != nil {
		return "", err
	}
	switch key {
	case "remote.protocol":
		return gcfg.Protocol, nil
	case "remote.host":
		return gcfg.Host, nil
	case "remote.port":
		return strconv.Itoa(gcfg.Port), nil
	case "remote.path":
		return gcfg.Path, nil
	case "remote.username":
		return gcfg.Username, nil
	case "remote.tls":
		return strconv.FormatBool(gcfg.TLS), nil
	case "remote.insecure":
		return strconv.FormatBool(gcfg.InsecureSkipVerify), nil
	case "remote.share":
		return gcfg.Share, nil
	case "remote.key_path":
		return gcfg.KeyPath, nil
	default:
		return "", fmt.Errorf("unknown remote key: %s", key)
	}
}

// setConfigValue writes a config value. With --global, writes to global config
// for user.name/user.email. Remote settings should be managed via 'drift sync remote'.
func setConfigValue(key, value string) error {
	switch key {
	case "user.name", "user.email":
		if configGlobal {
			return setGlobalUserValue(key, value)
		}
		if key == "user.name" {
			sharedConfig.User.Name = value
		} else {
			sharedConfig.User.Email = value
		}
		return config.SaveConfig(sharedStore.DriftDir(), sharedConfig)
	case "core.default_branch":
		sharedConfig.Core.DefaultBranch = value
		return config.SaveConfig(sharedStore.DriftDir(), sharedConfig)
	case "core.autocrlf":
		sharedConfig.Core.AutoCRLF = value
		return config.SaveConfig(sharedStore.DriftDir(), sharedConfig)
	case "sync.enabled":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean value: %s", value)
		}
		sharedConfig.Sync.Enabled = v
		return config.SaveConfig(sharedStore.DriftDir(), sharedConfig)
	case "sync.remote_name":
		sharedConfig.Sync.RemoteName = value
		return config.SaveConfig(sharedStore.DriftDir(), sharedConfig)
	case "remote.protocol", "remote.host", "remote.port", "remote.path",
		"remote.username", "remote.tls", "remote.insecure",
		"remote.share", "remote.key_path":
		return fmt.Errorf("remote.* keys are managed by 'drift sync remote' — use that command instead")
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
}

func setGlobalUserValue(key, value string) error {
	gcfg, err := driftsync.LoadGlobalConfig()
	if err != nil {
		return err
	}
	if key == "user.name" {
		gcfg.User.Name = value
	} else {
		gcfg.User.Email = value
	}
	return driftsync.SaveGlobalConfig(gcfg)
}

// unsetConfigValue resets a config key to its empty/default value.
func unsetConfigValue(key string) error {
	switch key {
	case "user.name", "user.email":
		if configGlobal {
			gcfg, err := driftsync.LoadGlobalConfig()
			if err != nil {
				return err
			}
			if key == "user.name" {
				gcfg.User.Name = ""
			} else {
				gcfg.User.Email = ""
			}
			return driftsync.SaveGlobalConfig(gcfg)
		}
		if key == "user.name" {
			sharedConfig.User.Name = ""
		} else {
			sharedConfig.User.Email = ""
		}
		return config.SaveConfig(sharedStore.DriftDir(), sharedConfig)
	case "core.default_branch":
		sharedConfig.Core.DefaultBranch = "main"
		return config.SaveConfig(sharedStore.DriftDir(), sharedConfig)
	case "core.autocrlf":
		sharedConfig.Core.AutoCRLF = ""
		return config.SaveConfig(sharedStore.DriftDir(), sharedConfig)
	case "sync.enabled":
		sharedConfig.Sync.Enabled = false
		return config.SaveConfig(sharedStore.DriftDir(), sharedConfig)
	case "sync.remote_name":
		sharedConfig.Sync.RemoteName = ""
		return config.SaveConfig(sharedStore.DriftDir(), sharedConfig)
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
}

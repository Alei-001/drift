package cmd

import (
	"github.com/Alei-001/drift/internal/project"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Alei-001/drift/internal/core"
)

// configField describes a user-configurable key exposed by 'drift config'.
type configField struct {
	key string
	typ string // "string", "bool", "int"
}

// configFields lists every key 'drift config' recognizes, in display order.
// Only user-facing keys are exposed here; algorithm tuning parameters (chunk
// sizes, compression) are intentionally omitted — they are hardcoded in
// core.DefaultConfig and should not be tuned by end users. Unknown keys are
// rejected so users get a clear error rather than silently writing a no-op.
var configFields = []configField{
	{key: "user.name", typ: "string"},
	{key: "user.email", typ: "string"},
}

// configFieldMap indexes configFields by key for O(1) lookup.
var configFieldMap = func() map[string]configField {
	m := make(map[string]configField, len(configFields))
	for _, f := range configFields {
		m[f.key] = f
	}
	return m
}()

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage drift configuration",
	Long:  "View and modify drift configuration values (user.name, compression, chunk sizes).",
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration values",
	Long:  "List all configuration values recognized by drift.",
	RunE:  runConfigList,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Show a single configuration value",
	Long:  "Show the value of a single configuration key.",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long:  "Set a configuration key to a new value and persist it to the .drift/ config file.",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

func init() {
	configCmd.AddCommand(configListCmd, configGetCmd, configSetCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigList(cmd *cobra.Command, args []string) error {
	cwd, err := getCwd()
	if err != nil {
		return err
	}
	store, cfg, err := openProjectOrReport("Config", "config", cwd)
	if err != nil {
		return err
	}
	defer store.Close()

	if globalJSON {
		entries := make([]configJSONEntry, 0, len(configFields))
		for _, f := range configFields {
			val, ok := configGetValue(cfg, f.key)
			if !ok {
				continue
			}
			entries = append(entries, configJSONEntry{Key: f.key, Value: val})
		}
		return outputJSON(JSONEnvelope{
			Command: "config",
			Status:  "ok",
			Data:    configListJSONData{Values: entries},
		})
	}

	// Quiet mode: success produces no output (exit code is authoritative).
	if globalQuiet {
		return nil
	}

	// Compute the column width from the longest key so the '=' aligns.
	maxLen := 0
	for _, f := range configFields {
		if len(f.key) > maxLen {
			maxLen = len(f.key)
		}
	}
	format := fmt.Sprintf("  %%-%ds = %%s\n", maxLen)

	fmt.Println(">>> Config")
	for _, f := range configFields {
		val, ok := configGetValue(cfg, f.key)
		if !ok {
			continue
		}
		fmt.Printf(format, f.key, val)
	}
	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]
	if _, ok := configFieldMap[key]; !ok {
		reportFailed("Config", "config", fmt.Sprintf("unknown config key '%s'.", key), "use 'drift config list' to see available keys.", nil)
		return ErrSilent
	}
	cwd, err := getCwd()
	if err != nil {
		return err
	}
	store, cfg, err := openProjectOrReport("Config", "config", cwd)
	if err != nil {
		return err
	}
	defer store.Close()

	val, ok := configGetValue(cfg, key)
	if !ok {
		reportFailed("Config", "config", fmt.Sprintf("unknown config key '%s'.", key), "use 'drift config list' to see available keys.", nil)
		return ErrSilent
	}
	if globalJSON {
		return outputJSON(JSONEnvelope{
			Command: "config",
			Status:  "ok",
			Data:    configGetJSONData{Key: key, Value: val},
		})
	}
	if !globalQuiet {
		fmt.Printf(">>> Config: %s\n", key)
		fmt.Println(val)
	}
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key, value := args[0], args[1]
	field, ok := configFieldMap[key]
	if !ok {
		reportFailed("Config", "config", fmt.Sprintf("unknown config key '%s'.", key), "use 'drift config list' to see available keys.", nil)
		return ErrSilent
	}

	ctx := cmd.Context()
	cwd, err := getCwd()
	if err != nil {
		return err
	}
	store, cfg, err := openProjectOrReport("Config", "config", cwd)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := project.SetConfigValue(ctx, store, cfg, key, value); err != nil {
		reportFailed("Config", "config", err.Error(), "", err)
		return ErrSilent
	}

	if !globalQuiet {
		statusOK("Config updated")
		if field.typ == "string" {
			fmt.Printf("  %s = %q\n", key, value)
		} else {
			fmt.Printf("  %s = %s\n", key, value)
		}
	}
	return nil
}

// configGetValue returns the string representation of a config key, or
// ("" , false) if the key is not recognized.
func configGetValue(cfg *core.Config, key string) (string, bool) {
	switch key {
	case "user.name":
		return cfg.User.Name, true
	case "user.email":
		return cfg.User.Email, true
	default:
		return "", false
	}
}

// configJSONEntry is one key/value pair in 'drift config list --json'.
type configJSONEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// configListJSONData is the data payload of 'drift config list --json'.
type configListJSONData struct {
	Values []configJSONEntry `json:"values"`
}

// configGetJSONData is the data payload of 'drift config get --json'.
type configGetJSONData struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

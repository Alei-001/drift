package cli

import (
	"fmt"
	"strings"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewConfigCmd creates the config subcommand.
func NewConfigCmd(application *apppkg.App) *cobra.Command {
	var (
		global bool
		unset  string
	)

	cmd := &cobra.Command{
		Use:   "config [<key> [<value>]]",
		Short: "Get or set configuration options",
		RunE: func(cmd *cobra.Command, args []string) error {
			scope := apppkg.LocalScope
			if global {
				scope = apppkg.GlobalScope
			}

			// Unset config
			if unset != "" {
				if err := application.ConfigUnset(scope, unset); err != nil {
					return err
				}
				fmt.Printf("Unset %s\n", unset)
				return nil
			}

			// Get config (single arg, not "list")
			if len(args) == 1 && args[0] != "list" {
				value, err := application.ConfigGet(scope, args[0])
				if err != nil {
					return err
				}
				fmt.Println(value)
				return nil
			}

			// Set config
			if len(args) >= 2 {
				key := args[0]
				value := args[1]
				if err := application.ConfigSet(scope, key, value); err != nil {
					return err
				}
				fmt.Printf("Set %s = %s\n", key, value)
				return nil
			}

			// List all config: drift config, drift config list, drift config --global
			entries, err := application.ConfigList(scope)
			if err != nil {
				return err
			}

			formatConfigList(entries)
			return nil
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "Use global config")
	cmd.Flags().StringVar(&unset, "unset", "", "Unset a config option")

	return cmd
}

func formatConfigList(entries []apppkg.ConfigEntry) {
	sectionOrder := []string{"core", "sync", "user", "remote"}
	bySection := make(map[string][]apppkg.ConfigEntry)
	for _, e := range entries {
		sec, _, _ := strings.Cut(e.Key, ".")
		bySection[sec] = append(bySection[sec], e)
	}

	for _, sec := range sectionOrder {
		group, ok := bySection[sec]
		if !ok {
			continue
		}
		keyWidth := 0
		for _, e := range group {
			_, name, _ := strings.Cut(e.Key, ".")
			if len(name) > keyWidth {
				keyWidth = len(name)
			}
		}
		fmt.Printf("[%s]\n", sec)
		for _, e := range group {
			_, name, _ := strings.Cut(e.Key, ".")
			fmt.Printf("  %-*s = %s\n", keyWidth, name, e.Value)
		}
	}
}

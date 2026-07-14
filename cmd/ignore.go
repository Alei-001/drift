package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/Alei-001/drift/internal/util/fsutil"
	"github.com/spf13/cobra"
)

// ignoreCmd is the parent command for ignore-rule management. It has no
// RunE so cobra displays help when invoked without a subcommand.
var ignoreCmd = &cobra.Command{
	Use:   "ignore",
	Short: "Manage ignore rules (list, add, remove)",
	Long:  "Manage .driftignore rules. Subcommands: list, add, remove.",
}

// ignoreListCmd prints all current ignore rules.
var ignoreListCmd = &cobra.Command{
	Use:   "list",
	Short: "List current ignore rules",
	Long:  "List all patterns currently in .driftignore.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Ignore", "ignore", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		ignorePath := filepath.Join(cwd, ".driftignore")
		rules, err := fsutil.ListIgnoreRules(ignorePath)
		if err != nil {
			return err
		}
		if globalJSON {
			ruleList := rules
			if ruleList == nil {
				ruleList = []string{}
			}
			return outputJSON(JSONEnvelope{
				Command: "ignore",
				Status:  "ok",
				Data:    ignoreJSONData{Rules: ruleList},
			})
		}
		// Quiet mode: success produces no output (exit code is authoritative).
		if globalQuiet {
			return nil
		}
		fmt.Printf(">>> Ignore rules (%d)\n", len(rules))
		for _, r := range rules {
			fmt.Println(r)
		}
		return nil
	},
}

// ignoreAddCmd appends one or more patterns to .driftignore, skipping
// duplicates.
var ignoreAddCmd = &cobra.Command{
	Use:   "add <pattern>...",
	Short: "Add ignore rules",
	Long:  "Add one or more patterns to .driftignore. Duplicates are skipped.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Ignore", "ignore", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		ignorePath := filepath.Join(cwd, ".driftignore")
		newPatterns, err := fsutil.AddIgnoreRules(ignorePath, args)
		if err != nil {
			return err
		}
		if !globalQuiet {
			statusOK("Ignore updated")
			for _, p := range newPatterns {
				fmt.Printf("+ %s\n", p)
			}
			fmt.Printf("\n  %d rules added.\n", len(newPatterns))
		}
		return nil
	},
}

// ignoreRemoveCmd removes a single pattern from .driftignore.
var ignoreRemoveCmd = &cobra.Command{
	Use:   "remove <pattern>",
	Short: "Remove an ignore rule",
	Long:  "Remove a single pattern from .driftignore.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Ignore", "ignore", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		ignorePath := filepath.Join(cwd, ".driftignore")
		pattern := args[0]
		if err := fsutil.RemoveIgnoreRule(ignorePath, pattern); err != nil {
			reportFailed("Ignore", "ignore", fmt.Sprintf("pattern '%s' not found.", pattern), "use 'drift ignore list' to see current rules.", err)
			return ErrSilent
		}
		if !globalQuiet {
			statusOK("Ignore updated")
			fmt.Printf("- %s\n", pattern)
			fmt.Printf("\n  1 rule removed.\n")
		}
		return nil
	},
}

func init() {
	ignoreCmd.AddCommand(ignoreListCmd)
	ignoreCmd.AddCommand(ignoreAddCmd)
	ignoreCmd.AddCommand(ignoreRemoveCmd)
	rootCmd.AddCommand(ignoreCmd)
}

// ignoreJSONData is the data payload of the 'drift ignore list --json' envelope.
type ignoreJSONData struct {
	Rules []string `json:"rules"`
}

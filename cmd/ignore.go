package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/internal/util/fsutil"
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
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Ignore", "ignore", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		ignorePath := filepath.Join(cwd, ".driftignore")
		rules, err := listIgnoreRules(ignorePath)
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
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Ignore", "ignore", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		ignorePath := filepath.Join(cwd, ".driftignore")
		existing, err := readIgnoreFile(ignorePath)
		if err != nil {
			return err
		}
		existingSet := make(map[string]bool)
		for _, r := range existing {
			existingSet[r] = true
		}
		var newPatterns []string
		seen := make(map[string]bool)
		for _, p := range args {
			p = strings.TrimSpace(p)
			if p == "" || existingSet[p] || seen[p] {
				continue
			}
			seen[p] = true
			newPatterns = append(newPatterns, p)
		}

		n, err := addIgnoreRules(ignorePath, newPatterns)
		if err != nil {
			return err
		}
		if !globalQuiet {
			statusOK("Ignore updated")
			for _, p := range newPatterns {
				fmt.Printf("+ %s\n", p)
			}
			fmt.Printf("\n  %d rules added.\n", n)
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
		cwd, err := getCwd(cmd)
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
		if err := removeIgnoreRule(ignorePath, pattern); err != nil {
			reportFailed("Ignore", "ignore", fmt.Sprintf("pattern '%s' not found.", pattern), "use 'drift ignore list' to see current rules.")
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

// readIgnoreFile reads the patterns from the ignore file at path.
func readIgnoreFile(path string) ([]string, error) {
	return fsutil.ReadIgnoreFile(path)
}

// addIgnoreRules appends patterns to the ignore file at filePath, skipping
// duplicates. Returns the number of patterns actually added.
func addIgnoreRules(filePath string, patterns []string) (int, error) {
	data, err := os.ReadFile(filePath)
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	existing, _ := fsutil.ReadIgnoreFile(filePath)
	set := make(map[string]bool)
	for _, r := range existing {
		set[r] = true
	}
	var added []string
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" || set[p] {
			continue
		}
		set[p] = true
		added = append(added, p)
	}
	var buf strings.Builder
	buf.Write(data)
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		buf.WriteByte('\n')
	}
	for _, p := range added {
		buf.WriteString(p)
		buf.WriteByte('\n')
	}
	if err := fsutil.WriteFileAtomic(filePath, []byte(buf.String()), 0644); err != nil {
		return 0, err
	}
	return len(added), nil
}

// listIgnoreRules reads the patterns from the ignore file at filePath.
func listIgnoreRules(filePath string) ([]string, error) {
	return readIgnoreFile(filePath)
}

// removeIgnoreRule removes the first occurrence of pattern from the ignore
// file at filePath. Returns an error if the file or pattern is not found.
func removeIgnoreRule(filePath string, pattern string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("pattern '%s' not found", pattern)
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	found := false
	var out []string
	for _, line := range lines {
		if strings.TrimSpace(line) == pattern {
			found = true
			continue
		}
		out = append(out, line)
	}
	if !found {
		return fmt.Errorf("pattern '%s' not found", pattern)
	}
	return fsutil.WriteFileAtomic(filePath, []byte(strings.Join(out, "\n")), 0644)
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

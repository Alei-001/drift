package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/util/fsutil"
)

var ignoreList bool
var ignoreRemove string

var ignoreCmd = &cobra.Command{
	Use:   "ignore",
	Short: "Manage ignore rules",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		store, _, err := porcelain.OpenProject(cwd)
		if err != nil {
			statusFailed("Ignore", ".drift/ directory not found.", "run 'drift init' first.")
			return nil
		}
		defer store.Close()
		ignorePath := filepath.Join(cwd, ".driftignore")

		if ignoreList {
			rules, err := listIgnoreRules(ignorePath)
			if err != nil {
				return err
			}
			fmt.Printf(">>> Ignore rules (%d)\n", len(rules))
			for _, r := range rules {
				fmt.Println(r)
			}
			return nil
		}

		if ignoreRemove != "" {
			if err := removeIgnoreRule(ignorePath, ignoreRemove); err != nil {
				statusFailed("Ignore", fmt.Sprintf("pattern '%s' not found.", ignoreRemove), "use 'drift ignore --list' to see current rules.")
				return nil
			}
			statusOK("Ignore updated")
			fmt.Printf("- %s\n", ignoreRemove)
			fmt.Printf("\n  1 rule removed.\n")
			return nil
		}

		if len(args) == 0 {
			return cmd.Help()
		}

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
		statusOK("Ignore updated")
		for _, p := range newPatterns {
			fmt.Printf("+ %s\n", p)
		}
		fmt.Printf("\n  %d rules added.\n", n)
		return nil
	},
}

func readIgnoreFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rules []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		rules = append(rules, trimmed)
	}
	return rules, nil
}

func addIgnoreRules(filePath string, patterns []string) (int, error) {
	data, err := os.ReadFile(filePath)
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	set := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		set[trimmed] = true
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

func listIgnoreRules(filePath string) ([]string, error) {
	return readIgnoreFile(filePath)
}

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
	ignoreCmd.Flags().BoolVar(&ignoreList, "list", false, "list ignore rules")
	ignoreCmd.Flags().StringVar(&ignoreRemove, "remove", "", "remove a rule")
	rootCmd.AddCommand(ignoreCmd)
}

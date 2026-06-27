package app

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) AddIgnorePattern(pattern string) error {
	path := filepath.Join(a.dir, ".driftignore")

	f, err := os.Open(path)
	if err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == pattern {
				f.Close()
				return fmt.Errorf("pattern %q already in .driftignore", pattern)
			}
		}
		if scanErr := scanner.Err(); scanErr != nil {
			f.Close()
			return fmt.Errorf("failed to read .driftignore: %w", scanErr)
		}
		f.Close()
	}

	f, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(pattern + "\n"); err != nil {
		return err
	}

	fmt.Printf("Added %q to .driftignore\n", pattern)
	return nil
}

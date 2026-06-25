package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	apppkg "github.com/drift/drift/internal/app"
	driftsync "github.com/drift/drift/internal/sync"
	"github.com/spf13/cobra"
)

// NewInitCmd creates the init subcommand.
func NewInitCmd(application *apppkg.App) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a Drift project",
		RunE: func(cmd *cobra.Command, args []string) error {
			if application.IsInitialized() {
				fmt.Println("Drift project already exists")
				return nil
			}

			if err := application.Init(); err != nil {
				return fmt.Errorf("init failed: %w", err)
			}

			fmt.Println("Drift project initialized")

			// Load existing global config (may already have user info).
			gcfg, _ := driftsync.LoadGlobalConfig()

			// If global config already has a user name, use it as default.
			defaultName := gcfg.User.Name
			defaultEmail := gcfg.User.Email
			name, email := promptUserInfoNew(defaultName, defaultEmail)

			// Save user info to global config so all projects inherit it.
			// Only update if the user provided new values.
			if name != "" {
				gcfg.User.Name = name
			}
			if email != "" {
				gcfg.User.Email = email
			}
			if name != "" || email != "" {
				if err := driftsync.SaveGlobalConfig(gcfg); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to save global config: %v\n", err)
				} else {
					fmt.Println("Saved your name and email globally for all projects.")
				}
			}

			fmt.Println("\nNext steps:")
			fmt.Println("  drift add .       # stage your files")
			fmt.Println("  drift save -m \"first version\"")
			fmt.Println("  drift history --all   # view history")
			return nil
		},
	}
}

// promptUserInfoNew asks the user for their name and email via stdin.
// If defaults are provided, they are shown in the prompt. Returns the
// defaults (if user presses Enter) or the entered values. Returns empty
// strings if stdin is not interactive.
func promptUserInfoNew(defaultName, defaultEmail string) (name, email string) {
	info, err := os.Stdin.Stat()
	if err != nil || (info.Mode()&os.ModeCharDevice) == 0 {
		// Non-interactive stdin (e.g. piped input).
		return "", ""
	}

	reader := bufio.NewReader(os.Stdin)

	if defaultName != "" {
		fmt.Printf("Your name [%s]: ", defaultName)
	} else {
		fmt.Print("Your name: ")
	}
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		name = input
	} else {
		name = defaultName
	}

	if defaultEmail != "" {
		fmt.Printf("Your email [%s]: ", defaultEmail)
	} else {
		fmt.Print("Your email: ")
	}
	input, _ = reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		email = input
	} else {
		email = defaultEmail
	}

	return name, email
}

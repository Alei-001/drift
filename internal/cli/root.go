package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/repo"
	"github.com/drift/drift/internal/storage"
	driftsync "github.com/drift/drift/internal/sync"
	"github.com/spf13/cobra"
)

var (
	sharedStore  *storage.Store
	sharedConfig *config.Config
	sharedDir    string
	sharedRepo   *repo.Repository

	// Global persistent flags.
	globalVerbose  bool
	globalQuiet    bool
	globalDryRun   bool
	globalNoColor  bool
	globalRepoPath string
)

var rootCmd = &cobra.Command{
	Use:   "drift",
	Short: "Drift - A lightweight version control tool for creative workers",
	Long:  "Drift lets creative workers manage their work like developers manage code.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Handle -C <path>: change directory before any other logic so
		// downstream code can rely on os.Getwd() pointing at the target.
		if globalRepoPath != "" {
			if err := os.Chdir(globalRepoPath); err != nil {
				return fmt.Errorf("cannot change to %q: %w", globalRepoPath, err)
			}
		}

		// Commands that don't require an initialized project.
		switch cmd.Name() {
		case "init", "help", "version", "clone":
			return nil
		}
		// 'sync remote' manages global config and doesn't need a project.
		if cmd.Name() == "remote" && cmd.Parent() != nil && cmd.Parent().Name() == "sync" {
			return nil
		}

		helpFlag, _ := cmd.Flags().GetBool("help")
		if helpFlag {
			return nil
		}

		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		sharedDir = dir

		sharedStore = storage.NewStore(dir)
		if !sharedStore.IsInitialized() {
			return fmt.Errorf("not a drift project (run 'drift init')")
		}

		cfg, err := config.LoadConfig(sharedStore.DriftDir())
		if err != nil {
			if !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
			}
			sharedConfig = config.DefaultConfig()
		} else {
			sharedConfig = cfg
		}

		sharedRepo = repo.New(sharedStore, sharedConfig, dir)

		// Load global config for default user identity (project config
		// takes precedence when set).
		if gcfg, err := driftsync.LoadGlobalConfig(); err == nil {
			sharedRepo.GlobalUser = config.UserConfig{
				Name:  gcfg.User.Name,
				Email: gcfg.User.Email,
			}
		}
		return nil
	},
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a Drift project",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		store := storage.NewStore(dir)
		if store.IsInitialized() {
			fmt.Println("Drift project already exists")
			return nil
		}

		if err := store.Init(); err != nil {
			return fmt.Errorf("init failed: %w", err)
		}

		// Create main branch (empty hash means no commits yet)
		if err := store.SaveRef("main", ""); err != nil {
			return fmt.Errorf("failed to create main branch: %w", err)
		}

		// Set HEAD to the default branch so branch detection works before first switch.
		if err := store.SaveRef("HEAD", "main"); err != nil {
			return fmt.Errorf("failed to set HEAD: %w", err)
		}

		fmt.Println("Drift project initialized")

		// Generate a project ID for sync support.
		cfg := config.DefaultConfig()
		cfg.Sync.ProjectID = driftsync.NewProjectID()

		// Load existing global config (may already have user info).
		gcfg, _ := driftsync.LoadGlobalConfig()

		// If global config already has a user name, use it as default.
		defaultName := gcfg.User.Name
		defaultEmail := gcfg.User.Email
		name, email := promptUserInfo(defaultName, defaultEmail)

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

		// Save project config (core settings + project ID; user info
		// comes from global config unless overridden per-project).
		if err := config.SaveConfig(store.DriftDir(), cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save config: %v\n", err)
		}

		fmt.Println("\nNext steps:")
		fmt.Println("  drift add .       # stage your files")
		fmt.Println("  drift save -m \"first version\"")
		fmt.Println("  drift history --all   # view history")
		return nil
	},
}

// promptUserInfo asks the user for their name and email via stdin.
// If defaults are provided, they are shown in the prompt. Returns the
// defaults (if user presses Enter) or the entered values. Returns empty
// strings if stdin is not interactive.
func promptUserInfo(defaultName, defaultEmail string) (name, email string) {
	// Skip prompt if stdin is not a terminal (e.g. piped input or test).
	fi, err := os.Stdin.Stat()
	if err != nil {
		return defaultName, defaultEmail
	}
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		// stdin is redirected (pipe/file) — skip interactive prompt.
		return defaultName, defaultEmail
	}

	reader := bufio.NewReader(os.Stdin)

	if defaultName != "" {
		fmt.Printf("\nEnter your name (for version history) [%s]: ", defaultName)
	} else {
		fmt.Print("\nEnter your name (for version history): ")
	}
	line, _ := reader.ReadString('\n')
	name = strings.TrimSpace(line)
	if name == "" {
		name = defaultName
	}

	if defaultEmail != "" {
		fmt.Printf("Enter your email (optional) [%s]: ", defaultEmail)
	} else {
		fmt.Print("Enter your email (optional): ")
	}
	line, _ = reader.ReadString('\n')
	email = strings.TrimSpace(line)
	if email == "" {
		email = defaultEmail
	}

	return name, email
}

func init() {
	rootCmd.AddCommand(initCmd)

	// Global persistent flags available to all subcommands.
	rootCmd.PersistentFlags().StringVarP(&globalRepoPath, "directory", "C", "", "Run as if drift was started in <path>")
	rootCmd.PersistentFlags().BoolVarP(&globalVerbose, "verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolVarP(&globalQuiet, "quiet", "q", false, "Quiet output (errors only)")
	rootCmd.PersistentFlags().BoolVar(&globalDryRun, "dry-run", false, "Preview without executing")
	rootCmd.PersistentFlags().BoolVar(&globalNoColor, "no-color", false, "Disable color output")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

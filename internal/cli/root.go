package cli

import (
	"fmt"
	"os"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/storage"
	"github.com/spf13/cobra"
)

var (
	sharedStore  *storage.Store
	sharedConfig *config.Config
	sharedDir    string
)

var rootCmd = &cobra.Command{
	Use:   "drift",
	Short: "Drift - A lightweight version control tool for creative workers",
	Long:  "Drift lets creative workers manage their work like developers manage code.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "init" || cmd.Name() == "help" {
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

		sharedConfig, _ = config.LoadConfig(sharedStore.DriftDir())
		if sharedConfig == nil {
			sharedConfig = config.DefaultConfig()
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

		fmt.Println("Drift project initialized")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

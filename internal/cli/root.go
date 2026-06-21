package cli

import (
	"fmt"
	"os"

	"github.com/drift/drift/internal/storage"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "drift",
	Short: "Drift - A lightweight version control tool for creative workers",
	Long:  "Drift lets creative workers manage their work like developers manage code.",
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

package main

import (
	"fmt"
	"os"

	"github.com/drift/drift/internal/app"
	"github.com/drift/drift/internal/cli"
	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/storage"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get current directory: %v\n", err)
		os.Exit(1)
	}

	store := storage.NewStore(dir)
	cfg, err := config.LoadConfig(store.DriftDir())
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: failed to load config: %v\n", err)
		}
		cfg = config.DefaultConfig()
	}

	application := app.New(store, cfg, dir)
	application.WarnIfOutdated()

	rootCmd := cli.BuildRootCmd(application)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
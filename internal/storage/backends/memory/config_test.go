package memory

import (
	"context"
	"testing"

	"github.com/your-org/drift/internal/core"
)

func TestGetConfig_DefaultWhenUnset(t *testing.T) {
	store := NewMemoryStorage()
	cfg, err := store.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Core.IgnoreFile != core.DefaultIgnoreFile {
		t.Errorf("IgnoreFile: got %q, want %q", cfg.Core.IgnoreFile, core.DefaultIgnoreFile)
	}
}

func TestSetConfig_RoundTrip(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	cfg := &core.Config{
		User: core.UserConfig{Name: "tester", Email: "t@example.com"},
		Core: core.CoreConfig{
			Compression:      true,
			CompressionLevel: 5,
			IgnoreFile:       ".customignore",
			AutoSaveInterval: 600,
			AutoSaveKeep:     20,
		},
	}
	if err := store.SetConfig(ctx, cfg); err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}
	got, err := store.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if got.User.Name != "tester" {
		t.Errorf("User.Name: got %q, want %q", got.User.Name, "tester")
	}
	if got.Core.IgnoreFile != ".customignore" {
		t.Errorf("Core.IgnoreFile: got %q, want %q", got.Core.IgnoreFile, ".customignore")
	}
	if got.Core.CompressionLevel != 5 {
		t.Errorf("CompressionLevel: got %d, want 5", got.Core.CompressionLevel)
	}
}

func TestGetConfig_ClonesState(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	store.SetConfig(ctx, &core.Config{Core: core.CoreConfig{IgnoreFile: ".origignore"}})

	got, _ := store.GetConfig(ctx)
	got.Core.IgnoreFile = ".mutated"

	again, _ := store.GetConfig(ctx)
	if again.Core.IgnoreFile != ".origignore" {
		t.Errorf("mutating returned config affected stored state: got %q, want %q",
			again.Core.IgnoreFile, ".origignore")
	}
}

func TestSetConfig_ClonesInput(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	cfg := &core.Config{Core: core.CoreConfig{IgnoreFile: ".origignore"}}
	store.SetConfig(ctx, cfg)

	// Mutate the input config after SetConfig; stored value should be unaffected.
	cfg.Core.IgnoreFile = ".mutated"
	got, _ := store.GetConfig(ctx)
	if got.Core.IgnoreFile != ".origignore" {
		t.Errorf("mutating input config affected stored state: got %q, want %q",
			got.Core.IgnoreFile, ".origignore")
	}
}

func TestGetConfig_AppliesNormalization(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	// Set a config with empty IgnoreFile; GetConfig should normalize it to the default.
	store.SetConfig(ctx, &core.Config{Core: core.CoreConfig{IgnoreFile: ""}})
	got, err := store.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if got.Core.IgnoreFile != core.DefaultIgnoreFile {
		t.Errorf("IgnoreFile after normalize: got %q, want %q",
			got.Core.IgnoreFile, core.DefaultIgnoreFile)
	}
}

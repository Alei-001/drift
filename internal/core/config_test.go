package core

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Core.IgnoreFile != DefaultIgnoreFile {
		t.Errorf("expected IgnoreFile %q, got %q", DefaultIgnoreFile, cfg.Core.IgnoreFile)
	}
	if cfg.Core.Compression != true {
		t.Error("expected Compression=true by default")
	}
	if cfg.Core.AutoSaveInterval != DefaultAutoSaveInterval {
		t.Errorf("expected AutoSaveInterval=%d, got %d", DefaultAutoSaveInterval, cfg.Core.AutoSaveInterval)
	}
	if cfg.Core.AutoSaveKeep != DefaultAutoSaveKeep {
		t.Errorf("expected AutoSaveKeep=%d, got %d", DefaultAutoSaveKeep, cfg.Core.AutoSaveKeep)
	}
}

func TestZstdLevel(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 3},    // zero returns default
		{1, 1},    // minimum
		{3, 3},    // normal
		{19, 19},  // maximum
		{20, 19},  // above maximum clamped
		{-1, 3},   // negative returns default
		{100, 19}, // way above clamped
	}
	for _, tc := range tests {
		cfg := &CoreConfig{CompressionLevel: tc.input}
		if got := cfg.ZstdLevel(); got != tc.want {
			t.Errorf("ZstdLevel() with level=%d = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	t.Run("zero-value struct gets defaults", func(t *testing.T) {
		cfg := CoreConfig{}
		cfg.Normalize()
		if cfg.IgnoreFile != DefaultIgnoreFile {
			t.Errorf("IgnoreFile = %q, want %q", cfg.IgnoreFile, DefaultIgnoreFile)
		}
		if cfg.AutoSaveInterval != DefaultAutoSaveInterval {
			t.Errorf("AutoSaveInterval = %d, want %d", cfg.AutoSaveInterval, DefaultAutoSaveInterval)
		}
		if cfg.AutoSaveKeep != DefaultAutoSaveKeep {
			t.Errorf("AutoSaveKeep = %d, want %d", cfg.AutoSaveKeep, DefaultAutoSaveKeep)
		}
	})

	t.Run("valid fields preserved", func(t *testing.T) {
		cfg := CoreConfig{
			IgnoreFile:       "custom-ignore",
			AutoSaveInterval: 600,
			AutoSaveKeep:     20,
		}
		cfg.Normalize()
		if cfg.IgnoreFile != "custom-ignore" {
			t.Errorf("IgnoreFile = %q, want %q", cfg.IgnoreFile, "custom-ignore")
		}
		if cfg.AutoSaveInterval != 600 {
			t.Errorf("AutoSaveInterval = %d, want 600", cfg.AutoSaveInterval)
		}
		if cfg.AutoSaveKeep != 20 {
			t.Errorf("AutoSaveKeep = %d, want 20", cfg.AutoSaveKeep)
		}
	})

	t.Run("idempotent on DefaultConfig", func(t *testing.T) {
		cfg := DefaultConfig()
		before := cfg.Core
		cfg.Core.Normalize()
		after := cfg.Core
		if before != after {
			t.Errorf("Normalize() should be idempotent on already-default config: before=%+v after=%+v", before, after)
		}
	})
}

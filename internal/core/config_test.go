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
	if cfg.Core.ChunkMinSize != DefaultChunkMinSize {
		t.Errorf("expected ChunkMinSize=%d, got %d", DefaultChunkMinSize, cfg.Core.ChunkMinSize)
	}
	if cfg.Core.ChunkAvgSize != DefaultChunkAvgSize {
		t.Errorf("expected ChunkAvgSize=%d, got %d", DefaultChunkAvgSize, cfg.Core.ChunkAvgSize)
	}
	if cfg.Core.ChunkMaxSize != DefaultChunkMaxSize {
		t.Errorf("expected ChunkMaxSize=%d, got %d", DefaultChunkMaxSize, cfg.Core.ChunkMaxSize)
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
		// Chunk sizes: 0 is preserved (means "use engine default"), not reset.
		if cfg.ChunkMinSize != 0 {
			t.Errorf("ChunkMinSize should stay 0, got %d", cfg.ChunkMinSize)
		}
		if cfg.ChunkAvgSize != 0 {
			t.Errorf("ChunkAvgSize should stay 0, got %d", cfg.ChunkAvgSize)
		}
		if cfg.ChunkMaxSize != 0 {
			t.Errorf("ChunkMaxSize should stay 0, got %d", cfg.ChunkMaxSize)
		}
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

	t.Run("negative chunk sizes clamped to 0", func(t *testing.T) {
		cfg := CoreConfig{
			ChunkMinSize: -100,
			ChunkAvgSize: -1,
			ChunkMaxSize: -50,
		}
		cfg.Normalize()
		if cfg.ChunkMinSize != 0 || cfg.ChunkAvgSize != 0 || cfg.ChunkMaxSize != 0 {
			t.Errorf("negative chunk sizes should be 0, got min=%d avg=%d max=%d",
				cfg.ChunkMinSize, cfg.ChunkAvgSize, cfg.ChunkMaxSize)
		}
	})

	t.Run("positive chunk sizes preserved", func(t *testing.T) {
		cfg := CoreConfig{
			ChunkMinSize: 1024,
			ChunkAvgSize: 4096,
			ChunkMaxSize: 16384,
		}
		cfg.Normalize()
		if cfg.ChunkMinSize != 1024 || cfg.ChunkAvgSize != 4096 || cfg.ChunkMaxSize != 16384 {
			t.Errorf("positive chunk sizes should be preserved, got min=%d avg=%d max=%d",
				cfg.ChunkMinSize, cfg.ChunkAvgSize, cfg.ChunkMaxSize)
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

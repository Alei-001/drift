package core

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Core.IgnoreFile != ".driftignore" {
		t.Errorf("expected IgnoreFile '.driftignore', got %q", cfg.Core.IgnoreFile)
	}
	if cfg.Core.Compression != true {
		t.Error("expected Compression=true by default")
	}
	if cfg.Core.AutoSaveInterval != 300 {
		t.Errorf("expected AutoSaveInterval=300, got %d", cfg.Core.AutoSaveInterval)
	}
	if cfg.Core.AutoSaveKeep != 10 {
		t.Errorf("expected AutoSaveKeep=10, got %d", cfg.Core.AutoSaveKeep)
	}
	// Chunk sizes should have sensible defaults (128K/256K/512K)
	if cfg.Core.ChunkMinSize != 128*1024 {
		t.Errorf("expected ChunkMinSize=%d, got %d", 128*1024, cfg.Core.ChunkMinSize)
	}
	if cfg.Core.ChunkAvgSize != 256*1024 {
		t.Errorf("expected ChunkAvgSize=%d, got %d", 256*1024, cfg.Core.ChunkAvgSize)
	}
	if cfg.Core.ChunkMaxSize != 512*1024 {
		t.Errorf("expected ChunkMaxSize=%d, got %d", 512*1024, cfg.Core.ChunkMaxSize)
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

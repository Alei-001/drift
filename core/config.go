package core

// Default configuration values. Centralized here so that DefaultConfig(),
// storage-layer normalization, and downstream consumers (chunker, fsutil)
// all reference the same source of truth instead of repeating magic literals.
const (
	DefaultChunkMinSize     = 128 * 1024  // 128KB
	DefaultChunkAvgSize     = 256 * 1024  // 256KB
	DefaultChunkMaxSize     = 512 * 1024  // 512KB
	DefaultIgnoreFile       = ".driftignore"
	DefaultAutoSaveInterval = 300
	DefaultAutoSaveKeep     = 10
)

const HeaderPeekSize = 512

type UserConfig struct {
	Name  string
	Email string
}

type CoreConfig struct {
	ChunkMinSize     int    `json:"chunk_min_size"`
	ChunkAvgSize     int    `json:"chunk_avg_size"`
	ChunkMaxSize     int    `json:"chunk_max_size"`
	Compression      bool   `json:"compression"`
	CompressionLevel int    `json:"compression_level"`
	IgnoreFile       string `json:"ignore_file"`
	AutoSaveInterval int    `json:"auto_save_interval"`
	AutoSaveKeep     int    `json:"auto_save_keep"`
}

type Config struct {
	User UserConfig `json:"user"`
	Core CoreConfig `json:"core"`
}

func DefaultConfig() *Config {
	return &Config{
		Core: CoreConfig{
			ChunkMinSize:     DefaultChunkMinSize,
			ChunkAvgSize:     DefaultChunkAvgSize,
			ChunkMaxSize:     DefaultChunkMaxSize,
			Compression:      true,
			CompressionLevel: 3,
			IgnoreFile:       DefaultIgnoreFile,
			AutoSaveInterval: DefaultAutoSaveInterval,
			AutoSaveKeep:     DefaultAutoSaveKeep,
		},
	}
}

// Normalize clamps invalid fields to their defaults. It is idempotent and
// safe to call on any CoreConfig, including zero-value structs and configs
// loaded from JSON that may have partial/missing fields.
//
// Chunk sizes: negative values are clamped to 0 (meaning "use the filetype
// engine's own default" — downstream chunkers handle 0 via a > 0 check).
// Zero is preserved on purpose so users can opt into engine-specific defaults
// by setting chunk_min_size: 0 in their config JSON.
//
// This logic lives in the core package so both filesystem and memory storage
// backends apply the same normalization, avoiding backend-specific drift.
func (c *CoreConfig) Normalize() {
	if c.ChunkMinSize < 0 {
		c.ChunkMinSize = 0
	}
	if c.ChunkAvgSize < 0 {
		c.ChunkAvgSize = 0
	}
	if c.ChunkMaxSize < 0 {
		c.ChunkMaxSize = 0
	}
	if c.IgnoreFile == "" {
		c.IgnoreFile = DefaultIgnoreFile
	}
	if c.AutoSaveInterval <= 0 {
		c.AutoSaveInterval = DefaultAutoSaveInterval
	}
	if c.AutoSaveKeep <= 0 {
		c.AutoSaveKeep = DefaultAutoSaveKeep
	}
}

func (c *CoreConfig) ZstdLevel() int {
	if c.CompressionLevel < 1 {
		return 3
	}
	if c.CompressionLevel > 19 {
		return 19
	}
	return c.CompressionLevel
}

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

// Zstd compression level bounds. DefaultZstdLevel is used when compression
// is enabled but no explicit level is configured; MinZstdLevel and
// MaxZstdLevel clamp out-of-range values in ZstdLevel().
const (
	DefaultZstdLevel = 3
	MinZstdLevel     = 1
	MaxZstdLevel     = 19
)

type UserConfig struct {
	Name  string
	Email string
}

type CoreConfig struct {
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
			Compression:      true,
			CompressionLevel: DefaultZstdLevel,
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
// This logic lives in the core package so both filesystem and memory storage
// backends apply the same normalization, avoiding backend-specific drift.
func (c *CoreConfig) Normalize() {
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
	if c.CompressionLevel < MinZstdLevel {
		return DefaultZstdLevel
	}
	if c.CompressionLevel > MaxZstdLevel {
		return MaxZstdLevel
	}
	return c.CompressionLevel
}

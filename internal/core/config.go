package core

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Default configuration values. Centralized here so that both DefaultConfig(),
// storage-layer normalization, and downstream consumers (chunker, fsutil)
// all reference the same source of truth instead of repeating magic literals.
const (
	DefaultChunkMinSize     = 128 * 1024 // 128KB
	DefaultChunkAvgSize     = 256 * 1024 // 256KB
	DefaultChunkMaxSize     = 512 * 1024 // 512KB
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
	// TrustMtime, when true, enables the (size, mtime) fast path in
	// CreateSnapshot: a file whose size and mtime match the index entry is
	// reused without re-chunking. This is a performance optimization that
	// trades correctness for speed — tools that preserve mtime while
	// changing content (cp -p, rsync --times, editor atomic-save that
	// restores mtime) would silently cause stale chunks to be reused.
	// Defaults to false (safe): every save re-chunks every file. Users who
	// understand the risk and need the speedup can set this true via
	// config.json or DRIFT_TRUST_MTIME=1.
	TrustMtime bool `json:"trust_mtime,omitempty"`
}

// ConfigVersion is the current on-disk config schema version. Bump when the
// config struct gains a field that requires migration logic; loaders can then
// branch on Version to apply transforms. Version 1 is the first versioned
// schema; a missing version (zero value) is read as a legacy file.
const ConfigVersion = 1

type Config struct {
	// Version is the schema version of the config file. Omitted from JSON
	// when zero (legacy files) so existing config.json files load unchanged.
	Version int        `json:"version,omitempty"`
	User    UserConfig `json:"user"`
	Core    CoreConfig `json:"core"`
}

func DefaultConfig() *Config {
	return &Config{
		Version: ConfigVersion,
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
// After Normalize, every field is guaranteed to hold a legal value, so
// callers can read CompressionLevel directly without going through ZstdLevel().
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
	// Clamp CompressionLevel into [MinZstdLevel, MaxZstdLevel], defaulting
	// out-of-range-low to DefaultZstdLevel (so a zero value means "use the
	// default" rather than "minimum compression"). After this clamp,
	// ZstdLevel() returns c.CompressionLevel unchanged.
	c.CompressionLevel = c.ZstdLevel()
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

// Environment variables that override config file values at runtime. They take
// precedence over the on-disk config.json so operators can tune drift per
// environment (CI, containers) without editing the repo. Overrides are applied
// during NormalizeConfig (see internal/storage/config_store.go) BEFORE
// Normalize, so out-of-range values are clamped rather than left invalid.
//
// DRIFT_LOG_LEVEL is handled separately in internal/util/logutil/logutil.go.
//
// Note: DRIFT_CHUNK_MIN_SIZE has no target field — chunk sizes are
// intentionally hardcoded (see cmd/config.go configFields comment) and are not
// user-tunable, so no env override is wired for it.
const (
	envAutoSaveInterval = "DRIFT_AUTO_SAVE_INTERVAL"
	envAutoSaveKeep     = "DRIFT_AUTO_SAVE_KEEP"
	envZstdLevel        = "DRIFT_ZSTD_LEVEL"
	envTrustMtime       = "DRIFT_TRUST_MTIME"
)

// ApplyEnvOverrides replaces config fields with the corresponding DRIFT_*
// environment variable when it is set to a parseable value. Unset or
// unparseable values are ignored (the file value is kept) so a typo cannot
// wipe a valid config. Out-of-range clamping is left to Normalize, which runs
// immediately after this in NormalizeConfig.
func (c *Config) ApplyEnvOverrides() {
	if v := os.Getenv(envAutoSaveInterval); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Core.AutoSaveInterval = n
		}
	}
	if v := os.Getenv(envAutoSaveKeep); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Core.AutoSaveKeep = n
		}
	}
	if v := os.Getenv(envZstdLevel); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Core.CompressionLevel = n
		}
	}
	if v := os.Getenv(envTrustMtime); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.Core.TrustMtime = b
		}
	}
}

// Validate checks semantic validity of the config AFTER Normalize has clamped
// out-of-range-low values to defaults. It catches values that are syntactically
// legal but operationally wrong — e.g. an IgnoreFile path containing ".."
// (path-traversal risk) or an absurdly large AutoSaveKeep that would exhaust
// disk. Validate is non-mutating; callers decide how to react
// (NormalizeConfig logs a warning and keeps the clamped value rather than
// failing the whole operation, since a clamped-but-suspicious value is usually
// preferable to aborting a workspace command).
func (c *Config) Validate() error {
	if c.Core.IgnoreFile != "" && strings.Contains(c.Core.IgnoreFile, "..") {
		return fmt.Errorf("ignore_file path contains '..': %s", c.Core.IgnoreFile)
	}
	if c.Core.AutoSaveKeep > 10000 {
		return fmt.Errorf("auto_save_keep unreasonably large: %d", c.Core.AutoSaveKeep)
	}
	return nil
}

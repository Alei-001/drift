package core

// UserConfig holds user identity configuration.
type UserConfig struct {
	Name  string
	Email string
}

// CoreConfig holds core settings for chunking and compression.
type CoreConfig struct {
	ChunkMinSize     int    // default 131072
	ChunkAvgSize     int    // default 262144
	ChunkMaxSize     int    // default 524288
	CompressionLevel int    // default 3
	IgnoreFile       string // default ".driftignore"
}

// Config is the top-level configuration for drift.
type Config struct {
	User UserConfig
	Core CoreConfig
}

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Core: CoreConfig{
			ChunkMinSize:     131072,
			ChunkAvgSize:     262144,
			ChunkMaxSize:     524288,
			CompressionLevel: 3,
			IgnoreFile:       ".driftignore",
		},
	}
}

// DefaultIgnorePatterns returns the default ignore patterns.
func DefaultIgnorePatterns() []string {
	return []string{".drift/", "**/.drift/**"}
}

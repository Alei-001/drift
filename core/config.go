package core

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
			ChunkMinSize:     128 * 1024,
			ChunkAvgSize:     256 * 1024,
			ChunkMaxSize:     512 * 1024,
			Compression:      true,
			CompressionLevel: 3,
			IgnoreFile:       ".driftignore",
			AutoSaveInterval: 300,
			AutoSaveKeep:     10,
		},
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

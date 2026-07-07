package storage

import (
	"context"

	"github.com/Alei-001/drift/internal/core"
)

// ConfigStorer provides access to configuration storage.
type ConfigStorer interface {
	GetConfig(ctx context.Context) (*core.Config, error)
	SetConfig(ctx context.Context, config *core.Config) error
	// SetCompressionConfig applies the compression settings to the backend.
	// enabled toggles zstd compression of chunk payloads; level is the zstd
	// encoder level (1-19, clamped by the implementation). Backends that do
	// not compress (e.g. the in-memory test backend) implement this as a
	// no-op so porcelain can apply config uniformly across backends without
	// type-asserting to a concrete implementation.
	SetCompressionConfig(enabled bool, level int) error
}

// NormalizeConfig applies shared invariants to a loaded config so both
// backends observe identical field semantics. It runs the core-level
// Normalize (empty/zero fields→defaults). Both the filesystem and memory
// backends call this from GetConfig so the logic lives in one place rather
// than being duplicated per backend.
func NormalizeConfig(cfg *core.Config) {
	cfg.Core.Normalize()
}


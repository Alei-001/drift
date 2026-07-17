package store

import (
	"context"
	"log/slog"

	"github.com/Alei-001/drift/internal/core"
)

// ConfigStorer provides access to configuration store.
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
// backends observe identical field semantics. It runs three phases:
//  1. ApplyEnvOverrides — DRIFT_* environment variables replace the file
//     values (runtime overrides for CI/containers).
//  2. Normalize — empty/zero/out-of-range fields are clamped to defaults,
//     including any values supplied via env overrides in the previous step.
//  3. Validate — semantic sanity check; on failure a warning is logged
//     (slog.Warn) and the clamped value is kept. Failing the whole operation
//     for a suspicious-but-clamped value would be more disruptive than the
//     misconfiguration itself.
//
// Both the filesystem and memory backends call this from GetConfig so the
// logic lives in one place rather than being duplicated per backend.
func NormalizeConfig(cfg *core.Config) {
	cfg.ApplyEnvOverrides()
	cfg.Core.Normalize()
	if err := cfg.Validate(); err != nil {
		slog.Warn("invalid config field", "error", err)
	}
}

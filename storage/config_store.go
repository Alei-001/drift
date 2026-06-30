package storage

import (
	"context"

	"github.com/your-org/drift/core"
)

// ConfigStorer provides access to configuration storage.
type ConfigStorer interface {
	GetConfig(ctx context.Context) (*core.Config, error)
	SetConfig(ctx context.Context, config *core.Config) error
}

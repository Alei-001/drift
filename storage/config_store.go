package storage

import "github.com/your-org/drift/core"

// ConfigStorer provides access to configuration storage.
type ConfigStorer interface {
	GetConfig() (*core.Config, error)
	SetConfig(config *core.Config) error
}

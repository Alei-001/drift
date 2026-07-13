package filesystem

import "github.com/Alei-001/drift/internal/storage"

// Layout constants are re-exported from the storage interface package so
// existing references within the filesystem backend continue to work.
// The single source of truth is storage/layout.go; keeping the canonical
// constants in the storage package lets the remote package depend on
// storage only, without reaching into a concrete backend.
const (
	DriftDir     = storage.DriftDir
	ChunksDir    = storage.ChunksDir
	SnapshotsDir = storage.SnapshotsDir
	ManifestsDir = storage.ManifestsDir
	RefsDir      = storage.RefsDir
	PreviewsDir  = storage.PreviewsDir
	HeadsDir     = storage.HeadsDir
	TagsDir      = storage.TagsDir
	LogsDir      = storage.LogsDir
	HeadFile     = storage.HeadFile
	IndexFile    = storage.IndexFile
	ConfigFile   = storage.ConfigFile
	PacksDir     = storage.PacksDir
)

package filesystem

// Directory layout constants for the .drift/ storage structure.
const (
	DriftDir     = ".drift"
	ChunksDir    = "chunks"
	SnapshotsDir = "snapshots"
	RefsDir      = "refs"
	PreviewsDir  = "previews"
	HeadsDir     = "heads"
	TagsDir      = "tags"
	LogsDir      = "logs"
	IndexFile    = "index"
	ConfigFile   = "config"
	// StorageLockFile is the process-level lock file that serializes
	// mutating operations across drift processes. It is created with
	// O_CREATE|O_EXCL and holds the PID of the lock holder.
	StorageLockFile = "storage.lock"
)

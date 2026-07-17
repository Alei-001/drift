package store

// Layout constants shared between filesystem backend and remote sync.
//
// Both the on-disk .drift/ layout and the remote wire layout use these
// directory names. Keeping them in the storage interface package (rather
// than in backends/filesystem) lets the remote package depend on storage
// only, without reaching into a concrete backend. See AGENTS.md
// "Package boundaries".
const (
	DriftDir     = ".drift"
	ChunksDir    = "chunks"
	SnapshotsDir = "snapshots"
	ManifestsDir = "manifests"
	RefsDir      = "refs"
	PreviewsDir  = "previews"
	HeadsDir     = "heads"
	TagsDir      = "tags"
	LogsDir      = "logs"
	HeadFile     = "HEAD"
	IndexFile    = "index"
	ConfigFile   = "config"
	PacksDir     = "packs"
)

// HashPath returns the two-level content-addressed path for a hex hash:
// the first 2 characters as the parent directory, the rest as the filename.
// This layout is shared by chunks, snapshots, and manifests both on disk
// and on the remote. Returns the input unchanged if shorter than 2 chars.
func HashPath(hexHash string) string {
	if len(hexHash) < 2 {
		return hexHash
	}
	return hexHash[:2] + "/" + hexHash[2:]
}

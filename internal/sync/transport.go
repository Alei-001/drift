// Package sync provides remote synchronization for drift projects.
//
// Sync operates at the object level: push uploads objects missing on the
// remote, pull downloads objects missing locally. Both are anchored on
// commit DAG reachability via tracking refs — no manifest, no file scanning.
package sync

import (
	"io"
)

// Transport is the minimal interface for remote storage. All keys are
// forward-slash paths relative to the project root, mirroring the local
// .drift/ directory structure.
type Transport interface {
	// Get retrieves raw bytes for the given key.
	Get(key string) (io.ReadCloser, error)

	// Put stores raw bytes at the given key, creating parent directories
	// as needed.
	Put(key string, data io.Reader) error

	// Exists reports whether a key exists on the remote.
	Exists(key string) (bool, error)

	// GetRef reads a ref value (the commit hash it points to).
	// name is the ref name without the "refs/" prefix (e.g. "heads/main").
	// Returns the full 64-char hash, or empty string if the ref does not exist.
	GetRef(name string) (string, error)

	// PutRef atomically writes a ref value.
	PutRef(name string, hash string) error

	// ListRefs returns all refs on the remote (e.g. "heads/main" → hash).
	ListRefs() (map[string]string, error)

	// Close releases any resources held by the transport.
	Close() error
}

// objectPath returns the remote key for an object with the given hash and type.
// Matches the local .drift/ structure exactly.
func objectPath(hash string, typ string) string {
	if len(hash) < 2 {
		return hash
	}
	switch typ {
	case "blob":
		return "objects/blobs/" + hash[:2] + "/" + hash[2:]
	case "tree":
		return "objects/trees/" + hash[:2] + "/" + hash[2:] + ".dre"
	case "commit":
		return "commits/" + hash[:2] + "/" + hash[2:] + ".dcm"
	default:
		return hash
	}
}

// refKey returns the transport key for a ref name.
func refKey(name string) string {
	return "refs/" + name + ".ref"
}

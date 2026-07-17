package remote

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// Sentinel errors returned by the remote package. Use errors.Is to classify.
var (
	// ErrUnsupported is returned by NewRemoteFS when cfg.Type does not match
	// any registered protocol.
	ErrUnsupported = os.ErrInvalid
	// ErrNotFound mirrors store.ErrNotFound for remote paths.
	ErrNotFound = os.ErrNotExist
)

// RemoteFS is the abstract filesystem interface that all remote protocols
// implement. push/pull logic operates solely against this interface, so
// adding a new protocol (e.g. S3) only requires implementing it here and
// registering it in an init() block.
//
// Path contract: all paths passed to these methods are relative to the
// remote root, use forward slashes (/), and have NO leading slash. The
// root directory is represented by "" or ".". Path helpers
// (chunkRemotePath, snapshotRemotePath, etc.) produce paths that already
// conform to this contract. Each implementation's resolve() method is
// responsible for converting these relative paths to whatever form the
// underlying protocol library requires (e.g. gowebdav needs absolute paths
// with a leading /, go-smb2 needs relative paths without one).
//
// Every method takes a context.Context. The underlying protocol libraries
// (gowebdav, go-smb2) do not yet honor context for in-flight network calls,
// so implementations check ctx.Err() at entry and return context.Canceled /
// context.DeadlineExceeded before issuing the request. This lets callers
// bail out of long batch operations (push/pull loops) without launching new
// requests, and leaves the interface ready for context-aware libraries.
type RemoteFS interface {
	// Stat returns metadata for a remote path, or an error wrapping
	// os.ErrNotExist when the path does not exist.
	Stat(ctx context.Context, path string) (*RemoteInfo, error)
	// Read opens a remote file for reading. The caller must close the
	// returned reader. Returns an error wrapping os.ErrNotExist when the
	// path does not exist.
	Read(ctx context.Context, path string) (io.ReadCloser, error)
	// Write uploads a file. Path's parent directories are created if
	// needed. If the file already exists it is overwritten.
	Write(ctx context.Context, path string, r io.Reader) error
	// Remove deletes a remote file. A missing file is not an error.
	Remove(ctx context.Context, path string) error
	// List enumerates entries under a directory path. Returns an empty
	// slice (not nil) when the directory is empty or does not exist.
	List(ctx context.Context, path string) ([]RemoteInfo, error)
	// MkdirAll creates a directory tree, similar to os.MkdirAll.
	MkdirAll(ctx context.Context, path string) error
	// Close releases protocol-level resources (connections, sessions).
	// It must be called exactly once when the RemoteFS is no longer needed.
	Close() error
}

// RemoteInfo is the metadata returned by Stat and List.
type RemoteInfo struct {
	Path    string
	Size    int64
	IsDir   bool
	ModTime time.Time
}

// ProtocolFactory constructs a RemoteFS from a RemoteConfig. Each protocol
// implementation registers its factory in an init() block via Register.
type ProtocolFactory func(cfg RemoteConfig) (RemoteFS, error)

// protocols is the global registry of protocol factories. Each protocol
// implementation adds itself here in its init() function.
var protocols = map[string]ProtocolFactory{}

// Register adds a protocol factory under the given name. It is called from
// each protocol implementation's init() block. Register panics if a factory
// is already registered for name, which indicates a duplicate init.
func Register(name string, f ProtocolFactory) {
	if _, exists := protocols[name]; exists {
		panic(fmt.Sprintf("remote: duplicate protocol registration for %q", name))
	}
	protocols[name] = f
}

// NewRemoteFS looks up the registered factory for cfg.Type and constructs a
// RemoteFS. Returns ErrUnsupported (os.ErrInvalid) for unknown protocol names.
func NewRemoteFS(cfg RemoteConfig) (RemoteFS, error) {
	f, ok := protocols[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unknown protocol %q: %w", cfg.Type, ErrUnsupported)
	}
	return f(cfg)
}

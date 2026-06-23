package storage

import (
	"io"

	"github.com/drift/drift/internal/core"
)

type Storage interface {
	PutBlob(data []byte) (string, error)
	PutBlobFromFile(path string) (string, error)
	PutBlobFromReader(r io.Reader) (string, error)
	GetBlob(hash string) ([]byte, error)

	PutTree(t *core.Tree) error
	GetTree(hash string) (*core.Tree, error)

	PutCommit(c *core.Commit) error
	GetCommit(id string) (*core.Commit, error)
	ListCommits() ([]*core.Commit, error)

	SaveRef(name string, hash string) error
	GetRef(name string) (string, error)

	SaveIndex(idx *core.Index) error
	LoadIndex(idx *core.Index) error

	IsInitialized() bool
}

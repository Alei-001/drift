package core

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Signature struct {
	Name  string
	Email string
}

// CommitIDLen is the number of hex characters used for the user-facing
// commit ID (an abbreviated form of the full hash, similar to Git's
// short hash). 8 hex chars = 4 bytes = ~4 billion possibilities, which
// is more than enough to avoid collisions in a single project.
const CommitIDLen = 8

type Commit struct {
	Hash      string
	ID        string // abbreviated hash (first CommitIDLen hex chars)
	Message   string
	Timestamp time.Time
	Parent    string
	Branch    string
	TreeHash  string
	Author    Signature
}

// NewCommit creates a commit with an abbreviated-hash ID derived from
// the commit's content hash. The ID is the first CommitIDLen hex
// characters of Hash, similar to Git's short hash.
// Returns an error if any string field contains NUL bytes, which would
// corrupt the hash computation.
func NewCommit(message, parent, branch, treeHash string, author Signature) (*Commit, error) {
	if strings.Contains(message, "\x00") ||
		strings.Contains(parent, "\x00") ||
		strings.Contains(branch, "\x00") ||
		strings.Contains(treeHash, "\x00") ||
		strings.Contains(author.Name, "\x00") ||
		strings.Contains(author.Email, "\x00") {
		return nil, fmt.Errorf("commit fields must not contain NUL bytes")
	}

	c := &Commit{
		Message:   message,
		Timestamp: time.Now(),
		Parent:    parent,
		Branch:    branch,
		TreeHash:  treeHash,
		Author:    author,
	}
	c.Hash = c.calculateHash()
	c.ID = c.Hash[:CommitIDLen]
	return c, nil
}

const nullHash = "0000000000000000000000000000000000000000000000000000000000000000"

func (c *Commit) isRoot() bool {
	return c.Parent == "" || c.Parent == nullHash
}

func (c *Commit) calculateHash() string {
	// The ID is derived from the hash, so it is NOT included in the
	// hash computation (that would be circular).
	// Use \x00 as a separator between fields to prevent ambiguity.
	sep := "\x00"
	data := c.Message + sep + strconv.FormatInt(c.Timestamp.UnixMilli(), 10) + sep + c.Parent + sep + c.Branch + sep + c.TreeHash + sep + c.Author.Name + sep + c.Author.Email
	return CalculateHash([]byte(data))
}

// ComputeHash recomputes the commit's hash from its fields. Used by the
// storage layer to verify integrity on read.
func (c *Commit) ComputeHash() string {
	return c.calculateHash()
}

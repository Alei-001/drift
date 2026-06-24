package core

import (
	"strconv"
	"time"
)

type Signature struct {
	Name  string
	Email string
}

type Commit struct {
	Hash      string
	ID        string
	Message   string
	Timestamp time.Time
	Parent    string
	Branch    string
	TreeHash  string
	Author    Signature
}

func NewCommit(id, message, parent, branch, treeHash string, author Signature) *Commit {
	c := &Commit{
		ID:        id,
		Message:   message,
		Timestamp: time.Now(),
		Parent:    parent,
		Branch:    branch,
		TreeHash:  treeHash,
		Author:    author,
	}
	c.Hash = c.calculateHash()
	return c
}

const nullHash = "0000000000000000000000000000000000000000000000000000000000000000"

func (c *Commit) isRoot() bool {
	return c.Parent == "" || c.Parent == nullHash
}

func (c *Commit) calculateHash() string {
	// Issue 29: use UnixMilli (matches the stored precision) instead of
	// RFC3339 (second-level). RFC3339 would collide for two commits in the
	// same second with otherwise identical fields.
	// Use \x00 as a separator between fields to prevent ambiguity (e.g.
	// ID="a",Message="b" vs ID="ab",Message="" producing the same hash).
	sep := "\x00"
	data := c.ID + sep + c.Message + sep + strconv.FormatInt(c.Timestamp.UnixMilli(), 10) + sep + c.Parent + sep + c.Branch + sep + c.TreeHash + sep + c.Author.Name + sep + c.Author.Email
	return CalculateHash([]byte(data))
}

// ComputeHash recomputes the commit's hash from its fields. Used by the
// storage layer to verify integrity on read.
func (c *Commit) ComputeHash() string {
	return c.calculateHash()
}

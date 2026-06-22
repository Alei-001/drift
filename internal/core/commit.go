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
	data := c.ID + c.Message + strconv.FormatInt(c.Timestamp.UnixMilli(), 10) + c.Parent + c.Branch + c.TreeHash + c.Author.Name + c.Author.Email
	return CalculateHash([]byte(data))
}

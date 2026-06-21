package core

import "time"

type Commit struct {
	Hash      string
	ID        string
	Message   string
	Timestamp time.Time
	Parent    string
	Branch    string
	TreeHash  string
}

func NewCommit(id, message, parent, branch, treeHash string) *Commit {
	c := &Commit{
		ID:        id,
		Message:   message,
		Timestamp: time.Now(),
		Parent:    parent,
		Branch:    branch,
		TreeHash:  treeHash,
	}
	c.Hash = c.calculateHash()
	return c
}

func (c *Commit) calculateHash() string {
	data := c.ID + c.Message + c.Timestamp.Format(time.RFC3339) + c.Parent + c.Branch + c.TreeHash
	return CalculateHash([]byte(data))
}

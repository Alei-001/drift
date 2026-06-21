package core

import "time"

type Commit struct {
	Hash      string    `json:"hash"`
	ID        string    `json:"id"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Parent    string    `json:"parent"`
	Branch    string    `json:"branch"`
	TreeHash  string    `json:"tree_hash"`
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

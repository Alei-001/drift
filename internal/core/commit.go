package core

import "time"

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
	data := c.ID + c.Message + c.Timestamp.Format(time.RFC3339) + c.Parent + c.Branch + c.TreeHash + c.Author.Name + c.Author.Email
	return CalculateHash([]byte(data))
}

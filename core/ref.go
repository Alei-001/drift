package core

// RefType represents the type of a reference.
type RefType string

const (
	RefTypeBranch RefType = "branch"
	RefTypeTag    RefType = "tag"
	RefTypeHead   RefType = "HEAD"
)

// Reference represents a named reference to a commit hash.
type Reference struct {
	Name   string
	Type   RefType
	Target Hash
	SymRef string
}

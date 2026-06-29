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

// IsSymRef reports whether this reference is a symbolic reference
// (e.g. HEAD -> heads/main) rather than a direct hash.
func (r *Reference) IsSymRef() bool {
	return r.SymRef != ""
}

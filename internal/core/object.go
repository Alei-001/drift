package core

type ObjectType int

const (
	BlobObject ObjectType = iota
	TreeObject
	CommitObject
)

func (t ObjectType) String() string {
	switch t {
	case BlobObject:
		return "blob"
	case TreeObject:
		return "tree"
	case CommitObject:
		return "commit"
	default:
		return "unknown"
	}
}

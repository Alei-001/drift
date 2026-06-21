package core

type Blob struct {
	Hash string
	Size int64
}

// NewBlob creates Blob metadata from data. The actual data should be stored via Store.PutBlob.
func NewBlob(data []byte) *Blob {
	return &Blob{
		Hash: CalculateHash(data),
		Size: int64(len(data)),
	}
}

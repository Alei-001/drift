package binary

// BinaryEngine is the fallback engine for binary files.
type BinaryEngine struct{}

// NewEngine creates a new BinaryEngine.
func NewEngine() *BinaryEngine {
	return &BinaryEngine{}
}

// Name returns "binary".
func (e *BinaryEngine) Name() string {
	return "binary"
}

// DetectByMagic returns false; binary has no magic signature.
func (e *BinaryEngine) DetectByMagic(header []byte) bool {
	return false
}

// DetectByExtension returns false; binary matches no specific extension.
func (e *BinaryEngine) DetectByExtension(path string) bool {
	return false
}

// DetectByHeuristic returns true; binary is the final fallback engine and
// matches any file that no other engine claimed.
func (e *BinaryEngine) DetectByHeuristic(path string, header []byte) bool {
	return true
}

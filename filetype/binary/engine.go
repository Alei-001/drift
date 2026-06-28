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

// Detect returns true for any file (binary is the fallback engine).
func (e *BinaryEngine) Detect(path string, header []byte) bool {
	return true
}

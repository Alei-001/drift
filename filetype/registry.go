package filetype

import "sync"

// Registry holds registered file type engines.
type Registry struct {
	mu      sync.RWMutex
	engines []Engine
}

var defaultRegistry = &Registry{}

// Register adds an engine to the default registry.
func Register(engine Engine) {
	defaultRegistry.Register(engine)
}

// DetectEngine finds the first matching engine for a file in the default registry.
func DetectEngine(path string, header []byte) Engine {
	return defaultRegistry.Detect(path, header)
}

// Register adds an engine to the registry.
func (r *Registry) Register(engine Engine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engines = append(r.engines, engine)
}

// Detect finds the best matching engine for a file using layered detection.
// Layer 1 (magic bytes) is queried first — the most reliable signal.
// Layer 2 (extension) is queried only if no magic matched.
// Layer 3 (heuristic) is the final fallback for unknown extensions.
// Returns nil if no engine matches (should not happen when binary is registered).
func (r *Registry) Detect(path string, header []byte) Engine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	// Layer 1: magic bytes (strongest)
	for _, e := range r.engines {
		if e.DetectByMagic(header) {
			return e
		}
	}
	// Layer 2: extension (medium)
	for _, e := range r.engines {
		if e.DetectByExtension(path) {
			return e
		}
	}
	// Layer 3: heuristic (weakest, fallback)
	for _, e := range r.engines {
		if e.DetectByHeuristic(path, header) {
			return e
		}
	}
	return nil
}

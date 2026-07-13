package filetype

import "sync"

// Registry holds registered file type engines.
type Registry struct {
	mu       sync.RWMutex
	engines  []Engine
	fallback Engine
}

var defaultRegistry = &Registry{}

// Register adds an engine to the default registry.
func Register(engine Engine) {
	defaultRegistry.Register(engine)
}

// SetFallback sets the fallback engine for the default registry. The
// fallback is returned by Detect when no registered engine matches.
// This should be called during initialization (e.g. init()) before any
// detection calls occur.
func SetFallback(engine Engine) {
	defaultRegistry.SetFallback(engine)
}

// DetectEngine finds the first matching engine for a file in the default
// registry.
//
// Engine registration order matters for heuristic detection: text must be
// registered before binary, because binary's DetectByHeuristic always
// returns true and would short-circuit text detection. The registration
// order in init.go is: text → image → video → binary.
func DetectEngine(path string, header []byte) Engine {
	return defaultRegistry.Detect(path, header)
}

// Register adds an engine to the registry.
func (r *Registry) Register(engine Engine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engines = append(r.engines, engine)
}

// SetFallback sets the fallback engine returned by Detect when no
// registered engine matches. Callers should set this during
// initialization to avoid nil returns from Detect.
func (r *Registry) SetFallback(engine Engine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallback = engine
}

// Detect finds the best matching engine for a file using layered detection.
// Layer 1 (magic bytes) is queried first — the most reliable signal.
// Layer 2 (extension) is queried only if no magic matched.
// Layer 3 (heuristic) is the final fallback for unknown extensions.
// If no engine matches, the explicit fallback engine is returned (or nil
// if no fallback was set — callers should check for nil in that case).
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
	// Explicit fallback (e.g. binary engine) — never nil when the
	// default registry is used, since init.go sets binary as fallback.
	return r.fallback
}

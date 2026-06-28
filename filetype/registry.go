package filetype

// Registry holds registered file type engines.
type Registry struct {
	engines []Engine
}

var defaultRegistry = &Registry{}

// Register adds an engine to the default registry.
func Register(engine Engine) {
	defaultRegistry.Register(engine)
}

// DefaultRegistry returns the default registry.
func DefaultRegistry() *Registry {
	return defaultRegistry
}

// DetectEngine finds the first matching engine for a file in the default registry.
func DetectEngine(path string, header []byte) Engine {
	return defaultRegistry.Detect(path, header)
}

// Register adds an engine to the registry.
func (r *Registry) Register(engine Engine) {
	r.engines = append(r.engines, engine)
}

// Detect finds the first matching engine for a file.
// Returns nil if no engine matches.
func (r *Registry) Detect(path string, header []byte) Engine {
	for _, e := range r.engines {
		if e.Detect(path, header) {
			return e
		}
	}
	return nil
}

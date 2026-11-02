package desktop

// Available reports whether desktop control can run in this process. It is
// false off Windows, and also false when the module is disabled by config, so
// the MCP layer can skip registration entirely.
func Available() bool { return platformAvailable() }

// NewBackend constructs the platform backend. On non-Windows platforms it
// returns a stub that fails with ErrUnsupportedPlatform, so callers get a
// clear message instead of a nil dereference.
func NewBackend(dpiAware bool) (Backend, error) { return newBackend(dpiAware) }

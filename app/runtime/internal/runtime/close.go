package runtime

type runtimeCloser interface {
	Close() error
}

// Close releases per-runtime external resources — MCP sessions and
// any future closer-owned handles. Idempotent.
func (r *Runtime) Close() error {
	if r == nil || r.closer == nil {
		return nil
	}
	return r.closer.Close()
}

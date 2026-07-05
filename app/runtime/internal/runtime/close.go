package runtime

// Close releases per-runtime external resources — MCP sessions and
// any future engine-owned handles. Idempotent.
func (r *Runtime) Close() error {
	if r == nil || r.engine == nil {
		return nil
	}
	return r.engine.Close()
}

package dispatch

// WireShapes exposes registered union, constraint and state-key metadata to
// artifact tooling. These facts describe the JSON binding, not operation
// execution, so dispatch remains their owner.
func WireShapes() *Shapes { return shapes }

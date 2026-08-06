package agent2

import "context"

// Definition is an immutable Agent behavior definition. Its methods may be
// called concurrently for different Processes. Implementations create
// a fresh Execution from validated Input or restore one from their own opaque,
// versioned ExecutionState. Definition methods must not depend on Host product
// identities, storage protocols, or mutable global registration.
type Definition interface {
	Descriptor() Descriptor
	Start(input Input) (Execution, error)
	Restore(state ExecutionState) (Execution, error)
}

// Execution is the single Strategy-owned state machine inside one Process.
// Step must be a bounded, deterministic state reduction over the current state
// and the supplied Signal prefix. It must not perform external I/O, read clock,
// random or global state, or start ownerless goroutines. External operations are
// returned as Effects. Snapshot must fail rather than return partial state.
//
// The Engine is the sole caller and never invokes Step concurrently for the same
// Execution. If Step or Snapshot fails, the instance is discarded and may only
// be rebuilt from the last stable ExecutionState.
type Execution interface {
	Step(ctx context.Context, signals []Signal) (Transition, error)
	Snapshot() (ExecutionState, error)
}

package runs

import (
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

// Equal reports whether two Pending values describe the same durable tree
// hand-off. Empty collections have one logical value regardless of nil backing,
// and timestamps compare by instant rather than location or monotonic metadata.
func (p Pending) Equal(other Pending) bool {
	return reflect.DeepEqual(canonicalPending(p), canonicalPending(other))
}

func canonicalPending(pending Pending) Pending {
	pending.Interrupts = slices.Clone(pending.Interrupts)
	pending.Bindings = slices.Clone(pending.Bindings)
	pending.Continuations = slices.Clone(pending.Continuations)
	pending.CreatedAt = canonicalTime(pending.CreatedAt)
	pending.Capabilities = canonicalPendingCapabilities(pending.Capabilities)
	for index := range pending.Continuations {
		pending.Continuations[index] = normalizeContinuationValue(pending.Continuations[index])
	}
	for index := range pending.Interrupts {
		pending.Interrupts[index] = normalizeInterruptValue(pending.Interrupts[index])
	}
	pending.Interrupts = nilIfEmpty(pending.Interrupts)
	pending.Bindings = nilIfEmpty(pending.Bindings)
	pending.Continuations = nilIfEmpty(pending.Continuations)
	return pending
}

func canonicalPendingCapabilities(capabilities run.RunCapabilities) run.RunCapabilities {
	capabilities = capabilities.Normalized()
	capabilities.InterruptKinds = nilIfEmpty(capabilities.InterruptKinds)
	return capabilities
}

func nilIfEmpty[S ~[]E, E any](values S) S {
	if len(values) == 0 {
		return nil
	}
	return values
}

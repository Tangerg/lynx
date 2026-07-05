package runtime

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

// stubAwaitable is a minimal core.Awaitable for parking tests.
type stubAwaitable struct{ id string }

func (s stubAwaitable) ID() string     { return s.id }
func (s stubAwaitable) PromptAny() any { return nil }
func (s stubAwaitable) OnResponseAny(any) (core.ResponseImpact, error) {
	return core.ImpactUnchanged, nil
}

// TestParkAwaitable_RefusesSecondConcurrentPark pins the concurrent-HITL guard:
// the pending-awaitable slot holds exactly one interrupt, so a second park while
// one is still pending FAILS rather than silently clobbering the first (which
// would lose an interrupt that could never be answered). deliverResponse frees
// the slot, so the normal sequential park → resume cycle is unaffected.
func TestParkAwaitable_RefusesSecondConcurrentPark(t *testing.T) {
	s := newProcessSignals()

	if got := s.parkAwaitable(stubAwaitable{id: "first"}); got != core.ActionWaiting {
		t.Fatalf("first park = %v, want ActionWaiting", got)
	}
	// A second park while the first is pending is refused — not a silent clobber.
	if got := s.parkAwaitable(stubAwaitable{id: "second"}); got != core.ActionFailed {
		t.Fatalf("second concurrent park = %v, want ActionFailed", got)
	}
	// The originally-parked awaitable is still the one pending.
	if aw := s.peekAwaitable(); aw == nil || aw.ID() != "first" {
		t.Fatalf("pending awaitable = %v, want the first (not overwritten)", aw)
	}

	// Delivering a response frees the slot, so the next park succeeds — the
	// sequential park → resume → re-park cycle keeps working.
	if _, err := s.deliverResponse(nil); err != nil {
		t.Fatalf("deliverResponse: %v", err)
	}
	if got := s.parkAwaitable(stubAwaitable{id: "third"}); got != core.ActionWaiting {
		t.Fatalf("park after resume = %v, want ActionWaiting (slot freed)", got)
	}

	// A nil request is always refused.
	if got := s.parkAwaitable(nil); got != core.ActionFailed {
		t.Fatalf("nil park = %v, want ActionFailed", got)
	}
}

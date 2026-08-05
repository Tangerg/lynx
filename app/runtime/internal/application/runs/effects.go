package runs

import "context"

// Nudge is a non-durable live workspace change notification.
type Nudge struct {
	CWD   string
	Paths []string
}

// Effects commits one segment's durable projections before publication and
// owns Run-boundary maintenance after the live stream closes.
type Effects interface {
	CommitOpening(ctx context.Context, opening OpeningCommit) error
	CommitEvent(ctx context.Context, commit EventCommit) error
	CommitTreeBarrier(ctx context.Context, barrier TreeBarrierCommit) error
	CommitWaitingSubtreeCancellation(
		ctx context.Context,
		commit WaitingSubtreeCancellationCommit,
	) (WaitingSubtreeCancellationResult, error)
	Nudge(cwd string, paths []string)
	Finish(ctx context.Context, fin Finish) error
}

// Finish describes terminal Run-boundary maintenance after the live stream closes.
type Finish struct {
	SessionID       string
	RunID           string
	CWD             string
	Parked          bool
	OpeningUserText string
}

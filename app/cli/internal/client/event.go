package client

// Event is one thing that happened while a run was streaming.
//
// The set is closed — the unexported method is what closes it — so a renderer's
// switch over events is exhaustive by construction. A [Runtime] implementation
// folds whatever its backend sends into exactly these; anything it cannot fold
// is a [BlockNotice], not a new event.
type Event interface {
	clientEvent()
}

// RunStarted opens a stream. It is the only place a run's id is announced, so a
// caller that wants to cancel or resume keeps it from here.
type RunStarted struct {
	RunID     string
	SessionID string
}

// BlockStarted appends a block to the transcript. A block whose body streams
// arrives with Text empty and grows through [BlockDelta].
type BlockStarted struct {
	Block Block
}

// BlockDelta appends to a block already started. Deltas concatenate: the body is
// the started text plus every delta in order, with nothing in between.
type BlockDelta struct {
	BlockID string
	Text    string
}

// BlockCompleted replaces a block with its final form. The whole block is
// carried, not a patch, so a client that missed deltas still ends up correct.
type BlockCompleted struct {
	Block Block
}

// PlanChanged carries the run's plan whenever it changes — always in full, for
// the same reason [BlockCompleted] is.
type PlanChanged struct {
	Items []PlanItem
}

// RunParked ends a stream without ending the run: the run is waiting on an
// answer. Resuming with [Runs.ResumeRun] continues it on a new stream.
type RunParked struct {
	Approval Approval
}

// RunFinished ends both the stream and the run.
type RunFinished struct {
	Outcome Outcome
	Usage   Usage
}

func (RunStarted) clientEvent()     {}
func (BlockStarted) clientEvent()   {}
func (BlockDelta) clientEvent()     {}
func (BlockCompleted) clientEvent() {}
func (PlanChanged) clientEvent()    {}
func (RunParked) clientEvent()      {}
func (RunFinished) clientEvent()    {}

package runs

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
	runfixture "github.com/Tangerg/scope/app/runtime/internal/testsupport/runfixture"
)

// fakeRunProjection is the durable answer to "what is this run", which is what a
// subscribe or a steer resolves through.
type fakeRunProjection struct {
	runs map[string]run.Run
	err  error
}

type racingRunProjection struct {
	value        run.Run
	beforeReturn func()
}

func (r *racingRunProjection) Run(context.Context, string) (run.Run, bool, error) {
	if r.beforeReturn != nil {
		r.beforeReturn()
	}
	return r.value, true, nil
}

func (*racingRunProjection) Tree(context.Context, string) ([]run.Run, error) { return nil, nil }

func (f *fakeRunProjection) Run(_ context.Context, runID string) (run.Run, bool, error) {
	if f.err != nil {
		return run.Run{}, false, f.err
	}
	run, ok := f.runs[runID]
	return run, ok, nil
}

func (f *fakeRunProjection) Tree(_ context.Context, runID string) ([]run.Run, error) {
	if f.err != nil {
		return nil, f.err
	}
	target, found := f.runs[runID]
	if !found {
		return nil, nil
	}
	rootRunID := target.Lineage().TreeRootID(target.ID())
	var tree []run.Run
	for _, record := range f.runs {
		if record.ID() == rootRunID || record.Lineage().RootRunID == rootRunID {
			tree = append(tree, record)
		}
	}
	return tree, nil
}

func runRecord(state run.State, activeSegmentID, spawnedBy string) run.Run {
	lineage := run.Lineage{SpawnedByItemID: spawnedBy}
	if spawnedBy != "" {
		lineage.ParentRunID = "run_parent"
		lineage.RootRunID = "run_root"
	}
	return runfixture.MustRestore(run.Snapshot{
		ID: testRunID, SessionID: "ses_1", State: state,
		ActiveSegmentID: activeSegmentID, Lineage: lineage,
	})
}

// liveCoordinator wires a Coordinator whose registry already holds one streaming
// segment, so a test can address it without driving a whole run.
func liveCoordinator(t *testing.T, record run.Run) (*Coordinator, *journal) {
	t.Helper()
	c := mustNewCoordinator(Dependencies{
		Releases: &fakeExecutionPorts{},
		Runs:     &fakeRunProjection{runs: map[string]run.Run{testRunID: record}},
	})
	hub := newJournal(streamScope{
		Epoch: c.segments.epoch, RunID: testRunID, SegmentID: testSegmentID,
	}, c.segments.retention)
	c.segments.open(Record{ID: testRunID, SegmentID: testSegmentID, SessionID: "ses_1", ExecutorID: "turn_1"},
		&runTreeOwner{hub: hub})
	return c, hub
}

// Every refusal names something different for the caller to do. Collapsing them
// into run_not_found — which is what a live-registry lookup alone can say — sends
// a client to look for an id that is right there.
func TestSubscribeRefusesWithTheReasonTheCallerCanActOn(t *testing.T) {
	for name, test := range map[string]struct {
		run       run.Run
		segmentID string
		want      error
	}{
		"child run": {
			run: runRecord(run.Running, testSegmentID, "item_9"), segmentID: testSegmentID,
			want: transcript.ErrNotRoot,
		},
		"waiting on a person": {
			run: runRecord(run.Waiting, "", ""), segmentID: testSegmentID,
			want: ErrRunWaiting,
		},
		"already finished": {
			run: runRecord(run.Completed, "", ""), segmentID: testSegmentID,
			want: ErrRunFinished,
		},
		"canceled counts as finished": {
			run: runRecord(run.Canceled, "", ""), segmentID: testSegmentID,
			want: ErrRunFinished,
		},
		// The case a resume creates: the run is still running, but not the segment
		// this caller was watching.
		"another segment": {
			run: runRecord(run.Running, "seg_2", ""), segmentID: testSegmentID,
			want: ErrStaleSegment,
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := liveCoordinator(t, test.run)
			_, err := c.Subscribe(t.Context(), SubscribeRequest{RunID: testRunID, SegmentID: test.segmentID})
			if !errors.Is(err, test.want) {
				t.Fatalf("Subscribe err = %v, want %v", err, test.want)
			}
			// A steer addresses the same thing, so it must refuse identically — the two
			// entry points into a running run cannot disagree about what it is doing.
			steerErr := c.Steer(t.Context(), SteerCommand{
				RunID:             testRunID,
				ExpectedSegmentID: test.segmentID,
				Input:             []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "wait"}},
			})
			if !errors.Is(steerErr, test.want) {
				t.Fatalf("Steer err = %v, want %v", steerErr, test.want)
			}
		})
	}
}

func TestSubscribeReportsAnUnknownRunAsNotFound(t *testing.T) {
	c := mustNewCoordinator(Dependencies{Runs: &fakeRunProjection{}})
	_, err := c.Subscribe(t.Context(), SubscribeRequest{RunID: "run_missing", SegmentID: testSegmentID})
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("Subscribe err = %v, want ErrRunNotFound", err)
	}
}

func TestSubscribeDoesNotRetargetAnOldSegmentToARacingResume(t *testing.T) {
	projection := &racingRunProjection{value: runRecord(run.Running, "segment_old", "")}
	coordinator := mustNewCoordinator(Dependencies{Runs: projection})
	oldHub := newJournal(streamScope{
		Epoch: coordinator.segments.epoch, RunID: testRunID, SegmentID: "segment_old",
	}, coordinator.segments.retention)
	newHub := newJournal(streamScope{
		Epoch: coordinator.segments.epoch, RunID: testRunID, SegmentID: "segment_new",
	}, coordinator.segments.retention)
	coordinator.segments.open(
		Record{ID: testRunID, SegmentID: "segment_old", SessionID: "ses_1", ExecutorID: "executor_old"},
		&runTreeOwner{hub: oldHub},
	)
	projection.beforeReturn = func() {
		coordinator.segments.open(
			Record{ID: testRunID, SegmentID: "segment_new", SessionID: "ses_1", ExecutorID: "executor_new"},
			&runTreeOwner{hub: newHub},
		)
	}

	_, err := coordinator.Subscribe(t.Context(), SubscribeRequest{
		RunID: testRunID, SegmentID: "segment_old",
	})
	if !errors.Is(err, ErrStaleSegment) {
		t.Fatalf("Subscribe across racing resume = %v, want ErrStaleSegment", err)
	}
	newHub.mu.Lock()
	newSubscribers := len(newHub.subs)
	newHub.mu.Unlock()
	if newSubscribers != 0 {
		t.Fatalf("old subscribe attached to replacement Segment: subscribers=%d", newSubscribers)
	}
}

func TestSubscribeWithoutACursorTailsAndNamesTheHead(t *testing.T) {
	c, hub := liveCoordinator(t, runRecord(run.Running, testSegmentID, ""))
	hub.append(ev(true)) // already published: history, not this subscription's

	attached, err := c.Subscribe(t.Context(), SubscribeRequest{RunID: testRunID, SegmentID: testSegmentID})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if attached.HeadCursor == "" {
		t.Fatal("head cursor is empty, want the position the tail starts after")
	}
	if attached.Record.SegmentID != testSegmentID {
		t.Fatalf("record segment = %q, want %q", attached.Record.SegmentID, testSegmentID)
	}
	hub.append(ev(true))
	hub.close()
	if got := drain(attached.Events); len(got) != 1 || got[0] != 2 {
		t.Fatalf("tail delivered %v, want only the event published after attaching", got)
	}
}

// The head cursor of one subscription is a legal cursor for the next: that round
// trip is the whole reconnect contract.
func TestSubscribeResumesFromTheHeadItWasHandedEarlier(t *testing.T) {
	c, hub := liveCoordinator(t, runRecord(run.Running, testSegmentID, ""))
	hub.append(ev(true))
	connection, drop := context.WithCancel(t.Context())
	first, err := c.Subscribe(connection, SubscribeRequest{RunID: testRunID, SegmentID: testSegmentID})
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}
	drop() // the connection goes away without the stream ever being read
	hub.append(ev(true))

	second, err := c.Subscribe(t.Context(), SubscribeRequest{
		RunID: testRunID, SegmentID: testSegmentID, Cursor: first.HeadCursor,
	})
	if err != nil {
		t.Fatalf("reconnect Subscribe: %v", err)
	}
	hub.close()
	if got := drain(second.Events); len(got) != 1 || got[0] != 2 {
		t.Fatalf("reconnect delivered %v, want the event missed while detached", got)
	}
}

// A cursor minted for another segment of the same run is refused rather than
// resolved: its sequence would name a real position in this stream, and serving it
// would hand the client a different execution's events under its own cursor.
func TestSubscribeRefusesACursorFromAnotherSegment(t *testing.T) {
	c, hub := liveCoordinator(t, runRecord(run.Running, testSegmentID, ""))
	hub.append(ev(true))
	other := newJournal(streamScope{Epoch: c.segments.epoch, RunID: testRunID, SegmentID: "seg_previous"}, c.segments.retention)
	other.append(ev(true))
	stale := other.tail().HeadCursor

	_, err := c.Subscribe(t.Context(), SubscribeRequest{
		RunID: testRunID, SegmentID: testSegmentID, Cursor: stale,
	})
	if !errors.Is(err, ErrReplayCursorInvalid) {
		t.Fatalf("Subscribe err = %v, want ErrReplayCursorInvalid", err)
	}
}

// Two Coordinators are two stream authorities, which is what a restart is: a
// cursor from the earlier one is unavailable, never silently rebound. The
// durable projection remains readable afterward, which is the cold truth a
// client combines with a new tail subscription.
func TestSubscribeRefusesACursorFromAnotherRuntimeInstance(t *testing.T) {
	previous, previousHub := liveCoordinator(t, runRecord(run.Running, testSegmentID, ""))
	previousHub.append(ev(true))
	before, err := previous.Subscribe(t.Context(), SubscribeRequest{RunID: testRunID, SegmentID: testSegmentID})
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}

	restarted, hub := liveCoordinator(t, runRecord(run.Running, testSegmentID, ""))
	hub.append(ev(true))
	_, err = restarted.Subscribe(t.Context(), SubscribeRequest{
		RunID: testRunID, SegmentID: testSegmentID, Cursor: before.HeadCursor,
	})
	if !errors.Is(err, ErrReplayUnavailable) {
		t.Fatalf("Subscribe err = %v, want ErrReplayUnavailable", err)
	}
	cold, found, queryErr := restarted.runs.Run(t.Context(), testRunID)
	if queryErr != nil || !found ||
		cold.State() != run.Running ||
		cold.ActiveSegmentID() != testSegmentID {
		t.Fatalf(
			"durable recovery after unavailable replay = found:%t Run:%+v err:%v",
			found,
			cold,
			queryErr,
		)
	}
}

// Real boot reconciliation runs before requests are served. If it has already
// settled an orphan as run_lost, lifecycle truth outranks cursor inspection:
// subscribe returns run_finished and the cold query exposes the terminal reason.
func TestSubscribeAfterOrphanRecoveryUsesFinishedStateBeforeOldCursor(t *testing.T) {
	previous, previousHub := liveCoordinator(t, runRecord(run.Running, testSegmentID, ""))
	previousHub.append(ev(true))
	before, err := previous.Subscribe(t.Context(), SubscribeRequest{
		RunID: testRunID, SegmentID: testSegmentID,
	})
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}

	outcome := run.OutcomeLost
	recovered := runfixture.MustRestore(run.Snapshot{
		ID: testRunID, SessionID: "ses_1", State: run.Failed,
		Outcome: &outcome, Failure: &run.Failure{Kind: run.FailureLost},
	})
	projection := &fakeRunProjection{runs: map[string]run.Run{testRunID: recovered}}
	restarted := mustNewCoordinator(Dependencies{Runs: projection})
	_, err = restarted.Subscribe(t.Context(), SubscribeRequest{
		RunID: testRunID, SegmentID: testSegmentID, Cursor: before.HeadCursor,
	})
	if !errors.Is(err, ErrRunFinished) {
		t.Fatalf("Subscribe after orphan recovery = %v, want ErrRunFinished", err)
	}
	cold, found, queryErr := projection.Run(t.Context(), testRunID)
	if failure, failed := cold.Failure(); queryErr != nil || !found ||
		!failed || failure.Kind != run.FailureLost {
		t.Fatalf(
			"cold recovery = found:%t Run:%+v err:%v, want terminal run_lost",
			found,
			cold,
			queryErr,
		)
	}
}

func TestSubscribeRefusesACallerThatCouldNotFollowTheRun(t *testing.T) {
	record := runRecord(run.Running, testSegmentID, "")
	c := mustNewCoordinator(Dependencies{
		Runs: &fakeRunProjection{runs: map[string]run.Run{testRunID: record}},
	})
	capabilities := run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval}}
	hub := newJournal(streamScope{Epoch: c.segments.epoch, RunID: testRunID, SegmentID: testSegmentID}, c.segments.retention)
	c.segments.open(Record{
		ID: testRunID, SegmentID: testSegmentID, SessionID: "ses_1", Capabilities: capabilities,
	}, &runTreeOwner{hub: hub})

	_, err := c.Subscribe(t.Context(), SubscribeRequest{RunID: testRunID, SegmentID: testSegmentID})
	if _, ok := errors.AsType[*run.InsufficientCapabilitiesError](err); !ok {
		t.Fatalf("Subscribe err = %v, want InsufficientCapabilitiesError", err)
	}
}

// A durable record that says Running while this process owns no stream for it is
// a broken invariant, not a state the client can act on: restart recovery
// terminalizes orphans before the runtime serves. Reporting it as a business
// refusal would teach the client something untrue about the run.
func TestSubscribeReportsAMissingLiveStreamAsAnInternalFault(t *testing.T) {
	c := mustNewCoordinator(Dependencies{Runs: &fakeRunProjection{
		runs: map[string]run.Run{testRunID: runRecord(run.Running, testSegmentID, "")},
	}})
	_, err := c.Subscribe(t.Context(), SubscribeRequest{RunID: testRunID, SegmentID: testSegmentID})
	switch {
	case err == nil:
		t.Fatal("Subscribe succeeded with no live stream")
	case errors.Is(err, ErrRunNotFound), errors.Is(err, ErrStaleSegment),
		errors.Is(err, ErrRunWaiting), errors.Is(err, ErrRunFinished):
		t.Fatalf("Subscribe err = %v, want an internal fault rather than a client-actionable refusal", err)
	}
}

func TestReplayRetentionIsWhatTheCoordinatorEnforces(t *testing.T) {
	c := mustNewCoordinator(Dependencies{})
	if c.ReplayRetention() != DefaultRetention() {
		t.Fatalf("retention = %+v, want %+v", c.ReplayRetention(), DefaultRetention())
	}
	custom := Retention{MaxEvents: 8, MaxBytes: 64}
	if got := mustNewCoordinator(Dependencies{Retention: custom}).ReplayRetention(); got != custom {
		t.Fatalf("retention = %+v, want %+v", got, custom)
	}
}

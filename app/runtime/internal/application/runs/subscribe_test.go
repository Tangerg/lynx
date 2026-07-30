package runs

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// fakeRunProjection is the durable answer to "what is this run", which is what a
// subscribe or a steer resolves through.
type fakeRunProjection struct {
	runs map[string]transcript.Run
	err  error
}

func (f *fakeRunProjection) Run(_ context.Context, runID string) (transcript.Run, bool, error) {
	if f.err != nil {
		return transcript.Run{}, false, f.err
	}
	run, ok := f.runs[runID]
	return run, ok, nil
}

func (f *fakeRunProjection) RunTree(_ context.Context, runID string) ([]transcript.Run, error) {
	if f.err != nil {
		return nil, f.err
	}
	target, found := f.runs[runID]
	if !found {
		return nil, nil
	}
	rootRunID := target.Lineage().TreeRootID(target.ID)
	var tree []transcript.Run
	for _, run := range f.runs {
		if run.ID == rootRunID || run.RootRunID == rootRunID {
			tree = append(tree, run)
		}
	}
	return tree, nil
}

func runRecord(state execution.RunState, activeSegmentID, spawnedBy string) transcript.Run {
	run := transcript.Run{
		ID: testRunID, SessionID: "ses_1", State: state,
		ActiveSegmentID: activeSegmentID, SpawnedByItemID: spawnedBy,
	}
	if spawnedBy != "" {
		run.ParentRunID = "run_parent"
		run.RootRunID = "run_root"
	}
	return run
}

// liveCoordinator wires a Coordinator whose registry already holds one streaming
// segment, so a test can address it without driving a whole run.
func liveCoordinator(t *testing.T, run transcript.Run) (*Coordinator, *Journal) {
	t.Helper()
	c := NewCoordinator(Dependencies{
		Turns: &fakeTurnControl{},
		Runs:  &fakeRunProjection{runs: map[string]transcript.Run{testRunID: run}},
	})
	hub := newJournal(streamScope{
		Epoch: c.epoch, RunID: testRunID, SegmentID: testSegmentID,
	}, c.retention)
	c.registry.Open(Record{ID: testRunID, SegmentID: testSegmentID, SessionID: "ses_1", TurnID: "turn_1"},
		&handle{hub: hub})
	return c, hub
}

// Every refusal names something different for the caller to do. Collapsing them
// into run_not_found — which is what a live-registry lookup alone can say — sends
// a client to look for an id that is right there.
func TestSubscribeRefusesWithTheReasonTheCallerCanActOn(t *testing.T) {
	for name, test := range map[string]struct {
		run       transcript.Run
		segmentID string
		want      error
	}{
		"child run": {
			run: runRecord(execution.Running, testSegmentID, "item_9"), segmentID: testSegmentID,
			want: transcript.ErrNotRoot,
		},
		"waiting on a person": {
			run: runRecord(execution.Interrupted, "", ""), segmentID: testSegmentID,
			want: ErrRunWaiting,
		},
		"already finished": {
			run: runRecord(execution.Completed, "", ""), segmentID: testSegmentID,
			want: ErrRunFinished,
		},
		"canceled counts as finished": {
			run: runRecord(execution.Canceled, "", ""), segmentID: testSegmentID,
			want: ErrRunFinished,
		},
		// The case a resume creates: the run is still running, but not the segment
		// this caller was watching.
		"another segment": {
			run: runRecord(execution.Running, "seg_2", ""), segmentID: testSegmentID,
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
	c := NewCoordinator(Dependencies{Runs: &fakeRunProjection{}})
	_, err := c.Subscribe(t.Context(), SubscribeRequest{RunID: "run_missing", SegmentID: testSegmentID})
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("Subscribe err = %v, want ErrRunNotFound", err)
	}
}

func TestSubscribeWithoutACursorTailsAndNamesTheHead(t *testing.T) {
	c, hub := liveCoordinator(t, runRecord(execution.Running, testSegmentID, ""))
	hub.Append(ev(true)) // already published: history, not this subscription's

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
	hub.Append(ev(true))
	hub.Close()
	if got := drain(attached.Events); len(got) != 1 || got[0] != 2 {
		t.Fatalf("tail delivered %v, want only the event published after attaching", got)
	}
}

// The head cursor of one subscription is a legal cursor for the next: that round
// trip is the whole reconnect contract.
func TestSubscribeResumesFromTheHeadItWasHandedEarlier(t *testing.T) {
	c, hub := liveCoordinator(t, runRecord(execution.Running, testSegmentID, ""))
	hub.Append(ev(true))
	connection, drop := context.WithCancel(t.Context())
	first, err := c.Subscribe(connection, SubscribeRequest{RunID: testRunID, SegmentID: testSegmentID})
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}
	drop() // the connection goes away without the stream ever being read
	hub.Append(ev(true))

	second, err := c.Subscribe(t.Context(), SubscribeRequest{
		RunID: testRunID, SegmentID: testSegmentID, Cursor: first.HeadCursor,
	})
	if err != nil {
		t.Fatalf("reconnect Subscribe: %v", err)
	}
	hub.Close()
	if got := drain(second.Events); len(got) != 1 || got[0] != 2 {
		t.Fatalf("reconnect delivered %v, want the event missed while detached", got)
	}
}

// A cursor minted for another segment of the same run is refused rather than
// resolved: its sequence would name a real position in this stream, and serving it
// would hand the client a different execution's events under its own cursor.
func TestSubscribeRefusesACursorFromAnotherSegment(t *testing.T) {
	c, hub := liveCoordinator(t, runRecord(execution.Running, testSegmentID, ""))
	hub.Append(ev(true))
	other := newJournal(streamScope{Epoch: c.epoch, RunID: testRunID, SegmentID: "seg_previous"}, c.retention)
	other.Append(ev(true))
	stale := other.Tail().HeadCursor

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
func TestSubscribeRefusesACursorFromAnotherProcess(t *testing.T) {
	previous, previousHub := liveCoordinator(t, runRecord(execution.Running, testSegmentID, ""))
	previousHub.Append(ev(true))
	before, err := previous.Subscribe(t.Context(), SubscribeRequest{RunID: testRunID, SegmentID: testSegmentID})
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}

	restarted, hub := liveCoordinator(t, runRecord(execution.Running, testSegmentID, ""))
	hub.Append(ev(true))
	_, err = restarted.Subscribe(t.Context(), SubscribeRequest{
		RunID: testRunID, SegmentID: testSegmentID, Cursor: before.HeadCursor,
	})
	if !errors.Is(err, ErrReplayUnavailable) {
		t.Fatalf("Subscribe err = %v, want ErrReplayUnavailable", err)
	}
	cold, found, queryErr := restarted.runs.Run(t.Context(), testRunID)
	if queryErr != nil || !found ||
		cold.State != execution.Running ||
		cold.ActiveSegmentID != testSegmentID {
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
	previous, previousHub := liveCoordinator(t, runRecord(execution.Running, testSegmentID, ""))
	previousHub.Append(ev(true))
	before, err := previous.Subscribe(t.Context(), SubscribeRequest{
		RunID: testRunID, SegmentID: testSegmentID,
	})
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}

	outcome := execution.OutcomeError
	recovered := runRecord(execution.Failed, "", "")
	recovered.Outcome = &outcome
	recovered.Error = &transcript.Problem{
		Kind:  transcript.RunLostProblem,
		Scope: transcript.RunProblem,
	}
	projection := &fakeRunProjection{runs: map[string]transcript.Run{testRunID: recovered}}
	restarted := NewCoordinator(Dependencies{Runs: projection})
	_, err = restarted.Subscribe(t.Context(), SubscribeRequest{
		RunID: testRunID, SegmentID: testSegmentID, Cursor: before.HeadCursor,
	})
	if !errors.Is(err, ErrRunFinished) {
		t.Fatalf("Subscribe after orphan recovery = %v, want ErrRunFinished", err)
	}
	cold, found, queryErr := projection.Run(t.Context(), testRunID)
	if queryErr != nil || !found ||
		cold.Error == nil ||
		cold.Error.Kind != transcript.RunLostProblem {
		t.Fatalf(
			"cold recovery = found:%t Run:%+v err:%v, want terminal run_lost",
			found,
			cold,
			queryErr,
		)
	}
}

func TestSubscribeRefusesACallerThatCouldNotFollowTheRun(t *testing.T) {
	run := runRecord(execution.Running, testSegmentID, "")
	c := NewCoordinator(Dependencies{
		Runs: &fakeRunProjection{runs: map[string]transcript.Run{testRunID: run}},
	})
	profile := execution.RunProtocolProfile{InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt}}
	hub := newJournal(streamScope{Epoch: c.epoch, RunID: testRunID, SegmentID: testSegmentID}, c.retention)
	c.registry.Open(Record{
		ID: testRunID, SegmentID: testSegmentID, SessionID: "ses_1", ProtocolProfile: profile,
	}, &handle{hub: hub})

	_, err := c.Subscribe(t.Context(), SubscribeRequest{RunID: testRunID, SegmentID: testSegmentID})
	if _, ok := errors.AsType[*execution.ProfileNotCovered](err); !ok {
		t.Fatalf("Subscribe err = %v, want ProfileNotCovered", err)
	}
}

// A durable record that says Running while this process owns no stream for it is
// a broken invariant, not a state the client can act on: restart recovery
// terminalizes orphans before the runtime serves. Reporting it as a business
// refusal would teach the client something untrue about the run.
func TestSubscribeReportsAMissingLiveStreamAsAnInternalFault(t *testing.T) {
	c := NewCoordinator(Dependencies{Runs: &fakeRunProjection{
		runs: map[string]transcript.Run{testRunID: runRecord(execution.Running, testSegmentID, "")},
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
	c := NewCoordinator(Dependencies{})
	if c.ReplayRetention() != DefaultRetention {
		t.Fatalf("retention = %+v, want %+v", c.ReplayRetention(), DefaultRetention)
	}
	custom := Retention{MaxEvents: 8, MaxBytes: 64}
	if got := NewCoordinator(Dependencies{Retention: custom}).ReplayRetention(); got != custom {
		t.Fatalf("retention = %+v, want %+v", got, custom)
	}
}

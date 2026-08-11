// Package runrecovery reconciles a dropped live segment with the runtime's
// authoritative cold projections.
package runrecovery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

const sessionAttachAttempts = 8

// Source is the narrow runtime surface needed for cold recovery.
type Source interface {
	agent.SessionReader
	SubscribeRun(context.Context, agent.SubscribeRun) (agent.SegmentStream, error)
}

// State is a coherent cold projection and, while its run is still executing,
// a stream attached before the final read. Stream is empty for waiting and
// finished runs.
type State struct {
	Snapshot agent.SessionSnapshot
	Run      agent.Run
	Stream   agent.SegmentStream
}

// Required reports whether a failed segment subscription must be reconciled
// from durable reads instead of retried with the same cursor.
func Required(err error) bool {
	return errors.Is(err, agent.ErrStaleSegment) ||
		errors.Is(err, agent.ErrRunWaiting) ||
		errors.Is(err, agent.ErrRunFinished) ||
		errors.Is(err, agent.ErrReplayCursorInvalid) ||
		errors.Is(err, agent.ErrReplayUnavailable)
}

// Recover follows the runtime's attach-then-read rule. For a live run it first
// attaches at the current segment head and only then performs the durable read,
// preventing an unobserved gap between the snapshot and later stream events.
func Recover(ctx context.Context, source Source, sessionID, runID string) (State, error) {
	first, run, err := read(ctx, source, sessionID, runID)
	if err != nil || run.Status != agent.RunStatusRunning {
		return State{Snapshot: first, Run: run}, err
	}
	stream, release, err := attach(ctx, source, run)
	if err != nil {
		return State{}, err
	}
	second, current, err := read(ctx, source, sessionID, runID)
	if err != nil {
		release()
		return State{}, err
	}
	if current.Status != agent.RunStatusRunning {
		release()
		return State{Snapshot: second, Run: current}, nil
	}
	if current.ActiveSegmentID != stream.SegmentID {
		release()
		return State{}, fmt.Errorf("%w: run %s changed from segment %s to %s during recovery", agent.ErrStaleSegment, runID, stream.SegmentID, current.ActiveSegmentID)
	}
	return State{Snapshot: second, Run: current, Stream: releaseWhenDone(stream, release)}, nil
}

// AttachSession obtains a coherent session projection and, when its current
// root Run is executing, a cursorless tail that was attached before the final
// cold read. It retries when another client crosses a Run or Segment boundary
// during that handshake.
func AttachSession(ctx context.Context, source Source, sessionID string) (State, error) {
	for range sessionAttachAttempts {
		first, err := readSnapshot(ctx, source, sessionID)
		if err != nil {
			return State{}, err
		}
		run, ok := first.ActiveRun()
		if !ok || run.Status != agent.RunStatusRunning {
			return stateWithoutStream(first), nil
		}

		stream, release, err := attach(ctx, source, run)
		if err != nil {
			if Required(err) {
				continue
			}
			return State{}, err
		}
		second, err := readSnapshot(ctx, source, sessionID)
		if err != nil {
			release()
			return State{}, err
		}
		current, active := second.ActiveRun()
		if !active || current.Status != agent.RunStatusRunning {
			release()
			return stateWithoutStream(second), nil
		}
		if current.ID != stream.RunID || current.ActiveSegmentID != stream.SegmentID {
			release()
			continue
		}
		return State{
			Snapshot: second,
			Run:      current,
			Stream:   releaseWhenDone(stream, release),
		}, nil
	}
	return State{}, fmt.Errorf("%w: session %s did not hold a stable active segment", agent.ErrStaleSegment, sessionID)
}

func attach(ctx context.Context, source Source, run agent.Run) (agent.SegmentStream, context.CancelFunc, error) {
	streamCtx, release := context.WithCancel(ctx)
	stream, err := source.SubscribeRun(streamCtx, agent.SubscribeRun{RunID: run.ID, SegmentID: run.ActiveSegmentID})
	if err != nil {
		release()
		return agent.SegmentStream{}, nil, err
	}
	if err := stream.ValidateSubscription(); err != nil {
		release()
		return agent.SegmentStream{}, nil, fmt.Errorf("recover run: %w", err)
	}
	return stream, release, nil
}

func releaseWhenDone(stream agent.SegmentStream, release context.CancelFunc) agent.SegmentStream {
	events := stream.Events
	stream.Events = func(yield func(agent.RunEvent, error) bool) {
		defer release()
		events(yield)
	}
	return stream
}

func stateWithoutStream(snapshot agent.SessionSnapshot) State {
	run, ok := snapshot.ActiveRun()
	if !ok {
		run, _ = snapshot.LatestRun()
	}
	return State{Snapshot: snapshot, Run: run}
}

func read(ctx context.Context, source agent.SessionReader, sessionID, runID string) (agent.SessionSnapshot, agent.Run, error) {
	snapshot, err := readSnapshot(ctx, source, sessionID)
	if err != nil {
		return agent.SessionSnapshot{}, agent.Run{}, err
	}
	run, ok := snapshot.RunByID(runID)
	if !ok {
		return agent.SessionSnapshot{}, agent.Run{}, fmt.Errorf("%w: %s", agent.ErrRunNotFound, runID)
	}
	return snapshot, run, nil
}

func readSnapshot(ctx context.Context, source agent.SessionReader, sessionID string) (agent.SessionSnapshot, error) {
	snapshot, err := source.GetSession(ctx, sessionID)
	if err != nil {
		return agent.SessionSnapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return agent.SessionSnapshot{}, fmt.Errorf("recover run: %w", err)
	}
	return snapshot, nil
}

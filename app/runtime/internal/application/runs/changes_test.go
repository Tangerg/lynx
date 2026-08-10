package runs

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
)

// changeRecorder collects notices from the pump goroutine as well as the request
// goroutine, so it is guarded.
type changeRecorder struct {
	mu      sync.Mutex
	notices []change.Notice
}

func (r *changeRecorder) publish(notice change.Notice) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notices = append(r.notices, notice)
}

func (r *changeRecorder) snapshot() []change.Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.notices)
}

func (r *changeRecorder) resources() []change.Resource {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]change.Resource, 0, len(r.notices))
	for _, notice := range r.notices {
		out = append(out, notice.Resource)
	}
	return out
}

func (r *changeRecorder) count(resource change.Resource) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, notice := range r.notices {
		if notice.Resource == resource {
			n++
		}
	}
	return n
}

// TestStartAndTerminalPublishRunAndSessionChanges: a window that is not following
// this run still shows it in a run list and a session list. Both ends of the run's
// life have to reach that window, or it shows work that finished minutes ago as
// still executing.
func TestStartAndTerminalPublishRunAndSessionChanges(t *testing.T) {
	exec := &fakeExecutor{}
	effects := &fakeEffects{}
	sessions := &fakeRunSessions{sess: sessionfixture.MustRestore(session.Snapshot{ID: "ses_1", CWD: "/work"})}
	control := &fakeExecutionPorts{startRef: ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}}
	changes := &changeRecorder{}
	c := NewCoordinator(Dependencies{
		RootStarts:   control,
		Observations: exec,
		Releases:     control,
		Conversation: emptyConversationReader{},
		Session:      testSessionPorts(sessions),
		Projection:   testProjectionPorts(effects),
		Now:          func() time.Time { return time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC) },
		NewRunID:     func() string { return "run_new" },
		NewSegmentID: func() string { return "seg_new" },
		Admissions:   new(admission.Gate),
		Changed:      changes.publish,
	})

	result, err := c.Start(t.Context(), StartCommand{
		SessionID: "ses_1",
		Input:     []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// The admission notice goes out with Start, before anything is consumed.
	if got := changes.resources(); !slices.Equal(got, []change.Resource{change.Runs, change.Sessions}) {
		t.Fatalf("published on start = %v, want the run and its session", got)
	}
	consumeEvents(result.Events)
	// Draining the stream ends the segment, and the synthesized terminal commits
	// before the journal closes — so by here the run is durably finished.
	if got := changes.count(change.Runs); got != 2 {
		t.Fatalf("run notices = %d, want one for the start and one for the terminal", got)
	}
	if got := changes.count(change.Sessions); got != 2 {
		t.Fatalf("session notices = %d, want one per run transition", got)
	}
	for _, notice := range changes.notices {
		if notice.Resource == change.Runs && !slices.Equal(notice.RunIDs, []string{"run_new"}) {
			t.Fatalf("run notice scope = %v, want [run_new]", notice.RunIDs)
		}
	}
}

// TestPlanSnapshotStaysOnOwningRunStream proves the run projection carries the
// committed value but does not claim ownership of its invalidation. The Plan
// use case publishes that notice immediately after its own durable CAS; this
// reducer only projects the resulting Tool fact onto the owning run stream.
func TestPlanSnapshotStaysOnOwningRunStream(t *testing.T) {
	exec := &fakeExecutor{events: []ExecutorPayload{PlanUpdated{State: testPlanState(t, plan.Snapshot{
		Steps:    []plan.Step{{Description: "tell the other window", Status: plan.StatusInProgress}},
		Revision: 2, UpdatedAt: time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC),
	})}}}
	changes := &changeRecorder{}
	control := &fakeExecutionPorts{startRef: ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}}
	sessions := &fakeRunSessions{sess: sessionfixture.MustRestore(session.Snapshot{ID: "ses_1", CWD: "/work"})}
	c := NewCoordinator(Dependencies{
		RootStarts:   control,
		Observations: exec,
		Releases:     control,
		Conversation: emptyConversationReader{},
		Session:      testSessionPorts(sessions),
		Projection:   testProjectionPorts(&fakeEffects{}),
		Now:          func() time.Time { return time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC) },
		NewRunID:     func() string { return "run_new" },
		NewSegmentID: func() string { return "seg_new" },
		Admissions:   new(admission.Gate),
		Changed:      changes.publish,
	})

	result, err := c.Start(t.Context(), StartCommand{
		SessionID: "ses_1",
		Input:     []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "plan"}},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	sawSnapshot := false
	for event := range result.Events {
		if _, ok := event.Payload.(StateSnapshot); ok {
			sawSnapshot = true
		}
	}
	if !sawSnapshot {
		t.Fatal("the run stream carried no state snapshot; the fixture proves nothing about the notice beside it")
	}
	if got := changes.count(change.PlanState); got != 0 {
		t.Fatalf("run projection published %d Plan notices; application/plans owns them", got)
	}
}

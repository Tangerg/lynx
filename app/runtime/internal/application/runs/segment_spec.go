package runs

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// segmentSpec is the prepared input to the package's segment supervisor.
type segmentSpec struct {
	// RunID is stable across every resumed Segment of the same logical Run.
	RunID string
	// SegmentID changes for every start or resume and scopes replay.
	SegmentID        string
	SessionID        string
	CWD              string
	ExecutorID       string
	ModelSelection   modelref.Selection
	GoalLeaseID      string
	ScheduledSession *session.Session
	SessionModel     *SessionModelUpdate
	ScheduleFiring   string
	CreatedAt        time.Time
	OpeningUserText  string
	Input            []transcript.ContentBlock
	// Limits and Capabilities are admission policy for a fresh Run. A
	// continuation reads the frozen values carried by Continuation.
	Limits       run.Limits
	Capabilities run.Capabilities
	Continuation *treeContinuation
	// admission transfers the pre-commit reservation to the live Run only after
	// its opening write-set commits.
	admission *admission.RunAdmission
	// BeginExecution crosses the executor side-effect boundary after opening commits.
	BeginExecution func(context.Context) error
	// CommitOpening is set only when a larger application transaction owns the
	// opening, such as waiting-subtree cancellation.
	CommitOpening func(context.Context, OpeningCommit) error
}

func (s segmentSpec) executorRef() ExecutorRef {
	return ExecutorRef{SessionID: s.SessionID, ExecutorID: s.ExecutorID}
}

func (s segmentSpec) priorMetrics() transcript.RunMetrics {
	if s.Continuation == nil {
		return transcript.RunMetrics{}
	}
	root, _ := s.Continuation.root()
	return root.Metrics
}

func (s segmentSpec) effectiveLimits() run.Limits {
	if s.Continuation == nil {
		return s.Limits
	}
	root, _ := s.Continuation.root()
	return root.Limits
}

func (s segmentSpec) effectiveCapabilities() run.Capabilities {
	if s.Continuation == nil {
		return s.Capabilities
	}
	return s.Continuation.capabilities
}

package runs

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessionadmission"
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
	SegmentID          string
	SessionID          string
	CWD                string
	ExecutorID         string
	ModelSelection     modelref.Selection
	GoalIncarnationID  string
	InitialSession     *session.Session
	SessionReplacement *SessionReplacement
	ScheduleFiring     string
	CreatedAt          time.Time
	OpeningUserText    string
	Input              []transcript.ContentBlock
	// ModelOnlyInput keeps an application-authored control message in the
	// provider conversation without publishing it as a user-visible transcript
	// Item. Only a fresh autonomous Goal root may set it; resumed input remains
	// human-authored and visible even when the Run belongs to a Goal.
	ModelOnlyInput bool
	// Limits and Capabilities are admission policy for a fresh Run. A
	// continuation reads the frozen values carried by Continuation.
	Limits       run.Limits
	Capabilities run.Capabilities
	Continuation *treeContinuation
	// admission transfers the pre-commit reservation to the live Run only after
	// its opening write-set commits.
	admission *sessionadmission.RunAdmission
	// BeginExecution crosses the executor side-effect boundary after opening commits.
	BeginExecution func(context.Context) error
	// DetachActivation is true for root Start/Resume commands whose durable
	// opening is their acceptance point. Their executor activation remains owned
	// by the Run lifecycle task but cannot retain the already-committed command
	// settlement. Composite commands leave this false when BeginExecution also
	// applies a post-commit Application transformation they must validate inline.
	DetachActivation bool
	// CommitOpening is set only when a larger application transaction owns the
	// opening, such as waiting-subtree cancellation.
	CommitOpening func(context.Context, OpeningCommit) error
}

func (s segmentSpec) executorRef() ExecutorRef {
	return ExecutorRef{SessionID: s.SessionID, ExecutorID: s.ExecutorID}
}

func (s segmentSpec) priorMetrics() run.Metrics {
	if s.Continuation == nil {
		return run.Metrics{}
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

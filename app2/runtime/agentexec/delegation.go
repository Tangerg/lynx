package agentexec

import (
	"context"
	"errors"
	"strings"
	"time"
)

const DelegateToolName = "delegate_task"

// DelegateTask is the model-facing semantic contract. It intentionally says
// only what work should be isolated; Framework and Lyra identities never enter
// model arguments.
type DelegateTask struct {
	Summary      string `json:"summary"`
	Instructions string `json:"instructions"`
}

func (task DelegateTask) Validate() error {
	if strings.TrimSpace(task.Summary) == "" || strings.TrimSpace(task.Instructions) == "" ||
		strings.TrimSpace(task.Summary) != task.Summary || strings.TrimSpace(task.Instructions) != task.Instructions ||
		len(task.Summary) > 512 || len(task.Instructions) > 64<<10 {
		return errors.New("agentexec: invalid delegated task")
	}
	return nil
}

type DelegateBinding struct {
	RunID, SegmentID, ParentRunID, RootRunID string
}

func (binding DelegateBinding) Valid() bool {
	return binding.RunID != "" && binding.SegmentID != "" && binding.ParentRunID != "" &&
		binding.RootRunID != "" && binding.RunID != binding.ParentRunID && binding.RunID != binding.RootRunID
}

// DelegateRequest is the app2-owned admission command translated from one
// exact Framework child relation and its already-observed model ToolCall.
type DelegateRequest struct {
	MemberID, ParentMemberID, ChildKey string
	ParentRunID, ParentSegmentID       string
	CallID                             string
	Task                               DelegateTask
	RequestedAt, StartedAt             time.Time
}

func (request DelegateRequest) Validate() error {
	if request.MemberID == "" || request.ParentMemberID == "" || request.ChildKey == "" ||
		request.MemberID == request.ParentMemberID || request.ParentRunID == "" ||
		request.ParentSegmentID == "" || request.CallID == "" ||
		request.RequestedAt.IsZero() || request.StartedAt.IsZero() {
		return errors.New("agentexec: invalid delegate admission request")
	}
	return request.Task.Validate()
}

type DelegateStartOutcome struct {
	MemberID string
	Started  bool
	Failure  string
}

// DelegationCoordinator is implemented by the Run application service. The
// adapter may supply Framework identities as opaque strings, but the port
// returns only product identities and owns all durable transactions.
type DelegationCoordinator interface {
	ReserveDelegate(context.Context, DelegateRequest) (DelegateBinding, error)
	ConcludeDelegateStart(context.Context, DelegateStartOutcome) (DelegateBinding, error)
}

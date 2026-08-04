package toolset

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/delegation"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// Semantics interprets concrete built-in calls for runtime policy. Unknown
// tools are treated as arbitrary execution and carry no remembered-rule subject.
type Semantics struct {
	plans planStateReader
}

type planStateReader interface {
	State(ctx context.Context, sessionID string) (plan.State, error)
}

// NewSemantics binds projections that require canonical tool-owned state. A
// nil Plan reader disables only the successful Plan replacement projection;
// safety, approval, and hook policy remain available.
func NewSemantics(plans planStateReader) Semantics {
	return Semantics{plans: plans}
}

var safetyClassByName = map[string]tool.SafetyClass{
	toolNameRead:                tool.SafetyClassSafe,
	toolNameGlob:                tool.SafetyClassSafe,
	toolNameGrep:                tool.SafetyClassSafe,
	toolNameLSP:                 tool.SafetyClassSafe,
	toolNameReadShellOutput:     tool.SafetyClassSafe,
	toolNameListSchedules:       tool.SafetyClassSafe,
	toolNameListSkills:          tool.SafetyClassSafe,
	toolNameLoadSkill:           tool.SafetyClassSafe,
	toolNameReadSkillResource:   tool.SafetyClassSafe,
	toolNameSearchMemory:        tool.SafetyClassSafe,
	toolNameSearchConversations: tool.SafetyClassSafe,
	toolNameSearchTools:         tool.SafetyClassSafe,
	toolNameAskUser:             tool.SafetyClassSafe,
	toolNameEnterPlanMode:       tool.SafetyClassSafe,
	toolNameExitPlanMode:        tool.SafetyClassSafe,
	toolNameSetPlan:             tool.SafetyClassSafe,
	toolNameReadToolResult:      tool.SafetyClassSafe,
	delegation.Name:             tool.SafetyClassSafe,
	toolNameCreateGoal:          tool.SafetyClassSafe,
	toolNameGetGoal:             tool.SafetyClassSafe,
	toolNameReportGoalOutcome:   tool.SafetyClassSafe,
	toolNameProposeSkill:        tool.SafetyClassSafe,

	toolNameWrite:          tool.SafetyClassWrite,
	toolNameEdit:           tool.SafetyClassWrite,
	toolNameApplyPatch:     tool.SafetyClassWrite,
	toolNameCreateSchedule: tool.SafetyClassWrite,
	toolNameDeleteSchedule: tool.SafetyClassWrite,

	toolNameShell:     tool.SafetyClassExec,
	toolNameStopShell: tool.SafetyClassExec,

	toolNameWebFetch:    tool.SafetyClassNetwork,
	toolNameWebSearch:   tool.SafetyClassNetwork,
	toolNameHTTPRequest: tool.SafetyClassNetwork,
}

// SafetyClass returns the call's side-effect class. Unknown tools fail closed
// because an extension can execute arbitrary work even when its name suggests a read.
func (Semantics) SafetyClass(name string) tool.SafetyClass {
	if class, ok := safetyClassByName[name]; ok {
		return class
	}
	return tool.SafetyClassExec
}

// UsesStandardPolicy reports whether a call enters the ordinary approval and
// PreToolUse/PostToolUse pipeline. Delegation is orchestration: every child
// tool is independently evaluated, while the parent call is represented by
// child lifecycle events and cannot be replayed as an ordinary tool gate.
func (Semantics) UsesStandardPolicy(name string) bool {
	return name != delegation.Name
}

// ApprovalSubject returns the stable identity used by remembered approval
// rules. Tools without a finer-grained identity deliberately return an empty
// subject, which means the rule covers the whole tool.
func (Semantics) ApprovalSubject(name string, arguments tool.Arguments) (string, error) {
	var field string
	switch name {
	case toolNameShell:
		field = "command"
	case toolNameRead, toolNameWrite, toolNameEdit:
		field = "path"
	default:
		return "", nil
	}
	subject, ok := arguments.StringField(field)
	if !ok || strings.TrimSpace(subject) == "" {
		return "", fmt.Errorf("toolset: tool %q requires non-empty string argument %q: %w", name, field, tool.ErrInvalidArguments)
	}
	return subject, nil
}

// ShellCommand returns the command text used by catastrophic-command policy.
// Malformed arguments return an empty command and remain gated by the tool's
// execution safety class.
func (Semantics) ShellCommand(name, rawArguments string) string {
	if name != toolNameShell {
		return ""
	}
	arguments, err := tool.ParseArguments(rawArguments)
	if err != nil {
		return ""
	}
	command, _ := arguments.StringField("command")
	return command
}

// ProjectOutcome returns the application event implied by a completed tool
// call. It reads canonical state rather than decoding a private tool result, so
// the client projection cannot drift from the state the tool actually wrote.
func (s Semantics) ProjectOutcome(
	ctx context.Context,
	sessionID, name string,
	succeeded bool,
) (runs.EngineEvent, error) {
	if !succeeded || name != toolNameSetPlan || s.plans == nil {
		return nil, nil
	}
	state, err := s.plans.State(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("toolset: project Plan replacement: %w", err)
	}
	return runs.PlanUpdated{State: state}, nil
}

func classifiedToolNames() []string {
	names := make([]string, 0, len(safetyClassByName))
	for name := range safetyClassByName {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

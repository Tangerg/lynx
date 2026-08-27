package toolset

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/domain/plan"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
)

// Interpreter interprets concrete built-in calls for runtime policy. Unknown
// tools are treated as arbitrary execution and carry no remembered-rule subject.
type Interpreter struct {
	plans planStateReader
}

type planStateReader interface {
	State(ctx context.Context, sessionID string) (plan.State, error)
}

// NewInterpreter binds projections that require canonical application state. A
// nil Plan reader disables only the successful Plan replacement projection;
// safety, approval, and hook policy remain available.
func NewInterpreter(plans planStateReader) Interpreter {
	return Interpreter{plans: plans}
}

// SafetyClass returns the call's side-effect class. Unknown tools fail closed
// because an extension can execute arbitrary work even when its name suggests a read.
func (Interpreter) SafetyClass(name string) tool.SafetyClass {
	if descriptor, ok := descriptorFor(name); ok {
		return descriptor.safety
	}
	return tool.SafetyClassExec
}

// UsesStandardPolicy reports whether a call enters the ordinary approval and
// PreToolUse/PostToolUse pipeline. Delegation is orchestration: every child
// tool is independently evaluated, while the parent call is represented by
// child lifecycle events and cannot be replayed as an ordinary tool gate.
func (Interpreter) UsesStandardPolicy(name string) bool {
	descriptor, ok := descriptorFor(name)
	return !ok || !descriptor.orchestration
}

// ApprovalSubject returns the stable identity used by remembered approval
// rules. Tools without a finer-grained identity deliberately return an empty
// subject, which means the rule covers the whole tool.
func (Interpreter) ApprovalSubject(name string, arguments tool.Arguments) (string, error) {
	var field string
	switch name {
	case tool.Shell:
		field = "command"
	case tool.Read:
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
func (Interpreter) ShellCommand(name, rawArguments string) string {
	if name != tool.Shell {
		return ""
	}
	arguments, err := tool.ParseArguments(rawArguments)
	if err != nil {
		return ""
	}
	command, _ := arguments.StringField("command")
	return command
}

// ProjectOutcome returns the application fact implied by a completed tool
// call. It reads canonical state rather than decoding a private tool result, so
// the published fact cannot drift from the state the tool actually wrote.
func (i Interpreter) ProjectOutcome(
	ctx context.Context,
	sessionID, name string,
	succeeded bool,
) (runs.ExecutionFact, error) {
	descriptor, known := descriptorFor(name)
	if !succeeded || !known || descriptor.outcome != planOutcomeProjection || i.plans == nil {
		return nil, nil
	}
	state, err := i.plans.State(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("toolset: project Plan replacement: %w", err)
	}
	return runs.PlanUpdated{State: state}, nil
}

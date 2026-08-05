package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// ToolInterpreter translates concrete tool names and argument schemas into the
// domain facts used by approval and transcript projection.
type ToolInterpreter interface {
	SafetyClass(toolName string) tool.SafetyClass
	UsesStandardPolicy(toolName string) bool
	ApprovalSubject(toolName string, arguments tool.Arguments) (string, error)
	ShellCommand(toolName, arguments string) string
	ProjectOutcome(
		ctx context.Context,
		sessionID, toolName string,
		succeeded bool,
	) (runs.ExecutionFact, error)
}

func (s *controller) toolSafetyClass(name string) tool.SafetyClass {
	if s.toolInterpreter == nil {
		return tool.SafetyClassExec
	}
	return s.toolInterpreter.SafetyClass(name)
}

func (s *controller) toolUsesStandardPolicy(name string) bool {
	return s.toolInterpreter == nil || s.toolInterpreter.UsesStandardPolicy(name)
}

func (s *controller) approvalSubject(name string, arguments tool.Arguments) (string, error) {
	if s.toolInterpreter == nil {
		return "", nil
	}
	return s.toolInterpreter.ApprovalSubject(name, arguments)
}

func (s *controller) shellCommand(name, arguments string) string {
	if s.toolInterpreter == nil {
		return ""
	}
	return s.toolInterpreter.ShellCommand(name, arguments)
}

func (s *controller) projectToolOutcome(
	ctx context.Context,
	sessionID, name string,
	succeeded bool,
) (runs.ExecutionFact, error) {
	if s.toolInterpreter == nil {
		return nil, nil
	}
	return s.toolInterpreter.ProjectOutcome(ctx, sessionID, name, succeeded)
}

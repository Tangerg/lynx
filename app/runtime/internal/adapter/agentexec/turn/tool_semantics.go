package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func (s *controller) toolSafetyClass(name string) tool.SafetyClass {
	if s.toolSemantics == nil {
		return tool.SafetyClassExec
	}
	return s.toolSemantics.SafetyClass(name)
}

func (s *controller) toolUsesStandardPolicy(name string) bool {
	return s.toolSemantics == nil || s.toolSemantics.UsesStandardPolicy(name)
}

func (s *controller) approvalSubject(name string, arguments tool.Arguments) (string, error) {
	if s.toolSemantics == nil {
		return "", nil
	}
	return s.toolSemantics.ApprovalSubject(name, arguments)
}

func (s *controller) shellCommand(name, arguments string) string {
	if s.toolSemantics == nil {
		return ""
	}
	return s.toolSemantics.ShellCommand(name, arguments)
}

func (s *controller) projectToolOutcome(
	ctx context.Context,
	sessionID, name string,
	succeeded bool,
) (runs.EngineEvent, error) {
	if s.toolSemantics == nil {
		return nil, nil
	}
	return s.toolSemantics.ProjectOutcome(ctx, sessionID, name, succeeded)
}

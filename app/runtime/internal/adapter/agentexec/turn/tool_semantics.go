package turn

import "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"

func (s *controller) toolSafetyClass(name string) tool.SafetyClass {
	if s.toolSemantics == nil {
		return tool.SafetyClassExec
	}
	return s.toolSemantics.SafetyClass(name)
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

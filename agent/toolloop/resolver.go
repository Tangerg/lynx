package toolloop

import (
	"fmt"

	"github.com/Tangerg/lynx/tools"
)

// ToolResolver resolves the executable tool advertised by a model request.
// Resolve must return a non-nil Tool whenever ok is true.
//
// The interface lives in the consuming package. tools.Registry is one
// implementation; applications may provide scoped or policy-aware resolvers.
type ToolResolver interface {
	Resolve(name string) (tools.Tool, bool)
}

var _ ToolResolver = (*tools.Registry)(nil)

func (s *runnerState) validateInput() error {
	if s == nil || s.request == nil {
		return fmt.Errorf("%w: request must not be nil", ErrInvalidInput)
	}
	if err := s.request.Validate(); err != nil {
		return fmt.Errorf("%w: request: %w", ErrInvalidInput, err)
	}
	if len(s.request.Tools) == 0 {
		return nil
	}
	if valueIsNil(s.resolver) {
		return fmt.Errorf("%w: request advertises tools but resolver is nil", ErrInvalidInput)
	}
	for _, definition := range s.request.Tools {
		tool, ok := s.resolver.Resolve(definition.Name)
		if !ok || valueIsNil(tool) {
			return fmt.Errorf("%w: advertised tool %q is not executable", ErrInvalidInput, definition.Name)
		}
		if !sameToolDefinition(definition, tool.Definition()) {
			return fmt.Errorf("%w: advertised tool %q definition does not match executable tool", ErrInvalidInput, definition.Name)
		}
	}
	return nil
}

package toolloop

import (
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
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

func validateRunInput(request *chat.Request, resolver ToolResolver) error {
	if request == nil {
		return fmt.Errorf("%w: request must not be nil", ErrInvalidInput)
	}
	if err := request.Validate(); err != nil {
		return fmt.Errorf("%w: request: %w", ErrInvalidInput, err)
	}
	if len(request.Tools) == 0 {
		return nil
	}
	if nilResolver(resolver) {
		return fmt.Errorf("%w: request advertises tools but resolver is nil", ErrInvalidInput)
	}
	for _, definition := range request.Tools {
		tool, ok := resolver.Resolve(definition.Name)
		if !ok || nilRuntimeTool(tool) {
			return fmt.Errorf("%w: advertised tool %q is not executable", ErrInvalidInput, definition.Name)
		}
		if !sameToolDefinition(definition, tool.Definition()) {
			return fmt.Errorf("%w: advertised tool %q definition does not match executable tool", ErrInvalidInput, definition.Name)
		}
	}
	return nil
}

package toolloop

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

var (
	// ErrInvalidInvocation reports a missing or inconsistent runtime input.
	ErrInvalidInvocation = errors.New("toolloop: invalid invocation")
	// ErrInvocationNotSerializable reports an attempt to encode runtime state as
	// provider protocol data.
	ErrInvocationNotSerializable = errors.New("toolloop: invocation is not serializable")
)

// ToolResolver is the single tool capability consumed by Invocation and the
// tool-loop runtime. Resolve must return a non-nil Tool whenever ok is true.
//
// The interface lives in the consuming package; tools.Registry is one concrete
// implementation, but applications may supply a scoped or policy-aware
// resolver without changing Invocation.
type ToolResolver interface {
	Resolve(name string) (tools.Tool, bool)
}

var _ ToolResolver = (*tools.Registry)(nil)

// Invocation combines a serializable provider Request with the runtime-only
// capability needed to execute its advertised tools. The resolver is adjacent
// to the Request, never embedded inside it.
type Invocation struct {
	Request *chat.Request
	Tools   ToolResolver
}

// NewInvocation validates request and its advertised tool names before
// returning an Invocation. A nil resolver is valid when request advertises no
// tools.
func NewInvocation(request *chat.Request, resolver ToolResolver) (*Invocation, error) {
	invocation := &Invocation{Request: request, Tools: resolver}
	if err := invocation.Validate(); err != nil {
		return nil, err
	}
	return invocation, nil
}

// Validate verifies the protocol request and ensures every advertised tool can
// be resolved to a non-nil executable capability.
func (i *Invocation) Validate() error {
	if i == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidInvocation)
	}
	if i.Request == nil {
		return fmt.Errorf("%w: missing request", ErrInvalidInvocation)
	}
	if err := i.Request.Validate(); err != nil {
		return fmt.Errorf("%w: request: %w", ErrInvalidInvocation, err)
	}
	if len(i.Request.Tools) == 0 {
		return nil
	}
	if nilResolver(i.Tools) {
		return fmt.Errorf("%w: request advertises tools but resolver is nil", ErrInvalidInvocation)
	}
	for _, definition := range i.Request.Tools {
		tool, ok := i.Tools.Resolve(definition.Name)
		if !ok || nilRuntimeTool(tool) {
			return fmt.Errorf("%w: advertised tool %q is not executable", ErrInvalidInvocation, definition.Name)
		}
	}
	return nil
}

// MarshalJSON rejects serialization because Invocation contains executable
// runtime state. Serialize Request and Event values instead.
func (Invocation) MarshalJSON() ([]byte, error) {
	return nil, ErrInvocationNotSerializable
}

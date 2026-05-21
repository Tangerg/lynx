package tool

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/lyra/internal/engine"
)

// New returns the [Service] implementation backed by engine. List
// snapshots the engine's registered tools; Invoke routes by tool
// name to the registered tool's Call method (no agent loop involved
// — direct synchronous invocation).
func New(eng *engine.Engine) Service {
	if eng == nil {
		panic("tool: engine is required")
	}
	return &impl{engine: eng}
}

type impl struct {
	engine *engine.Engine
}

func (s *impl) List(_ context.Context) ([]Tool, error) {
	chatTools := s.engine.Tools()
	out := make([]Tool, 0, len(chatTools))
	for _, t := range chatTools {
		def := t.Definition()
		out = append(out, Tool{
			Name:        def.Name,
			Description: def.Description,
			Schema:      def.InputSchema,
			SafetyClass: defaultSafetyClass(def.Name),
		})
	}
	return out, nil
}

func (s *impl) Invoke(ctx context.Context, name string, arguments string) (string, error) {
	if name == "" {
		return "", errors.New("tool: name must not be empty")
	}
	for _, t := range s.engine.Tools() {
		if t.Definition().Name == name {
			return t.Call(ctx, arguments)
		}
	}
	return "", fmt.Errorf("tool: %q not registered", name)
}

// defaultSafetyClass maps a tool name to its built-in default safety
// classification. Centralised here so the SafetyClass-by-name table
// has a single source of truth. M4 will let users override per-tool
// via config.
func defaultSafetyClass(name string) SafetyClass {
	switch name {
	case "read", "glob", "grep":
		return SafetyClassSafe
	case "write", "edit":
		return SafetyClassWrite
	case "bash":
		return SafetyClassExec
	default:
		// Unknown tool — treat as Exec until proven otherwise.
		return SafetyClassExec
	}
}


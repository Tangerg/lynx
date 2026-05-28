package tool

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
)

// Source is the narrow surface tool.Service consumes: just a
// snapshot of the currently-registered chat tools. *engine.Engine
// satisfies it implicitly via its Tools() accessor; tests pass a
// stub that returns a fixed slice without needing a real platform.
type Source interface {
	Tools() []chat.Tool
}

// New returns the [Service] implementation backed by src. List
// snapshots the registered tools; Invoke routes by tool name to
// the registered tool's Call method (no agent loop involved —
// direct synchronous invocation).
func New(src Source) Service {
	if src == nil {
		panic("tool: source is required")
	}
	return &engineBacked{src: src}
}

// engineBacked is the single Service implementation today. The
// "engine-backed" label is descriptive — the source is typically
// the engine but could be any Source (tests, mocks).
type engineBacked struct {
	src Source
}

func (s *engineBacked) List(_ context.Context) ([]Tool, error) {
	chatTools := s.src.Tools()
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

func (s *engineBacked) Invoke(ctx context.Context, name string, arguments string) (string, error) {
	if name == "" {
		return "", errors.New("tool: name must not be empty")
	}
	for _, t := range s.src.Tools() {
		if t.Definition().Name == name {
			return t.Call(ctx, arguments)
		}
	}
	return "", fmt.Errorf("tool: %q not registered", name)
}

// defaultSafetyClass maps a tool name to its built-in default safety
// classification. Centralized here so the SafetyClass-by-name table
// has a single source of truth. A future milestone may let users
// override per-tool via config.
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

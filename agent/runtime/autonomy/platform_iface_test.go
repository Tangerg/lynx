package autonomy

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// stubPlatform satisfies the unexported platform interface without
// touching the real *runtime.Platform. Its existence ensures the
// interface stays narrow — if Autonomy ever regrows a wider
// dependency on Platform, this stub stops compiling.
type stubPlatform struct {
	agents []*core.Agent
}

func (s *stubPlatform) Agents() []*core.Agent { return s.agents }

func (s *stubPlatform) RunAgent(
	_ context.Context,
	_ *core.Agent,
	_ map[string]any,
	_ core.ProcessOptions,
) (*runtime.AgentProcess, error) {
	return nil, nil
}

// TestPlatformInterfaceStaysNarrow is a compile-time check: if the
// unexported platform interface gains a method that *stubPlatform
// doesn't implement, this file fails to build. Cheap regression
// tripwire keeping the autonomy-to-platform coupling honest.
func TestPlatformInterfaceStaysNarrow(t *testing.T) {
	var _ platform = (*stubPlatform)(nil)
}

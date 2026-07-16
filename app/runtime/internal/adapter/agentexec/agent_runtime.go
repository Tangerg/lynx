package agentexec

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	agentruntime "github.com/Tangerg/lynx/agent/runtime"
)

var (
	_ processStarter  = (*agentruntime.Engine)(nil)
	_ processRestorer = (*agentruntime.Engine)(nil)
	_ processControl  = (*agentruntime.Engine)(nil)
)

type processStarter interface {
	Start(context.Context, *core.Agent, map[string]any, core.ProcessOptions) (*agentruntime.Process, <-chan error)
}

type processRestorer interface {
	Resumable(context.Context, string) (bool, error)
	RestoreResumable(context.Context, string, core.ProcessOptions) (*agentruntime.Process, error)
}

type processControl interface {
	Kill(string) error
	Resume(string, string, any) error
	ContinueAsync(context.Context, string) <-chan error
	Remove(string) error
	ProcessStore() core.ProcessStore
}

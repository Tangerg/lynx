package kernel

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	agentruntime "github.com/Tangerg/lynx/agent/runtime"
)

var (
	_ processStarter  = (*agentruntime.Platform)(nil)
	_ processRestorer = (*agentruntime.Platform)(nil)
	_ processControl  = (*agentruntime.Platform)(nil)
)

type processStarter interface {
	StartAgent(context.Context, *core.Agent, map[string]any, core.ProcessOptions) (*agentruntime.AgentProcess, <-chan error)
}

type processRestorer interface {
	RestoreProcess(context.Context, string, core.ProcessOptions) (*agentruntime.AgentProcess, error)
	ContinueProcess(context.Context, string) error
}

type processControl interface {
	KillProcess(string) error
	ResumeProcess(string, any) (core.ResponseImpact, error)
	ContinueProcessAsync(context.Context, string) <-chan error
	RemoveProcess(string) error
	ProcessStore() core.ProcessStore
}

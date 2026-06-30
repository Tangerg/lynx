package kernel

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	agentruntime "github.com/Tangerg/lynx/agent/runtime"
)

var _ agentRuntime = (*agentruntime.Platform)(nil)

type agentRuntime interface {
	processRunner
	processControl

	Deploy(*core.Agent) error
}

type processRunner interface {
	StartAgent(context.Context, *core.Agent, map[string]any, core.ProcessOptions) (*agentruntime.AgentProcess, <-chan error)
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

package mock

import "github.com/Tangerg/lynx/app/cli/internal/agent"

func projectRun(run *runState) agent.Run {
	return agent.Run{
		ID: run.id, SessionID: run.sessionID, Provider: run.provider, Model: run.model,
		Status: run.status, ActiveSegmentID: run.active, Limits: run.limits, Outcome: run.outcome, Usage: run.usage.Clone(),
	}
}

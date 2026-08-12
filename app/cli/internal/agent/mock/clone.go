package mock

import "github.com/Tangerg/lynx/app/cli/internal/agent"

func projectRun(run *runState) agent.Run {
	return agent.Run{
		ID: run.id, SessionID: run.sessionID, Provider: run.provider, Model: run.model,
		Lineage: run.lineage, Status: run.status, ActiveSegmentID: run.active,
		Limits: run.limits, Outcome: run.outcome.Clone(), Usage: run.usage.Clone(),
	}
}

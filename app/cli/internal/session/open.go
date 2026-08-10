// Package session owns the application use cases around durable conversations.
package session

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type runtime interface {
	CreateSession(context.Context, agent.CreateSession) (agent.Session, error)
	GetSession(context.Context, string) (agent.SessionSnapshot, error)
}

// Open restores the selected session or creates a new one in workspace.
func Open(ctx context.Context, rt runtime, id, workspace string) (agent.SessionSnapshot, error) {
	if id != "" {
		snapshot, err := rt.GetSession(ctx, id)
		if err != nil {
			return agent.SessionSnapshot{}, fmt.Errorf("open session: %w", err)
		}
		if err := snapshot.Validate(); err != nil {
			return agent.SessionSnapshot{}, fmt.Errorf("open session: %w", err)
		}
		return snapshot, nil
	}

	created, err := rt.CreateSession(ctx, agent.CreateSession{Workspace: workspace})
	if err != nil {
		return agent.SessionSnapshot{}, fmt.Errorf("create session: %w", err)
	}
	snapshot := agent.SessionSnapshot{Session: created}
	if err := snapshot.Validate(); err != nil {
		return agent.SessionSnapshot{}, fmt.Errorf("create session: %w", err)
	}
	return snapshot, nil
}

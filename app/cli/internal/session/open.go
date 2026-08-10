// Package session owns the application use cases around durable conversations.
package session

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

type runtime interface {
	CreateSession(context.Context, client.NewSession) (client.Session, error)
	GetSession(context.Context, string) (client.SessionSnapshot, error)
}

// Open restores the selected session or creates a new one in workspace.
func Open(ctx context.Context, rt runtime, id, workspace string) (client.SessionSnapshot, error) {
	if id != "" {
		snapshot, err := rt.GetSession(ctx, id)
		if err != nil {
			return client.SessionSnapshot{}, fmt.Errorf("open session: %w", err)
		}
		if err := snapshot.Validate(); err != nil {
			return client.SessionSnapshot{}, fmt.Errorf("open session: %w", err)
		}
		return snapshot, nil
	}

	created, err := rt.CreateSession(ctx, client.NewSession{Workspace: workspace})
	if err != nil {
		return client.SessionSnapshot{}, fmt.Errorf("create session: %w", err)
	}
	snapshot := client.SessionSnapshot{Session: created}
	if err := snapshot.Validate(); err != nil {
		return client.SessionSnapshot{}, fmt.Errorf("create session: %w", err)
	}
	return snapshot, nil
}

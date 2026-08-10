package server

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
)

func saveTestPlan(ctx context.Context, store *sqlite.PlanStore, sessionID string, steps []plan.Step) error {
	current, err := store.State(ctx, sessionID)
	if err != nil {
		return err
	}
	updatedAt := current.UpdatedAt().Add(time.Nanosecond)
	if current.UpdatedAt().IsZero() {
		updatedAt = time.Unix(1, 0).UTC()
	}
	replacement, err := current.Replace(steps, updatedAt)
	if err != nil {
		return err
	}
	return store.Save(ctx, sessionID, current.Revision(), replacement)
}

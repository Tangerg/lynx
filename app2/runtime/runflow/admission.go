package runflow

import (
	"context"
	"errors"
	"time"

	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
)

// WaitSessionStartable waits on durable admission truth rather than an
// in-process notification. This remains correct when another Runtime process
// owns the Run that currently keeps the session busy.
func (service *Service) WaitSessionStartable(ctx context.Context, sessionID string) error {
	const retry = 125 * time.Millisecond
	for {
		_, err := service.store.GetOpenRootRun(ctx, sessionID)
		if errors.Is(err, rundomain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		timer := time.NewTimer(retry)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-service.lifetime.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return context.Cause(service.lifetime)
		}
	}
}

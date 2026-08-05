package mcp

import (
	"context"
	"testing"
	"time"
)

func shutdownCoordinator(c *Coordinator) error {
	c.BeginShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return c.AwaitShutdown(ctx)
}

func requireCoordinatorShutdown(t testing.TB, c *Coordinator) {
	t.Helper()
	if err := shutdownCoordinator(c); err != nil {
		t.Fatalf("shutdown mcp coordinator: %v", err)
	}
}

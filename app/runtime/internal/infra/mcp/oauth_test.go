package mcp

import (
	"context"
	"errors"
	"testing"
)

func TestNewOAuthFlowHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	flow, err := newOAuthFlow(ctx)
	if flow != nil {
		flow.close(t.Context())
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("newOAuthFlow error = %v, want context.Canceled", err)
	}
}

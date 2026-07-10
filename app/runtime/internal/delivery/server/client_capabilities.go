package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func interruptKindsFromContext(ctx context.Context) []string {
	caps, ok := protocol.ClientCapabilitiesFrom(ctx)
	if !ok || len(caps.InterruptTypes) == 0 {
		return nil
	}
	kinds := make([]string, len(caps.InterruptTypes))
	for i, kind := range caps.InterruptTypes {
		kinds[i] = string(kind)
	}
	return kinds
}

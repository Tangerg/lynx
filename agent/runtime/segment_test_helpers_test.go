package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/runtime"
)

func awaitSegment(t *testing.T, segment *runtime.Segment) runtime.RunCompletion {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	completion, err := segment.Await(ctx)
	if err != nil {
		t.Fatalf("await segment: %v", err)
	}
	return completion
}

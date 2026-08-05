package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/runtime"
)

func awaitRun(t *testing.T, handle *runtime.RunHandle) runtime.RunCompletion {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	completion, err := handle.Await(ctx)
	if err != nil {
		t.Fatalf("await run: %v", err)
	}
	return completion
}

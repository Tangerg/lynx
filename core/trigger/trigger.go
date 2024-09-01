package trigger

import (
	"context"
	"github.com/Tangerg/lynx/core/worker"
)

type Trigger interface {
	AddWorkers(ctx context.Context, workers ...worker.Worker) (int, error)
}

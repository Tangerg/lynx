package job

import (
	"context"
)

type Job interface {
	Start(ctx context.Context) error
	Stop() error
}

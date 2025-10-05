package idgenerators

import (
	"context"
)

type IDGenerator interface {
	Generate(ctx context.Context, objects ...any) (string, error)
}

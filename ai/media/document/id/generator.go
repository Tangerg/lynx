package id

import (
	"context"
)

type Generator interface {
	Generate(ctx context.Context, objects ...any) (string, error)
}

package document

import (
	"context"
)

type Writer interface {
	Write(ctx context.Context, docs []*Document) error
}

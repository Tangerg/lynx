package document

import "context"

type Transformer interface {
	Transform(ctx context.Context, docs []*Document) ([]*Document, error)
}

package document

import "context"

type Reader interface {
	Read(ctx context.Context) ([]*Document, error)
}

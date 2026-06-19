package document

import "context"

var (
	_ Reader      = (*Nop)(nil)
	_ Writer      = (*Nop)(nil)
	_ Transformer = (*Nop)(nil)
	_ Formatter   = (*Nop)(nil)
	_ Batcher     = (*Nop)(nil)
)

// Nop is the do-nothing implementation that satisfies every document
// interface. Useful as a default formatter on freshly built documents,
// as a placeholder in pipelines that want to skip a stage, or as a
// test double.
type Nop struct{}

// nopSingleton is the shared zero-state instance — Nop has no fields,
// so allocating a fresh one per caller would just produce garbage.
var nopSingleton = &Nop{}

func NewNop() *Nop { return nopSingleton }

func (n *Nop) Read(_ context.Context) ([]*Document, error) { return nil, nil }

func (n *Nop) Write(_ context.Context, _ []*Document) error { return nil }

func (n *Nop) Format(doc *Document, _ MetadataMode) string {
	if doc == nil {
		return ""
	}
	return doc.Text
}

func (n *Nop) Transform(_ context.Context, docs []*Document) ([]*Document, error) {
	return docs, nil
}

func (n *Nop) Batch(_ context.Context, docs []*Document) ([][]*Document, error) {
	return [][]*Document{docs}, nil
}

package document

type Transformer interface {
	Transform([]*Document) ([]*Document, error)
}

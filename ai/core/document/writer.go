package document

type Writer interface {
	Write(docs []*Document) error
}

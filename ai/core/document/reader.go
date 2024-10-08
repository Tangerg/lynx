package document

type Reader interface {
	Read() ([]*Document, error)
}

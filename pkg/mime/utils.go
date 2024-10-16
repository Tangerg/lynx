package mime

func New(_type string, subType string) (*Mime, error) {
	return newMime(_type, subType)
}
func newMime(_type string, subType string) (*Mime, error) {
	return NewBuilder().
		WithType(_type).
		WithSubType(subType).
		Build()
}

package media

type Media struct {
	mimeType *MimeType
	data     []byte
}

func (m *Media) MimeType() *MimeType {
	return m.mimeType
}
func (m *Media) Data() []byte {
	return m.data
}

package media

import (
	pkgmime "github.com/Tangerg/lynx/pkg/mime"
)

type Media struct {
	mimeType *pkgmime.Mime
	data     []byte
}

func (m *Media) MimeType() *pkgmime.Mime {
	return m.mimeType
}

func (m *Media) Data() []byte {
	return m.data
}

func New(mimeType *pkgmime.Mime, data []byte) *Media {
	return &Media{
		mimeType: mimeType,
		data:     data,
	}
}

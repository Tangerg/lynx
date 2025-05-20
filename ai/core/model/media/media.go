package media

import (
	pkgmime "github.com/Tangerg/lynx/pkg/mime"
)

type Media struct {
	mimeType *pkgmime.MIME
	data     []byte
}

func (m *Media) MimeType() *pkgmime.MIME {
	return m.mimeType
}

func (m *Media) Data() []byte {
	return m.data
}

func New(mimeType *pkgmime.MIME, data []byte) *Media {
	return &Media{
		mimeType: mimeType,
		data:     data,
	}
}

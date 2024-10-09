package embedding

import (
	"github.com/Tangerg/lynx/ai/core/model"
	"github.com/Tangerg/lynx/pkg/mime"
)

type ModalityType string

const (
	Text  ModalityType = "text"
	Image ModalityType = "image"
	Audio ModalityType = "audio"
	Video ModalityType = "video"
)

var _ model.ResultMetadata = (*ResultMetadata)(nil)

type ResultMetadata struct {
	model.ResultMetadata
	modalityType ModalityType
	documentId   string
	mimeType     *mime.Mime
	documentData []byte
}

func (r *ResultMetadata) ModalityType() ModalityType {
	return r.modalityType
}
func (r *ResultMetadata) DocumentId() string {
	return r.documentId
}
func (r *ResultMetadata) DocumentData() []byte {
	return r.documentData
}
func (r *ResultMetadata) MimeType() *mime.Mime {
	return r.mimeType
}

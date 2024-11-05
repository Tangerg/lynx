package result

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

type EmbeddingResultMetadata struct {
	model.ResultMetadata
	modalityType ModalityType
	documentId   string
	mimeType     *mime.Mime
	documentData []byte
}

func (e *EmbeddingResultMetadata) ModalityType() ModalityType {
	return e.modalityType
}
func (e *EmbeddingResultMetadata) DocumentId() string {
	return e.documentId
}
func (e *EmbeddingResultMetadata) DocumentData() []byte {
	return e.documentData
}
func (e *EmbeddingResultMetadata) MimeType() *mime.Mime {
	return e.mimeType
}

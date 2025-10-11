package embedding

import (
	"errors"

	"github.com/Tangerg/lynx/pkg/mime"
)

type ModalityType string

const (
	Text  ModalityType = "text"
	Image ModalityType = "image"
	Audio ModalityType = "audio"
	Video ModalityType = "video"
)

func (m ModalityType) String() string {
	return string(m)
}

type ResultMetadata struct {
	ModalityType ModalityType
	MimeType     *mime.MIME
	Extra        map[string]any
}

func (r *ResultMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

func (r *ResultMetadata) Get(key string) (any, bool) {
	r.ensureExtra()
	v, ok := r.Extra[key]
	return v, ok
}

func (r *ResultMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.Extra[key] = value
}

type Result struct {
	Embedding []float64
	Metadata  *ResultMetadata
}

func NewResult(embedding []float64, metadata *ResultMetadata) (*Result, error) {
	if len(embedding) == 0 {
		return nil, errors.New("embedding is empty")
	}
	if metadata == nil {
		return nil, errors.New("metadata is required")
	}
	return &Result{
		Embedding: embedding,
		Metadata:  metadata,
	}, nil
}

package embedding

import (
	"errors"

	"github.com/Tangerg/lynx/ai/model"
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

func (m ModalityType) IsText() bool {
	return m == Text
}

func (m ModalityType) IsImage() bool {
	return m == Image
}

func (m ModalityType) IsAudio() bool {
	return m == Audio
}

func (m ModalityType) IsVideo() bool {
	return m == Video
}

type ResultMetadata struct {
	ModalityType ModalityType
	MimeType     *mime.MIME
	DocumentID   string
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

var _ model.Result[[]float64, *ResultMetadata] = (*Result)(nil)

type Result struct {
	index     int64
	embedding []float64
	metadata  *ResultMetadata
}

func NewResult(index int64, embedding []float64, metadata *ResultMetadata) (*Result, error) {
	if len(embedding) == 0 {
		return nil, errors.New("embedding is empty")
	}
	if metadata == nil {
		return nil, errors.New("metadata is required")
	}
	return &Result{
		index:     index,
		embedding: embedding,
		metadata:  metadata,
	}, nil
}

func (r Result) Output() []float64 {
	return r.embedding
}

func (r Result) Metadata() *ResultMetadata {
	return r.metadata
}

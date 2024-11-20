package image

import (
	"github.com/Tangerg/lynx/ai/core/image/request"
	"strconv"
)

var _ request.ImageRequestOptions = (*OpenAIImageRequestOptions)(nil)

type OpenAIImageRequestOptions struct {
	n              int
	model          string
	width          int
	height         int
	responseFormat string
	style          string
	quality        string
	size           string
	user           string
}

func (o *OpenAIImageRequestOptions) N() int {
	return o.n
}

func (o *OpenAIImageRequestOptions) Model() string {
	return o.model
}

func (o *OpenAIImageRequestOptions) Width() int {
	return o.width
}

func (o *OpenAIImageRequestOptions) Height() int {
	return o.height
}

func (o *OpenAIImageRequestOptions) ResponseFormat() string {
	return o.responseFormat
}

func (o *OpenAIImageRequestOptions) Style() string {
	return o.style
}

func (o *OpenAIImageRequestOptions) Quality() string {
	return o.quality
}

func (o *OpenAIImageRequestOptions) Size() string {
	if o.size == "" {
		o.size = strconv.Itoa(o.width) + "x" + strconv.Itoa(o.height)
	}
	return o.size
}

func (o *OpenAIImageRequestOptions) User() string {
	return o.user
}

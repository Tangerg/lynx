package model

import (
	"github.com/Tangerg/lynx/ai/core/audio/transcription/request"
	"github.com/Tangerg/lynx/ai/core/audio/transcription/response"
	"github.com/Tangerg/lynx/ai/core/model"
)

type TranscriptionModel[O request.TranscriptionRequestOptions] interface {
	model.Model[*request.TranscriptionRequest[O], *response.TranscriptionResponse]
}

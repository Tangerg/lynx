package transcription

import (
	"bytes"
	"context"
	"github.com/Tangerg/lynx/ai/core/audio/transcription/model"
	"github.com/Tangerg/lynx/ai/core/audio/transcription/request"
	"github.com/Tangerg/lynx/ai/core/audio/transcription/response"
	"github.com/Tangerg/lynx/ai/core/audio/transcription/result"
	"github.com/Tangerg/lynx/ai/providers/openai/api"
	"github.com/sashabaranov/go-openai"
)

type OpenAITranscriptionRequest = request.TranscriptionRequest[*OpenAITranscodingRequestOptions]

var _ model.TranscriptionModel[*OpenAITranscodingRequestOptions] = (*OpenAITranscriptionModel)(nil)

type OpenAITranscriptionModel struct {
	api api.OpenAIApi
}

func (o *OpenAITranscriptionModel) createApiRequest(req *OpenAITranscriptionRequest) *openai.AudioRequest {
	reader := bytes.NewReader(req.Instructions().Data())
	tgs := make([]openai.TranscriptionTimestampGranularity, 0, len(req.Options().TimestampGranularities()))
	for _, tg := range req.Options().TimestampGranularities() {
		tgs = append(tgs, openai.TranscriptionTimestampGranularity(tg))
	}
	return &openai.AudioRequest{
		Model:                  req.Options().Model(),
		Reader:                 reader,
		Prompt:                 req.Options().Prompt(),
		Language:               req.Options().Language(),
		Temperature:            float32(req.Options().Temperature()),
		Format:                 openai.AudioResponseFormat(req.Options().ResponseFormat()),
		TimestampGranularities: tgs,
	}
}
func (o *OpenAITranscriptionModel) createResponse(cresp *openai.AudioResponse) *response.TranscriptionResponse {
	return response.NewTranscriptionResponse(
		result.NewTranscriptionResult(cresp.Text, nil),
		nil,
	)
}

func (o *OpenAITranscriptionModel) Call(ctx context.Context, req *OpenAITranscriptionRequest) (*response.TranscriptionResponse, error) {
	creq := o.createApiRequest(req)
	cres, err := o.api.CreateTranscription(ctx, creq)
	if err != nil {
		return nil, err
	}
	return o.createResponse(cres), nil
}

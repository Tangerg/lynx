package image

import (
	"context"
	"github.com/Tangerg/lynx/ai/core/image/image"
	"github.com/Tangerg/lynx/ai/core/image/model"
	"github.com/Tangerg/lynx/ai/core/image/request"
	"github.com/Tangerg/lynx/ai/core/image/response"
	"github.com/Tangerg/lynx/ai/core/image/result"
	"github.com/Tangerg/lynx/ai/providers/openai/api"
	"github.com/sashabaranov/go-openai"
)

type OpenAIImageRequest = request.ImageRequest[*OpenAIImageRequestOptions]
type OpenAIImageResponse = response.ImageResponse[*OpenAIImageResultMetadata]

var _ model.ImageModel[*OpenAIImageRequestOptions, *OpenAIImageResultMetadata] = (*OpenAIImageModel)(nil)

type OpenAIImageModel struct {
	openAIApi      *api.OpenAIApi
	defaultOptions *OpenAIImageRequestOptions
}

func (o *OpenAIImageModel) createApiRequest(req *OpenAIImageRequest) *openai.ImageRequest {
	return &openai.ImageRequest{
		Prompt:         req.Instructions()[0].Text(),
		Model:          req.Options().Model(),
		N:              req.Options().N(),
		Quality:        req.Options().Quality(),
		Size:           req.Options().Size(),
		Style:          req.Options().Style(),
		ResponseFormat: req.Options().ResponseFormat(),
		User:           req.Options().User(),
	}
}

func (o *OpenAIImageModel) convertResponse(cresp *openai.ImageResponse) *OpenAIImageResponse {
	results := make([]*result.ImageResult[*OpenAIImageResultMetadata], 0, len(cresp.Data))
	for _, data := range cresp.Data {
		results = append(results,
			result.NewImageResult[*OpenAIImageResultMetadata](
				image.NewImage(data.URL, data.B64JSON),
				nil,
			),
		)
	}
	return response.NewImageResponse[*OpenAIImageResultMetadata](results, nil)
}

func (o *OpenAIImageModel) Call(ctx context.Context, req *OpenAIImageRequest) (*OpenAIImageResponse, error) {
	creq := o.createApiRequest(req)
	cres, err := o.openAIApi.CreateImage(ctx, creq)
	if err != nil {
		return nil, err
	}
	return o.convertResponse(cres), nil
}

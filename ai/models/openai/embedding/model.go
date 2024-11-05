package embedding

import (
	"context"
	"fmt"
	"github.com/Tangerg/lynx/ai/core/embedding/model"
	"github.com/Tangerg/lynx/ai/core/embedding/request"
	"github.com/Tangerg/lynx/ai/core/embedding/response"
	"github.com/Tangerg/lynx/ai/core/embedding/result"

	"github.com/samber/lo"
	"github.com/sashabaranov/go-openai"

	"github.com/Tangerg/lynx/ai/models/openai/api"
)

type OpenAIEmbeddingRequest = request.EmbeddingRequest[*OpenAIEmbeddingOptions]

var _ model.EmbeddingModel[*OpenAIEmbeddingOptions] = (*OpenAIEmbeddingModel)(nil)

type OpenAIEmbeddingModel struct {
	openAIApi *api.OpenAIApi
}

func NewOpenAIEmbeddingModel(openAIApi *api.OpenAIApi) *OpenAIEmbeddingModel {
	return &OpenAIEmbeddingModel{openAIApi: openAIApi}
}

func (o *OpenAIEmbeddingModel) createApiRequest(req *OpenAIEmbeddingRequest) *openai.EmbeddingRequestStrings {
	return &openai.EmbeddingRequestStrings{
		Input:          req.Instructions(),
		Model:          openai.SmallEmbedding3,
		User:           req.Options().User(),
		EncodingFormat: "float",
		Dimensions:     req.Options().Dimensions(),
	}
}

func (o *OpenAIEmbeddingModel) createResponse(resp *openai.EmbeddingResponse) *response.EmbeddingResponse {
	ebds := make([]*result.EmbeddingResult, 0, len(resp.Data))
	for _, data := range resp.Data {
		ebds = append(ebds,
			result.NewEmbedding(
				lo.Map(data.Embedding, func(item float32, index int) float64 {
					return float64(item)
				}),
				data.Index,
				nil),
		)
	}
	rv := response.NewEmbeddingResponse(ebds, nil)
	return rv
}

func (o *OpenAIEmbeddingModel) Call(ctx context.Context, req *OpenAIEmbeddingRequest) (*response.EmbeddingResponse, error) {
	ereq := o.createApiRequest(req)
	embeddings, err := o.openAIApi.CreateEmbeddings(ctx, ereq)
	if err != nil {
		return nil, err
	}
	fmt.Println(embeddings)
	return o.createResponse(&embeddings), nil
}

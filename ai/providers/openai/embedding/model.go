package embedding

import (
	"context"
	"github.com/Tangerg/lynx/ai/core/embedding/model"
	"github.com/Tangerg/lynx/ai/core/embedding/request"
	"github.com/Tangerg/lynx/ai/core/embedding/response"
	"github.com/Tangerg/lynx/ai/core/embedding/result"
	"github.com/Tangerg/lynx/ai/providers/openai/api"
	"github.com/samber/lo"
	"github.com/sashabaranov/go-openai"
)

type OpenAIEmbeddingRequest = request.EmbeddingRequest[*OpenAIEmbeddingRequestOptions]

var _ model.EmbeddingModel[*OpenAIEmbeddingRequestOptions] = (*OpenAIEmbeddingModel)(nil)

type OpenAIEmbeddingModel struct {
	openAIApi *api.OpenAIApi
}

func (o *OpenAIEmbeddingModel) getEncodingFormat(format string) openai.EmbeddingEncodingFormat {
	switch format {
	case "float", "base64":
		return openai.EmbeddingEncodingFormat(format)
	default:
		return openai.EmbeddingEncodingFormatFloat
	}
}
func (o *OpenAIEmbeddingModel) getModel(model string) openai.EmbeddingModel {
	switch model {
	case "text-similarity-ada-001", "text-similarity-babbage-001", "text-similarity-curie-001",
		"text-similarity-davinci-001", "text-search-ada-doc-001", "text-search-ada-query-001",
		"text-search-babbage-doc-001", "text-search-babbage-query-001", "text-search-curie-doc-001",
		"text-search-curie-query-001", "text-search-davinci-doc-001", "text-search-davinci-query-001",
		"code-search-ada-code-001", "code-search-ada-text-001", "code-search-babbage-code-001",
		"code-search-babbage-text-001", "text-embedding-ada-002", "text-embedding-3-small",
		"text-embedding-3-large":
		return openai.EmbeddingModel(model)
	default:
		return openai.SmallEmbedding3
	}
}
func (o *OpenAIEmbeddingModel) createApiRequest(req *OpenAIEmbeddingRequest) *openai.EmbeddingRequestStrings {
	return &openai.EmbeddingRequestStrings{
		Input:          req.Instructions(),
		Model:          o.getModel(req.Options().Model()),
		User:           req.Options().User(),
		EncodingFormat: o.getEncodingFormat(req.Options().EncodingFormat()),
		Dimensions:     req.Options().Dimensions(),
	}
}
func (o *OpenAIEmbeddingModel) createResponse(cresp *openai.EmbeddingResponse) *response.EmbeddingResponse {
	ebds := make([]*result.EmbeddingResult, 0, len(cresp.Data))
	for _, data := range cresp.Data {
		ebds = append(ebds,
			result.NewEmbedding(
				lo.Map(data.Embedding, func(item float32, _ int) float64 {
					return float64(item)
				}),
				data.Index,
				nil),
		)
	}
	return response.NewEmbeddingResponse(ebds, nil)
}

func (o *OpenAIEmbeddingModel) Call(ctx context.Context, req *OpenAIEmbeddingRequest) (*response.EmbeddingResponse, error) {
	creq := o.createApiRequest(req)
	cres, err := o.openAIApi.CreateEmbeddings(ctx, creq)
	if err != nil {
		return nil, err
	}
	return o.createResponse(cres), nil
}

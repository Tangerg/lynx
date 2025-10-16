package openai

import (
	"context"
	"testing"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/model/embedding"
	"github.com/Tangerg/lynx/pkg/assert"
)

func newEmbeddingModel() *EmbeddingModel {
	defaultOptions := assert.Must(embedding.NewOptions(baseModel))

	return assert.Must(NewEmbeddingModel(
		newAPIKey(),
		defaultOptions,
		option.WithBaseURL(baseURL),
	))
}

func TestEmbeddingModel_Call(t *testing.T) {
	model := newEmbeddingModel()
	response, err := model.Call(context.Background(), assert.Must(embedding.NewRequest([]string{"test string"})))
	if err != nil {
		t.Fatal(err)
	}
	t.Log(response.Result().Embedding)
}

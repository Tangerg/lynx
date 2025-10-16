package embedding_test

import (
	"context"
	"os"
	"testing"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/embedding"
	"github.com/Tangerg/lynx/ai/providers/models/openai"
	"github.com/Tangerg/lynx/pkg/assert"
)

var (
	baseURL   = "https://api.siliconflow.cn/v1"
	baseModel = "BAAI/bge-m3"
)

func newAPIKey() model.ApiKey {
	apiKey := os.Getenv("apiKey")

	return model.NewApiKey(apiKey)
}

func newEmbeddingModel() *openai.EmbeddingModel {
	defaultOptions := assert.Must(embedding.NewOptions(baseModel))

	return assert.Must(openai.NewEmbeddingModel(
		newAPIKey(),
		defaultOptions,
		option.WithBaseURL(baseURL),
	))
}

func newEmbeddingClient() *embedding.Client {
	req := assert.Must(
		embedding.NewClientRequest(newEmbeddingModel()),
	)

	return assert.Must(
		embedding.NewClient(req),
	)
}

func TestClient_EmbedText(t *testing.T) {
	client := newEmbeddingClient()
	embeddings, _, err := client.
		EmbedText("test text").
		Call().
		Embedding(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Log(embeddings)
}

func TestClient_EmbedTexts(t *testing.T) {
	client := newEmbeddingClient()
	embeddings, _, err := client.
		EmbedTexts([]string{"test text1", "test text2"}).
		Call().
		Embeddings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range embeddings {
		t.Log(item)
	}
}

package embedding

import (
	"context"
	"os"
	"testing"

	"github.com/Tangerg/lynx/ai/core/embedding"
	"github.com/Tangerg/lynx/ai/models/openai/api"
)

func client() *OpenAIEmbeddingModel {
	token := os.Getenv("OPENAI_TOKEN")
	openAIApi := api.NewOpenAIApi(token)
	return NewOpenAIEmbeddingModel(openAIApi)
}
func TestNewOpenAIEmbeddingModel(t *testing.T) {
	cli := client()
	req := embedding.NewRequest[*OpenAIEmbeddingOptions]([]string{"hello world!"}, &OpenAIEmbeddingOptions{})
	resp, err := cli.Call(context.Background(), req)
	t.Log(err)
	t.Log(len(resp.Result().Output()))
	for _, f := range resp.Result().Output() {
		t.Log(f)
	}
}

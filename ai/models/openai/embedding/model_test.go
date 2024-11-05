package embedding

import (
	"context"
	"github.com/Tangerg/lynx/ai/core/embedding/request"
	"os"
	"testing"

	"github.com/Tangerg/lynx/ai/models/openai/api"
)

func client() *OpenAIEmbeddingModel {
	token := os.Getenv("OPENAI_TOKEN")
	openAIApi := api.NewOpenAIApi(token)
	return NewOpenAIEmbeddingModel(openAIApi)
}
func TestNewOpenAIEmbeddingModel(t *testing.T) {
	cli := client()
	req := request.NewEmbeddingRequest[*OpenAIEmbeddingOptions]([]string{"hello world!"}, &OpenAIEmbeddingOptions{})
	resp, err := cli.Call(context.Background(), req)
	t.Log(err)
	t.Log(len(resp.Result().Output()))
	for _, f := range resp.Result().Output() {
		t.Log(f)
	}
}

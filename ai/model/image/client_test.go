package image_test

import (
	"context"
	"os"
	"testing"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/extensions/models/openai"
	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/image"
	"github.com/Tangerg/lynx/pkg/assert"
)

var (
	baseURL   = "https://api.siliconflow.cn/v1"
	baseModel = "Qwen/Qwen-Image"
)

func newAPIKey() model.ApiKey {
	apiKey := os.Getenv("apiKey")

	return model.NewApiKey(apiKey)
}

func newImageModel() *openai.ImageModel {
	defaultOptions := assert.Must(image.NewOptions(baseModel))

	return assert.Must(openai.NewImageModel(
		newAPIKey(),
		defaultOptions,
		option.WithBaseURL(baseURL),
	))
}

func newImageClient() *image.Client {
	return assert.Must(
		image.NewClientWithModel(newImageModel()),
	)
}

func TestClient_Generate(t *testing.T) {
	client := newImageClient()

	img, _, err := client.
		Generate().
		WithPrompt("an island").
		Call().
		Image(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Log(img.URL, img.B64JSON)
}

func TestClient_GeneratePrompt(t *testing.T) {
	client := newImageClient()

	img, _, err := client.
		GeneratePrompt("an island").
		Call().
		Image(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Log(img.URL, img.B64JSON)
}

func TestClient_GenerateRequest(t *testing.T) {
	client := newImageClient()

	img, _, err := client.
		GenerateRequest(assert.Must(image.NewRequest("an island"))).
		Call().
		Image(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Log(img.URL, img.B64JSON)
}

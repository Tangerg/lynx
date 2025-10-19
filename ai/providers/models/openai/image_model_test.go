package openai

import (
	"context"
	"testing"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/model/image"
	"github.com/Tangerg/lynx/pkg/assert"
)

func newImageModel() *ImageModel {
	defaultOptions := assert.Must(image.NewOptions(baseModel))

	return assert.Must(NewImageModel(
		newAPIKey(),
		defaultOptions,
		option.WithBaseURL(baseURL),
	))
}

func TestImageModel_Call(t *testing.T) {
	model := newImageModel()
	response, err := model.Call(context.Background(), assert.Must(image.NewRequest("an island near sea, with seagulls, moon shining over the sea, light house, boats int he background, fish flying over the sea")))
	if err != nil {
		t.Fatal(err)
	}
	t.Log(response.Result().Image.URL)
	t.Log(response.Result().Image.B64JSON)
}

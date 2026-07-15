package embeddingclient_test

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/embeddingclient"
)

func Example() {
	model := embedding.ModelFunc(func(context.Context, *embedding.Request) (*embedding.Response, error) {
		result, _ := embedding.NewResult([]float64{0.1, 0.2}, &embedding.ResultMetadata{})
		return embedding.NewResponse([]*embedding.Result{result}, &embedding.ResponseMetadata{})
	})
	client, err := embeddingclient.New(model)
	if err != nil {
		panic(err)
	}
	vector, err := client.EmbedText(context.Background(), "lynx")
	if err != nil {
		panic(err)
	}
	fmt.Println(len(vector))
	// Output: 2
}

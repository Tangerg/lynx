package embeddingclient_test

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/embeddingclient"
)

func Example() {
	model := embedding.ModelFunc(func(context.Context, *embedding.Request) (*embedding.Response, error) {
		output, _ := embedding.NewOutput([]float64{0.1, 0.2}, &embedding.OutputMetadata{})
		return embedding.NewResponse([]*embedding.Output{output}, &embedding.ResponseMetadata{})
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

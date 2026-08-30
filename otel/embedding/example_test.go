package embedding_test

import (
	"context"
	"fmt"

	coreembedding "github.com/Tangerg/scope/core/embedding"
	otelembedding "github.com/Tangerg/scope/otel/embedding"
)

func ExampleMiddleware_Wrap() {
	middleware, err := otelembedding.NewMiddleware(otelembedding.MiddlewareConfig{Provider: "example"})
	if err != nil {
		panic(err)
	}
	model, err := middleware.Wrap(coreembedding.ModelFunc(func(context.Context, *coreembedding.Request) (*coreembedding.Response, error) {
		return &coreembedding.Response{Metadata: &coreembedding.ResponseMetadata{Model: "served-model"}}, nil
	}))
	if err != nil {
		panic(err)
	}
	request, err := coreembedding.NewRequest([]string{"text"})
	if err != nil {
		panic(err)
	}
	response, err := model.Call(context.Background(), request)
	if err != nil {
		panic(err)
	}

	fmt.Println(response.Metadata.Model)
	// Output:
	// served-model
}

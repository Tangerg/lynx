package inmemory_test

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/vectorstore/inmemory"
)

func Example() {
	model := embedding.ModelFunc(func(context.Context, *embedding.Request) (*embedding.Response, error) {
		return nil, nil
	})
	store, err := inmemory.NewStore(inmemory.StoreConfig{EmbeddingModel: model})
	if err != nil {
		panic(err)
	}
	fmt.Println(store.Len())
	// Output: 0
}

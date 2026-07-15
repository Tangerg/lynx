package embedding_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/embedding"
)

func Example() {
	request, err := embedding.NewRequest([]string{"lynx", "wild cat"})
	if err != nil {
		panic(err)
	}
	options, err := embedding.NewOptions("text-embedding-model")
	if err != nil {
		panic(err)
	}
	dimensions := int64(3)
	options.Dimensions = &dimensions
	request.Options = options

	fmt.Println(len(request.Texts), request.Options.Model, *request.Options.Dimensions)
	// Output:
	// 2 text-embedding-model 3
}

package moderation_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/moderation"
)

func Example() {
	request, err := moderation.NewRequest([]string{"content to classify"})
	if err != nil {
		panic(err)
	}
	options, err := moderation.NewOptions("moderation-model")
	if err != nil {
		panic(err)
	}
	request.Options = options

	categories := moderation.Categories{
		"violence": {Flagged: true, Score: 0.91},
	}
	fmt.Println(request.Options.Model, categories.Flagged())
	// Output:
	// moderation-model true
}

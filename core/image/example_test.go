package image_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/image"
)

func Example() {
	request, err := image.NewRequest("A lynx walking through snow")
	if err != nil {
		panic(err)
	}
	options, err := image.NewOptions("image-model")
	if err != nil {
		panic(err)
	}
	options.OutputFormat = "image/png"
	request.Options = options

	fmt.Println(request.Options.Model, request.Options.OutputFormat)
	// Output:
	// image-model image/png
}

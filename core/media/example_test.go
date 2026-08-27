package media_test

import (
	"fmt"

	"github.com/Tangerg/scope/core/media"
)

func Example() {
	attachment, err := media.NewBytes("image/png", []byte("scope"))
	if err != nil {
		panic(err)
	}
	attachment.ID = "image-1"
	attachment.Name = "scope.png"

	fmt.Println(attachment.Source.Kind, len(attachment.Source.Bytes), attachment.Name)
	// Output:
	// bytes 5 scope.png
}

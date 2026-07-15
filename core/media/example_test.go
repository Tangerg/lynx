package media_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/media"
)

func Example() {
	attachment, err := media.NewBytes("image/png", []byte("lynx"))
	if err != nil {
		panic(err)
	}
	attachment.ID = "image-1"
	attachment.Name = "lynx.png"

	fmt.Println(attachment.Source.Kind, len(attachment.Source.Bytes), attachment.Name)
	// Output:
	// bytes 4 lynx.png
}

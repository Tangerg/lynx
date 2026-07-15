package transcription_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/transcription"
)

func Example() {
	audio, err := media.NewBytes("audio/wav", []byte("audio"))
	if err != nil {
		panic(err)
	}
	request, err := transcription.NewRequest(audio)
	if err != nil {
		panic(err)
	}
	options, err := transcription.NewOptions("transcription-model")
	if err != nil {
		panic(err)
	}
	options.Language = "en"
	options.ResponseFormat = "verbose_json"
	request.Options = options

	fmt.Println(request.Audio.MIME, request.Options.Language)
	// Output:
	// audio/wav en
}

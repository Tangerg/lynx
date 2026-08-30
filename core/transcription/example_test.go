package transcription_test

import (
	"fmt"

	"github.com/Tangerg/scope/core/media"
	"github.com/Tangerg/scope/core/transcription"
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
	options := transcription.Options{Model: "transcription-model"}
	err = options.Validate()
	if err != nil {
		panic(err)
	}
	options.Language = "en"
	request.Options = options

	fmt.Println(request.Audio.MIME, request.Options.Language)
	// Output:
	// audio/wav en
}

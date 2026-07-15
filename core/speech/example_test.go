package speech_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/speech"
)

func Example() {
	request, err := speech.NewRequest("Hello from Lynx.")
	if err != nil {
		panic(err)
	}
	options, err := speech.NewOptions("speech-model")
	if err != nil {
		panic(err)
	}
	options.Voice = "alloy"
	options.ResponseFormat = "mp3"
	options.Speed = 1
	request.Options = options

	fmt.Println(request.Options.Model, request.Options.Voice, request.Options.Speed)
	// Output:
	// speech-model alloy 1
}

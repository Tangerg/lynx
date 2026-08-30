package speech_test

import (
	"fmt"

	"github.com/Tangerg/scope/core/speech"
)

func Example() {
	request, err := speech.NewRequest("Hello from Scope.")
	if err != nil {
		panic(err)
	}
	options := speech.Options{Model: "speech-model"}
	err = options.Validate()
	if err != nil {
		panic(err)
	}
	options.Voice = "alloy"
	options.OutputFormat = "mp3"
	options.Speed = 1
	request.Options = options

	fmt.Println(request.Options.Model, request.Options.Voice, request.Options.Speed)
	// Output:
	// speech-model alloy 1
}

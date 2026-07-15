package chat_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

func Example() {
	request, err := chat.NewRequest(
		chat.NewSystemMessage("Answer concisely."),
		chat.NewUserMessage(chat.NewTextPart("What is a lynx?")),
	)
	if err != nil {
		panic(err)
	}
	request.Options = chat.Options{Model: "provider-model"}

	fmt.Println(request.Messages[1].Text())
	fmt.Println(request.Options.Model)
	// Output:
	// What is a lynx?
	// provider-model
}

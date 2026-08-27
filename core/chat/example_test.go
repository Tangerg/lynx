package chat_test

import (
	"fmt"

	"github.com/Tangerg/scope/core/chat"
)

func Example() {
	request, err := chat.NewRequest(
		chat.NewSystemMessage("Answer concisely."),
		chat.NewUserMessage(chat.NewTextPart("What is a scope?")),
	)
	if err != nil {
		panic(err)
	}
	request.Options = chat.Options{Model: "provider-model"}

	fmt.Println(request.Messages[1].Text())
	fmt.Println(request.Options.Model)
	// Output:
	// What is a scope?
	// provider-model
}

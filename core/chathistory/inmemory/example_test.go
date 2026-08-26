package inmemory_test

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/chathistory"
	"github.com/Tangerg/lynx/core/chathistory/inmemory"
)

func Example() {
	store := inmemory.New()
	message := chat.NewUserMessage(chat.NewTextPart("hello"))
	if err := store.Write(context.Background(), chathistory.ConversationID("demo"), message); err != nil {
		panic(err)
	}
	messages, err := store.Read(context.Background(), chathistory.ConversationID("demo"))
	if err != nil {
		panic(err)
	}
	fmt.Println(len(messages), messages[0].Text())
	// Output: 1 hello
}

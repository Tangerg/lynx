package chathistory_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/chathistory"
)

func Example() {
	conversationID, err := chathistory.NewConversationID("customer-42")
	if err != nil {
		panic(err)
	}
	fmt.Println(conversationID)
	// Output: customer-42
}

package history_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/history"
)

func Example() {
	conversationID, err := history.NewConversationID("customer-42")
	if err != nil {
		panic(err)
	}
	fmt.Println(conversationID)
	// Output: customer-42
}

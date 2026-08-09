package bootstrap

import (
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/application/conversations"
)

type messageEnvironment struct {
	store        conversations.Store
	conversation *conversations.Messages
}

func buildMessageEnvironment(store conversations.Store) (messageEnvironment, error) {
	if store == nil {
		return messageEnvironment{}, errors.New("runtime: ConversationStore is required")
	}
	return messageEnvironment{
		store:        store,
		conversation: conversations.NewMessages(store),
	}, nil
}

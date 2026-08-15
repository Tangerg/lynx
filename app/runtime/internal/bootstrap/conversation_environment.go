package bootstrap

import (
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/application/conversations"
)

type conversationEnvironment struct {
	store    conversations.Store
	messages *conversations.Messages
}

func buildConversationEnvironment(store conversations.Store, compactions conversations.CompactionStore) (conversationEnvironment, error) {
	if store == nil {
		return conversationEnvironment{}, errors.New("runtime: ConversationStore is required")
	}
	if compactions == nil {
		return conversationEnvironment{}, errors.New("runtime: ConversationCompactions is required")
	}
	return conversationEnvironment{
		store:    store,
		messages: conversations.NewMessages(store, compactions),
	}, nil
}

package arch_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/models/bedrock"
)

func TestChatConstructorCompiles(t *testing.T) {
	t.Parallel()
	var _ func(context.Context, bedrock.ChatConfig) (*bedrock.Chat, error) = bedrock.NewChat
}

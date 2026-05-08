package mcp

import (
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
)

// PromptMessagesToChat converts the messages of an MCP GetPromptResult
// into the []chat.Message form chat.ClientRequest.WithMessages expects.
//
// Roles outside "user" / "assistant" fall through to user. Messages
// whose body has no text payload (image, audio, embedded resource) are
// dropped — the chat schema is text-first; richer content support
// awaits a chat schema bump.
//
//	res, _ := session.GetPrompt(ctx, &mcp.GetPromptParams{Name: "..."})
//	chatReq := client.Chat().WithMessages(mcp.PromptMessagesToChat(res.Messages)...)
func PromptMessagesToChat(messages []*sdkmcp.PromptMessage) []chat.Message {
	out := make([]chat.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		text := textOfContent(msg.Content)
		if text == "" {
			continue
		}
		if string(msg.Role) == "assistant" {
			out = append(out, chat.NewAssistantMessage(text))
		} else {
			out = append(out, chat.NewUserMessage(text))
		}
	}
	return out
}

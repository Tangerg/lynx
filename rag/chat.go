package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

func resolvePromptTemplate(current *chatclient.Template, fallback string, required ...string) (*chatclient.Template, error) {
	if current == nil {
		var err error
		current, err = chatclient.ParseTemplate(fallback)
		if err != nil {
			return nil, err
		}
	}
	if err := current.Require(required...); err != nil {
		return nil, err
	}
	return current, nil
}

func callPrompt(ctx context.Context, client *chatclient.Client, prompt *chatclient.Template, data any) (string, error) {
	message, err := prompt.UserMessage(data)
	if err != nil {
		return "", err
	}
	response, err := client.Call(ctx, &chat.Request{Messages: []chat.Message{message}})
	if err != nil {
		return "", err
	}
	return response.Text(), nil
}

func formatChatHistory(messages []chat.Message) string {
	var output strings.Builder
	for messageIndex := range messages {
		if output.Len() != 0 {
			output.WriteString("\n\n")
		}
		fmt.Fprintf(&output, "%s: ", messages[messageIndex].Role)
		for partIndex := range messages[messageIndex].Parts {
			if partIndex != 0 {
				output.WriteString(" ")
			}
			part := messages[messageIndex].Parts[partIndex]
			switch part.Kind {
			case chat.PartText:
				output.WriteString(part.Text)
			case chat.PartMedia:
				fmt.Fprintf(&output, "[media %s]", part.Media.MIME)
			case chat.PartReasoning:
				output.WriteString("[reasoning omitted]")
			case chat.PartToolCall:
				fmt.Fprintf(&output, "[tool call %s %s]", part.ToolCall.Name, part.ToolCall.Arguments)
			case chat.PartToolResult:
				fmt.Fprintf(&output, "[tool result %s %s]", part.ToolResult.Name, part.ToolResult.Result)
			}
		}
	}
	return output.String()
}

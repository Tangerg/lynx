package client

import "github.com/Tangerg/lynx/ai/model/chat/request"

type ChatClient interface {
	Chat() ChatClientRequest
	ChatText(text string) ChatClientRequest
	ChatRequest(request *request.ChatRequest[request.ChatOptions]) ChatClientRequest
}

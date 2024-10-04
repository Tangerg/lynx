package api

import (
	"strings"

	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type AdvisedRequest struct {
	chatModel     model.ChatModel[prompt.ChatOptions, metadata.ChatGenerationMetadata]
	userText      string
	systemText    string
	chatOptions   prompt.ChatOptions
	messages      []message.ChatMessage
	userParams    map[string]any
	systemParams  map[string]any
	advisors      []Advisor
	advisorParams map[string]any
}

func (a *AdvisedRequest) ChatModel() model.ChatModel[prompt.ChatOptions, metadata.ChatGenerationMetadata] {
	return a.chatModel
}
func (a *AdvisedRequest) UserText() string {
	return strings.TrimSpace(a.userText)
}
func (a *AdvisedRequest) SystemText() string {
	return strings.TrimSpace(a.systemText)
}
func (a *AdvisedRequest) ChatOptions() prompt.ChatOptions {
	return a.chatOptions
}
func (a *AdvisedRequest) Messages() []message.ChatMessage {
	return a.messages
}
func (a *AdvisedRequest) UserParams() map[string]any {
	return a.userParams
}
func (a *AdvisedRequest) SystemParams() map[string]any {
	return a.systemParams
}
func (a *AdvisedRequest) Advisors() []Advisor {
	return a.advisors
}
func (a *AdvisedRequest) AdvisorParams() map[string]any {
	return a.advisorParams
}

func NewAdvisedRequestBuilder() *AdvisedRequestBuilder {
	return &AdvisedRequestBuilder{
		request: &AdvisedRequest{},
	}
}

type AdvisedRequestBuilder struct {
	request *AdvisedRequest
}

func (b *AdvisedRequestBuilder) FromAdvisedRequest(a *AdvisedRequest) *AdvisedRequestBuilder {
	b.request = a
	return b
}

func (b *AdvisedRequestBuilder) WithChatModel(chatModel model.ChatModel[prompt.ChatOptions, metadata.ChatGenerationMetadata]) *AdvisedRequestBuilder {
	b.request.chatModel = chatModel
	return b
}

func (b *AdvisedRequestBuilder) WithUserText(userText string) *AdvisedRequestBuilder {
	b.request.userText = userText
	return b
}

func (b *AdvisedRequestBuilder) WithSystemText(systemText string) *AdvisedRequestBuilder {
	b.request.systemText = systemText
	return b
}

func (b *AdvisedRequestBuilder) WithChatOptions(chatOptions prompt.ChatOptions) *AdvisedRequestBuilder {
	b.request.chatOptions = chatOptions
	return b
}

func (b *AdvisedRequestBuilder) WithMessages(messages ...message.ChatMessage) *AdvisedRequestBuilder {
	b.request.messages = messages
	return b
}

func (b *AdvisedRequestBuilder) WithUserParam(userParams map[string]any) *AdvisedRequestBuilder {
	b.request.userParams = userParams
	return b
}

func (b *AdvisedRequestBuilder) WithSystemParam(systemParams map[string]any) *AdvisedRequestBuilder {
	b.request.systemParams = systemParams
	return b
}

func (b *AdvisedRequestBuilder) WithAdvisors(advisors ...Advisor) *AdvisedRequestBuilder {
	b.request.advisors = advisors
	return b
}

func (b *AdvisedRequestBuilder) WithAdvisorParam(advisorParams map[string]any) *AdvisedRequestBuilder {
	b.request.advisorParams = advisorParams
	return b
}

func (b *AdvisedRequestBuilder) Build() *AdvisedRequest {
	return b.request
}

package api

import (
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type AdvisedRequest[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	chatModel     model.ChatModel[O, M]
	chatOptions   O
	userText      string
	userParams    map[string]any
	systemText    string
	systemParams  map[string]any
	messages      []message.ChatMessage
	advisors      []Advisor
	advisorParams map[string]any
}

func (a *AdvisedRequest[O, M]) ChatModel() model.ChatModel[O, M] {
	return a.chatModel
}
func (a *AdvisedRequest[O, M]) ChatOptions() O {
	return a.chatOptions
}
func (a *AdvisedRequest[O, M]) UserText() string {
	return a.userText
}
func (a *AdvisedRequest[O, M]) UserParams() map[string]any {
	return a.userParams
}
func (a *AdvisedRequest[O, M]) SystemText() string {
	return a.systemText
}
func (a *AdvisedRequest[O, M]) SystemParams() map[string]any {
	return a.systemParams
}
func (a *AdvisedRequest[O, M]) Messages() []message.ChatMessage {
	return a.messages
}
func (a *AdvisedRequest[O, M]) Advisors() []Advisor {
	return a.advisors
}
func (a *AdvisedRequest[O, M]) AdvisorParams() map[string]any {
	return a.advisorParams
}

func NewAdvisedRequestBuilder[O prompt.ChatOptions, M metadata.ChatGenerationMetadata]() *AdvisedRequestBuilder[O, M] {
	return &AdvisedRequestBuilder[O, M]{
		request: &AdvisedRequest[O, M]{},
	}
}

type AdvisedRequestBuilder[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	request *AdvisedRequest[O, M]
}

func (b *AdvisedRequestBuilder[O, M]) FromAdvisedRequest(req *AdvisedRequest[O, M]) *AdvisedRequestBuilder[O, M] {
	b.request = req
	return b
}

func (b *AdvisedRequestBuilder[O, M]) WithChatModel(chatModel model.ChatModel[O, M]) *AdvisedRequestBuilder[O, M] {
	b.request.chatModel = chatModel
	return b
}

func (b *AdvisedRequestBuilder[O, M]) WithChatOptions(chatOptions O) *AdvisedRequestBuilder[O, M] {
	b.request.chatOptions = chatOptions
	return b
}

func (b *AdvisedRequestBuilder[O, M]) WithUserText(userText string) *AdvisedRequestBuilder[O, M] {
	b.request.userText = userText
	return b
}

func (b *AdvisedRequestBuilder[O, M]) WithUserParam(userParams map[string]any) *AdvisedRequestBuilder[O, M] {
	b.request.userParams = userParams
	return b
}

func (b *AdvisedRequestBuilder[O, M]) WithSystemText(systemText string) *AdvisedRequestBuilder[O, M] {
	b.request.systemText = systemText
	return b
}

func (b *AdvisedRequestBuilder[O, M]) WithSystemParam(systemParams map[string]any) *AdvisedRequestBuilder[O, M] {
	b.request.systemParams = systemParams
	return b
}

func (b *AdvisedRequestBuilder[O, M]) WithMessages(messages ...message.ChatMessage) *AdvisedRequestBuilder[O, M] {
	b.request.messages = messages
	return b
}

func (b *AdvisedRequestBuilder[O, M]) WithAdvisors(advisors ...Advisor) *AdvisedRequestBuilder[O, M] {
	b.request.advisors = advisors
	return b
}

func (b *AdvisedRequestBuilder[O, M]) WithAdvisorParam(advisorParams map[string]any) *AdvisedRequestBuilder[O, M] {
	b.request.advisorParams = advisorParams
	return b
}

func (b *AdvisedRequestBuilder[O, M]) Build() *AdvisedRequest[O, M] {
	return b.request
}

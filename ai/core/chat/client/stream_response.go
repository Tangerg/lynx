package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
)

type StreamResponseSpec interface {
	Content() string
	ChatResponse() *completion.ChatCompletion[metadata.ChatGenerationMetadata]
}

var _ StreamResponseSpec = (*DefaultStreamResponseSpec)(nil)

type DefaultStreamResponseSpec struct {
}

func NewDefaultStreamResponseSpec(req *DefaultChatClientRequest) *DefaultStreamResponseSpec {
	return &DefaultStreamResponseSpec{}
}

func (s *DefaultStreamResponseSpec) Content() string {
	//TODO implement me
	panic("implement me")
}

func (s *DefaultStreamResponseSpec) ChatResponse() *completion.ChatCompletion[metadata.ChatGenerationMetadata] {
	//TODO implement me
	panic("implement me")
}

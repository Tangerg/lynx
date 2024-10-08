package message

import "github.com/Tangerg/lynx/ai/core/model/media"

func NewUserMessage(content string, metadata map[string]any, m ...*media.Media) *UserMessage {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata[KeyOfMessageType] = User.String()
	if len(m) == 0 {
		m = make([]*media.Media, 0)
	}
	return &UserMessage{
		content:  content,
		metadata: metadata,
		media:    m,
	}
}

type UserMessage struct {
	content  string
	media    []*media.Media
	metadata map[string]any
}

func (s *UserMessage) Type() Type {
	return User
}

func (s *UserMessage) Content() string {
	return s.content
}

func (s *UserMessage) Metadata() map[string]any {
	return s.metadata
}

func (s *UserMessage) Media() []*media.Media {
	return s.media
}

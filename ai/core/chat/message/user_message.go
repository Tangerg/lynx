package message

import "github.com/Tangerg/lynx/ai/core/model/media"

func NewUserMessage(content string, metadata map[string]any, md ...*media.Media) *UserMessage {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata[KeyOfMessageType] = User.String()

	rv := &UserMessage{
		content:  content,
		metadata: metadata,
		media:    make([]*media.Media, 0, len(md)),
	}
	for _, m := range md {
		if m != nil {
			rv.media = append(rv.media, m)
		}
	}

	return rv
}

var _ ChatMessage = (*UserMessage)(nil)

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

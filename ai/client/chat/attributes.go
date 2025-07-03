package chat

type Attribute string

func (attr Attribute) String() string {
	return string(attr)
}

const (
	AttrChatOutputFormat Attribute = "lynx.ai.client.chat.output.format"
)

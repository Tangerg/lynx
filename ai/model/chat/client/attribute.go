package client

type Attribute string

func (attr Attribute) String() string {
	return string(attr)
}

const (
	AttrOutputFormat Attribute = "lynx.ai.model.chat.client.output_format"
)

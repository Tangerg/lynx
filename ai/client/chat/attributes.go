package chat

type Attribute string

func (attr Attribute) String() string {
	return string(attr)
}

const (
	OutputFormat Attribute = "lynx:ai:client:chat:output:format"
)

package middleware

const (
	ResponseFormatKey = "response_format"
)

type ChatRequestMode string

const (
	CallRequest   ChatRequestMode = "call"
	StreamRequest ChatRequestMode = "stream"
)

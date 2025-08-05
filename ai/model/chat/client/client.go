package client

import (
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
)

type Client struct {
	defaultConfig *Config
}

func NewClient(config *Config) (*Client, error) {
	if config == nil {
		return nil, errors.New("config is nil")
	}

	return &Client{
		defaultConfig: config.Clone(),
	}, nil
}

func (c *Client) Chat() *Config {
	return c.defaultConfig.Clone()
}

func (c *Client) ChatText(text string) *Config {
	userMessage := messages.NewUserMessage(text)

	textChatRequest, _ := chat.NewRequest(
		[]messages.Message{userMessage},
		c.defaultConfig.getChatOptions(),
	)

	return c.ChatRequest(textChatRequest)
}

func (c *Client) ChatRequest(chatRequest *chat.Request) *Config {
	clonedConfig := c.defaultConfig.Clone()

	if chatRequest.Options() != nil {
		clonedConfig.WithChatOptions(chatRequest.Options())
	}

	if len(chatRequest.Instructions()) > 0 {
		clonedConfig.WithMessages(chatRequest.Instructions()...)
	}

	if len(chatRequest.Params()) > 0 {
		clonedConfig.WithParams(chatRequest.Params())
	}

	return clonedConfig
}

func (c *Client) Fork() *Client {
	return &Client{
		defaultConfig: c.defaultConfig.Clone(),
	}
}

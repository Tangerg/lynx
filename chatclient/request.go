package chatclient

import (
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

func (c *Client) prepareRequest(request *chat.Request, outputFormat *chat.OutputFormat) (*chat.Request, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: nil request", chat.ErrInvalidRequest)
	}
	if outputFormat != nil && request.Options.OutputFormat != nil {
		return nil, fmt.Errorf("%w: request options already define output_format", ErrInvalidOutputFormat)
	}
	prepared := request.Clone()
	merged, err := c.defaults.Merged(request.Options)
	if err != nil {
		return nil, err
	}
	if outputFormat != nil {
		merged.OutputFormat = outputFormat.Clone()
	}
	prepared.Options = merged
	if err := prepared.Validate(); err != nil {
		return nil, err
	}
	return prepared, nil
}

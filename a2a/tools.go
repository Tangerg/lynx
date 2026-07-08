package a2a

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/a2aproject/a2a-go/v2/a2aclient"

	"github.com/Tangerg/lynx/core/model/chat"
)

// Endpoint describes one remote A2A agent to expose as a chat tool.
type Endpoint struct {
	Name       string
	CardURL    string
	HTTPClient *http.Client
}

// Tools resolves every endpoint and wraps each remote agent as a chat tool.
// Callers own the returned clients and should close them with [CloseClients].
func Tools(ctx context.Context, endpoints ...Endpoint) ([]chat.Tool, []*a2aclient.Client, error) {
	clients := make([]*a2aclient.Client, 0, len(endpoints))
	tools := make([]chat.Tool, 0, len(endpoints))
	seen := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		client, card, err := Dial(ctx, endpoint.CardURL, DialOptions{HTTPClient: endpoint.HTTPClient})
		if err != nil {
			return nil, nil, errors.Join(err, CloseClients(clients))
		}
		clients = append(clients, client)

		tool, err := newTool(toolConfig{Client: client, Card: card, Name: endpoint.Name})
		if err != nil {
			return nil, nil, errors.Join(err, CloseClients(clients))
		}
		name := tool.Definition().Name
		if _, dup := seen[name]; dup {
			err := fmt.Errorf("a2a.Tools: duplicate tool name %q", name)
			return nil, nil, errors.Join(err, CloseClients(clients))
		}
		seen[name] = struct{}{}
		tools = append(tools, tool)
	}
	return tools, clients, nil
}

// CloseClients destroys every client and joins any errors.
func CloseClients(clients []*a2aclient.Client) error {
	var errs []error
	for _, client := range clients {
		if client == nil {
			continue
		}
		if err := client.Destroy(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
